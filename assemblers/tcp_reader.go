package assemblers

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/honeycombio/gopacket"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/rs/zerolog/log"
)

type message struct {
	data      []byte
	timestamp time.Time
	// Seq will hold SEQ or ACK number for incoming or outgoing HTTP TCP segments
	// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
	Seq int
}

type tcpReader struct {
	isClient  bool
	srcIp     string
	srcPort   string
	dstIp     string
	dstPort   string
	data      []byte
	parent    *tcpStream
	messages  chan message
	timestamp time.Time
	seq       int
}

func NewTcpReader(isClient bool, stream *tcpStream, net gopacket.Flow, transport gopacket.Flow, config config.Config) *tcpReader {
	return &tcpReader{
		parent:   stream,
		isClient: isClient,
		srcIp:    net.Src().String(),
		dstIp:    net.Dst().String(),
		srcPort:  transport.Src().String(),
		dstPort:  transport.Dst().String(),
		messages: make(chan message, config.ChannelBufferSize),
	}
}

func (reader *tcpReader) Read(p []byte) (int, error) {
	var msg message
	ok := true
	for ok && len(reader.data) == 0 {
		msg, ok = <-reader.messages
		reader.timestamp = msg.timestamp
		reader.seq = msg.Seq
		reader.data = msg.data
		msg.data = nil // clear the []byte so we can release the memory
	}
	if !ok || len(reader.data) == 0 {
		return 0, io.EOF
	}

	l := copy(p, reader.data)
	reader.data = reader.data[l:]
	return l, nil
}

func (reader *tcpReader) run(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		b := bufio.NewReader(reader)
		if reader.isClient {
			req, err := http.ReadRequest(b)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				log.Debug().
					Err(err).
					Str("ident", reader.parent.ident).
					Msg("Error reading HTTP request")
				continue
			}

			ident := fmt.Sprintf("%s:%d", reader.parent.ident, reader.seq)
			if entry, ok := reader.parent.matcher.GetOrStoreRequest(ident, reader.timestamp, req); ok {
				// we have a match, process complete request/response pair
				reader.processEvent(ident, entry)
			}
		} else {
			res, err := http.ReadResponse(b, nil)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			} else if err != nil {
				log.Debug().
					Err(err).
					Str("ident", reader.parent.ident).
					Msg("Error reading HTTP response")
				continue
			}

			ident := fmt.Sprintf("%s:%d", reader.parent.ident, reader.seq)
			if entry, ok := reader.parent.matcher.GetOrStoreResponse(ident, reader.timestamp, res); ok {
				// we have a match, process complete request/response pair
				reader.processEvent(ident, entry)
			}
		}
	}
}

func (reader *tcpReader) processEvent(ident string, entry *entry) {
	reader.parent.events <- HttpEvent{
		RequestId:         ident,
		Request:           entry.request,
		Response:          entry.response,
		RequestTimestamp:  entry.requestTimestamp,
		ResponseTimestamp: entry.responseTimestamp,
		SrcIp:             reader.srcIp,
		DstIp:             reader.dstIp,
	}
}

func (reader *tcpReader) close() error {
	close(reader.messages)
	reader.data = nil // release the data, free up that memory! ᕕ( ᐛ )ᕗ
	return nil
}

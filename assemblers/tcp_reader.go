package assemblers

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/rs/zerolog/log"
)

type tcpReader struct {
	streamIdent string
	isClient    bool
	srcIp       string
	srcPort     string
	dstIp       string
	dstPort     string
	matcher     *httpMatcher
	events      chan HttpEvent
}

func NewTcpReader(streamIdent string, isClient bool, net gopacket.Flow, transport gopacket.Flow, matcher *httpMatcher, httpEvents chan HttpEvent) *tcpReader {
	return &tcpReader{
		streamIdent: streamIdent,
		isClient:    isClient,
		srcIp:       net.Src().String(),
		dstIp:       net.Dst().String(),
		srcPort:     transport.Src().String(),
		dstPort:     transport.Dst().String(),
		matcher:     matcher,
		events:      httpEvents,
	}
}

func (reader *tcpReader) reassembledSG(sg reassembly.ScatterGather, ac reassembly.AssemblerContext) {
	len, _ := sg.Lengths()
	data := sg.Fetch(len)
	ctx, ok := ac.(*Context)
	if !ok {
		log.Warn().
			Msg("Failed to cast ScatterGather to ContextWithSeq")
	}

	b := bytes.NewReader(data)
	r := bufio.NewReader(b)
	if reader.isClient {
		// We use TCP SEQ & ACK numbers to identify request/response pairs
		// ACK corresponds to SEQ of the HTTP response
		// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
		reqIdent := fmt.Sprintf("%s:%d", reader.streamIdent, ctx.ack)
		req, err := http.ReadRequest(r)

		// read request body to end and close
		DiscardBytesToEOF(req.Body)
		req.Body.Close()

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		} else if err != nil {
			log.Info().
				Err(err).
				Str("ident", reader.streamIdent).
				Msg("Error reading HTTP request")
			return
		}
		if entry, ok := reader.matcher.GetOrStoreRequest(reqIdent, ctx.CaptureInfo.Timestamp, req); ok {
			// we have a match, process complete request/response pair
			reader.processEvent(reqIdent, entry)
		}
	} else {
		// We use TCP SEQ & ACK numbers to identify request/response pairs
		// SEQ corresponds to ACK of the HTTP request
		// https://madpackets.com/2018/04/25/tcp-sequence-and-acknowledgement-numbers-explained/
		resIdent := fmt.Sprintf("%s:%d", reader.streamIdent, ctx.seq)
		res, err := http.ReadResponse(r, nil)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		} else if err != nil {
			log.Info().
				Err(err).
				Str("ident", resIdent).
				Msg("Error reading HTTP response")
			return
		}

		// read response body to end and close
		DiscardBytesToEOF(res.Body)
		res.Body.Close()

		if entry, ok := reader.matcher.GetOrStoreResponse(resIdent, ctx.CaptureInfo.Timestamp, res); ok {
			// we have a match, process complete request/response pair
			reader.processEvent(resIdent, entry)
		}
	}
}

func (reader *tcpReader) processEvent(ident string, entry *entry) {
	reader.events <- HttpEvent{
		RequestId:         ident,
		Request:           entry.request,
		Response:          entry.response,
		RequestTimestamp:  entry.requestTimestamp,
		ResponseTimestamp: entry.responseTimestamp,
		SrcIp:             reader.srcIp,
		DstIp:             reader.dstIp,
	}
}

var discardBuffer = make([]byte, 4096)

// DiscardBytesToFirstError will read in all bytes up to the first error
// reported by the given reader, then return the number of bytes discarded
// and the error encountered.
func DiscardBytesToFirstError(r io.Reader) (discarded int, err error) {
	for {
		n, e := r.Read(discardBuffer)
		discarded += n
		if e != nil {
			return discarded, e
		}
	}
}

// DiscardBytesToEOF will read in all bytes from a Reader until it
// encounters an io.EOF, then return the number of bytes.  Be careful
// of this... if used on a Reader that returns a non-io.EOF error
// consistently, this will loop forever discarding that error while
// it waits for an EOF.
func DiscardBytesToEOF(r io.Reader) (discarded int) {
	for {
		n, e := DiscardBytesToFirstError(r)
		discarded += n
		if e == io.EOF {
			return
		}
	}
}

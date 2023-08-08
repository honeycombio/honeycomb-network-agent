// Copyright 2012 Google, Inc. All rights reserved.
//
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file in the root of the source
// tree.

// This binary provides sample code for using the gopacket TCP assembler and TCP
// stream reader.  It reads packets off the wire and reconstructs HTTP requests
// it sees, logging them.
package httputils

import (
	"bufio"
	"flag"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/examples/util"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
	"github.com/honeycombio/libhoney-go"
)

var iface = flag.String("i", "eth0", "Interface to get packets from")
// for local mac use en0 interface for wifi
// var iface = flag.String("i", "en0", "Interface to get packets from")
var fname = flag.String("r", "", "Filename to read from, overrides -i")
var snaplen = flag.Int("s", 1600, "SnapLen for pcap packet capture")
// var filter = flag.String("f", "tcp and dst port 80", "BPF filter for pcap")
// var filter = flag.String("f", "tcp portrange 80-443", "BPF filter for pcap")
var filter = flag.String("f", "tcp", "BPF filter for pcap")
// var filter = flag.String("f", "tcp and dst port 1234", "BPF filter for pcap")
var logAllPackets = flag.Bool("v", false, "Logs every packet in great detail")

// Build a simple HTTP request parser using tcpassembly.StreamFactory and tcpassembly.Stream interfaces

// httpStreamFactory implements tcpassembly.StreamFactory
type httpStreamFactory struct{}

// httpStream will handle the actual decoding of http requests.
type httpStream struct {
	net, transport gopacket.Flow
	r              tcpreader.ReaderStream
}

func (h *httpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	hstream := &httpStream{
		net:       net,
		transport: transport,
		r:         tcpreader.NewReaderStream(),
	}
	go hstream.run() // Important... we must guarantee that data from the reader stream is read.

	// ReaderStream implements tcpassembly.Stream, so we can return a pointer to it.
	return &hstream.r
}

func (h *httpStream) run() {
	buf := bufio.NewReader(&h.r)
	for {
		req, err := http.ReadRequest(buf)
		if err == io.EOF {
			// We must read until we see an EOF... very important!
			return
		} else if err != nil {
			log.Println("Error reading stream", h.net, h.transport, ":", err)
		} else {
			bodyBytes := tcpreader.DiscardBytesToEOF(req.Body)
			req.Body.Close()

			log.Println("name", "http_packet")
			log.Println("http.destination_ip", h.net.Dst().String())
			log.Println("http.source_ip", h.net.Src().String())
			log.Println("http.request_method:", req.Method)
			log.Println("http.request_url", req.RequestURI)
			log.Println("http.destination_port:", h.transport.Dst().String())
			log.Println("http.source_port:", h.transport.Src().String())
			log.Println("http.event_type", h.transport.Dst().EndpointType().String())
			log.Println("http.request_body:", req.Body)
			log.Println("http.request_header:", req.Header)
			log.Println("http.request_url:", req.RequestURI)
			log.Println("http.body_bytes:", bodyBytes)

			log.Println("Received request from stream", h.net, h.transport, ":", req, "with", bodyBytes, "bytes in request body")

			ev := libhoney.NewEvent()
			ev.AddField("name", "http_packet")
			ev.AddField("http.destination_ip", h.net.Dst().String())
			ev.AddField("http.source_ip", h.net.Src().String())
			ev.AddField("http.request_method", req.Method)
			ev.AddField("http.request_url", req.RequestURI)
			ev.AddField("http.destination_port", h.transport.Dst().String())
			ev.AddField("http.source_port", h.transport.Src().String())
			ev.AddField("http.event_type", h.transport.Dst().EndpointType().String())
			ev.AddField("http.request_body", req.Body)
			ev.AddField("http.request_header", req.Header)
			ev.AddField("http.request_bodybytes", bodyBytes)

			err := ev.Send()
			if err != nil {
				log.Printf("error sending event: %v\n", err)
			}
		}
	}
}

func HttpStream() {
	defer util.Run()()
	var handle *pcap.Handle
	var err error

	// Set up pcap packet capture
	if *fname != "" {
		log.Printf("Reading from pcap dump %q", *fname)
		handle, err = pcap.OpenOffline(*fname)
	} else {
		log.Printf("Starting capture on interface %q", *iface)
		handle, err = pcap.OpenLive(*iface, int32(*snaplen), true, pcap.BlockForever)
	}
	if err != nil {
		log.Fatal(err)
	}

	if err := handle.SetBPFFilter(*filter); err != nil {
		log.Fatal(err)
	}

	// Set up assembly
	streamFactory := &httpStreamFactory{}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)

	log.Println("reading in packets")
	// Read in packets, pass to assembler.
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packets := packetSource.Packets()
	ticker := time.Tick(time.Minute)
	for {
		select {
		case packet := <-packets:
			// A nil packet indicates the end of a pcap file.
			if packet == nil {
				return
			}
			if *logAllPackets {
				log.Println(packet)
			}
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeTCP {
				log.Println("Unusable packet")
				continue
			}
			tcp := packet.TransportLayer().(*layers.TCP)
			assembler.AssembleWithTimestamp(packet.NetworkLayer().NetworkFlow(), tcp, packet.Metadata().Timestamp)

		case <-ticker:
			// Every minute, flush connections that haven't seen activity in the past 2 minutes.
			assembler.FlushOlderThan(time.Now().Add(time.Minute * -2))
		}
	}
}

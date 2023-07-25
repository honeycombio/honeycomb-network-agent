package probes

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/honeycombio/libhoney-go"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 -cc clang -cflags $CFLAGS bpf tcp_probe.c

const mapKey uint32 = 0

type Event struct {
	StartTime uint64
	EndTime uint64
	Daddr uint32
	Dport uint16
	Saddr uint32
	Sport uint16
	BytesSent uint64
}

func Setup() {
	// Load pre-compiled programs and maps into the kernel.
	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, nil); err != nil {
		log.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	// Deploy tcp_connect kprobe
	kprobeTcpConnect, err := link.Kprobe("tcp_connect", objs.KprobeTcpConnect, nil)
	if err != nil {
		log.Fatalf("opening kprobe: %s", err)
	}
	defer kprobeTcpConnect.Close()

	// Deploy tcp_sendmsg kprobe
	kprobeSendMsg, err := link.Kprobe("tcp_sendmsg", objs.KprobeSendmsg, nil)
	if err != nil {
		log.Fatalf("opening kprobe: %s", err)
	}
	defer kprobeSendMsg.Close()

	// Deploy tcp_close kprobe
	kprobeTcpClose, err := link.Kprobe("tcp_close", objs.KprobeTcpClose, nil)
	if err != nil {
		log.Fatal("failed to open kretprobe: %s", err)
	}
	defer kprobeTcpClose.Close()

	// Setup perf event reader to read probe events
	reader, err := perf.NewReader(objs.Events, os.Getpagesize())
	if err != nil {
		log.Fatalf("failed creating perf reader: %v", err)
	}

	log.Println("Agent is ready!")
	var event Event
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				return
			}
			continue
		}

		if record.LostSamples != 0 {
			continue
		}

		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Println("error parsing perf event", err)
			continue
		}

		// log.Printf("event: %+v\n", event)

		ev := libhoney.NewEvent()
		ev.AddField("name", "tcp_event")
		ev.AddField("duration_ms", (event.EndTime - event.StartTime) / 1_000_000) // convert ns to ms
		ev.AddField("source", fmt.Sprintf("%s:%d", toIP4(event.Saddr), event.Sport))
		ev.AddField("dest", fmt.Sprintf("%s:%d", toIP4(event.Daddr), event.Dport))
		ev.AddField("num_bytes", event.BytesSent)
		err = ev.Send()
		if err != nil {
			log.Printf("error sending event: %v\n", err)
		}
	}
}

func toIP4(addr uint32) net.IP {
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, addr)
	return ip
}
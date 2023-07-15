package socket

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log"
	"os"
	"syscall"
	"unsafe"

	"github.com/cilium/ebpf/perf"
	"golang.org/x/sys/unix"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 -cc clang -cflags $CFLAGS bpf socket.c

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

	socketFilter := objs.SocketHttpFilter

	// Deploy socket filter
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		log.Fatal("Failed to create socket: ", err)
	}
	err = syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ATTACH_BPF, socketFilter.FD())
	if err != nil {
		log.Fatal("Failed to attach socket filter: ", err)
	}
	defer unix.Close(fd)

	// Setup perf event reader to read probe events
	reader, err := perf.NewReader(objs.Events, os.Getpagesize())
	if err != nil {
		log.Fatalf("failed creating perf reader: %v", err)
	}

	log.Println("Waiting for events..")
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

		log.Printf("event: %+v\n", event)

		// ev := libhoney.NewEvent()
		// ev.AddField("name", "socket_event")
		// ev.AddField("duration_ms", (event.EndTime - event.StartTime) / 1_000_000) // convert ns to ms
		// ev.AddField("source", fmt.Sprintf("%s:%d", toIP4(event.Saddr), event.Sport))
		// ev.AddField("dest", fmt.Sprintf("%s:%d", toIP4(event.Daddr), event.Dport))
		// ev.AddField("num_bytes", event.BytesSent)
		// err = ev.Send()
		// if err != nil {
		// 	log.Printf("error sending event: %v\n", err)
		// }
	}
}

func isLittleEndian() bool {
	var a uint16 = 1

	return *(*byte)(unsafe.Pointer(&a)) == 1
}

func htons(a uint16) uint16 {
	if isLittleEndian() {
		var arr [2]byte
		binary.LittleEndian.PutUint16(arr[:], a)
		return binary.BigEndian.Uint16(arr[:])
	}
	return a
}
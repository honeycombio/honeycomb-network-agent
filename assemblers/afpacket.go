package assemblers

import (
	"fmt"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/bpf"
)

type afpacketHandle struct {
	TPacket *afpacket.TPacket
}

// ZeroCopyReadPacketData satisfies ZeroCopyPacketDataSource interface
func (h *afpacketHandle) ZeroCopyReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error) {
	return h.TPacket.ZeroCopyReadPacketData()
}

// SetBPFFilter translates a BPF filter string into BPF RawInstruction and applies them.
func (h *afpacketHandle) SetBPFFilter(filter string, snaplen int) (err error) {
	pcapBPF, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, snaplen, filter)
	if err != nil {
		return err
	}
	bpfIns := []bpf.RawInstruction{}
	for _, ins := range pcapBPF {
		bpfIns2 := bpf.RawInstruction{
			Op: ins.Code,
			Jt: ins.Jt,
			Jf: ins.Jf,
			K:  ins.K,
		}
		bpfIns = append(bpfIns, bpfIns2)
	}
	if h.TPacket.SetBPF(bpfIns); err != nil {
		return err
	}
	return h.TPacket.SetBPF(bpfIns)
}

// LinkType returns ethernet link type.
func (h *afpacketHandle) LinkType() layers.LinkType {
	return layers.LinkTypeEthernet
}

// Close will close afpacket source.
func (h *afpacketHandle) Close() {
	h.TPacket.Close()
}

// SocketStats prints received, dropped, queue-freeze packet stats.
func (h *afpacketHandle) SocketStats() (afpacket.SocketStats, afpacket.SocketStatsV3, error) {
	return h.TPacket.SocketStats()
}

func newAfpacketSource(config config) (*gopacket.PacketSource, error) {
	// subtract 1 from snaplen to account for the VLAN frame header
	snaplen := config.Snaplen - 1
	if snaplen < 0 {
		snaplen = 0
	}

	frameSize, blockSize, numBlocks, err := afpacketComputeSize(config.TargetSizeMB, snaplen, os.Getpagesize())
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("interface", config.Interface).
		Int("snaplen", snaplen).
		Str("bpf_filter", config.bpfFilter).
		Int("frame_size", frameSize).
		Int("block_size", blockSize).
		Int("num_blocks", numBlocks).
		Int("target_size_mb", targetSizeMB).
		Int("page_size", os.Getpagesize()).
		Msg("Configuring afpacket packet source")

	opts := []interface{}{
		afpacket.OptFrameSize(frameSize),
		afpacket.OptBlockSize(blockSize),
		afpacket.OptNumBlocks(numBlocks),
		afpacket.OptAddVLANHeader(false),
		afpacket.OptPollTimeout(pcap.BlockForever),
		afpacket.SocketRaw,
		afpacket.TPacketVersion3,
	}
	if config.Interface != "any" {
		opts = append(opts, afpacket.OptInterface(config.Interface))
	}

	handle := &afpacketHandle{}
	handle.TPacket, err = afpacket.NewTPacket(
		opts...,
	)
	if err != nil {
		return nil, err
	}

	if config.bpfFilter != "" {
		handle.SetBPFFilter(config.bpfFilter, config.Snaplen)
	}

	go logAfpacketHandleStats(handle)
	return gopacket.NewPacketSource(
		handle.TPacket,
		handle.LinkType(),
	), nil
}

func logAfpacketHandleStats(handle *afpacketHandle) {
	ticker := time.NewTicker(time.Second * 10)
	for {
		<-ticker.C
		_, socketStatsV3, err := handle.SocketStats()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get afpacket socket stats")
			continue
		}
		log.Info().
			Uint("packets", socketStatsV3.Packets()).
			Uint("drops", socketStatsV3.Drops()).
			Msg("Afpacket handle stats")
	}
}

// afpacketComputeSize computes the block_size and the num_blocks in such a way that the
// allocated mmap buffer is close to but smaller than target_size_mb.
// The restriction is that the block_size must be divisible by both the
// frame size and page size.
func afpacketComputeSize(targetSizeMb int, snaplen int, pageSize int) (
	frameSize int, blockSize int, numBlocks int, err error) {

	if snaplen < pageSize {
		frameSize = pageSize / (pageSize / snaplen)
	} else {
		frameSize = (snaplen/pageSize + 1) * pageSize
	}

	// 128 is the default from the gopacket library so just use that
	blockSize = frameSize * 128
	numBlocks = (targetSizeMb * 1024 * 1024) / blockSize

	if numBlocks == 0 {
		return 0, 0, 0, fmt.Errorf("Interface buffersize is too small")
	}

	return frameSize, blockSize, numBlocks, nil
}

package assemblers

import (
	"encoding/json"
	"flag"
	"time"

	"github.com/rs/zerolog/log"
)

const timeout time.Duration = time.Second * 30

var maxcount = flag.Int("c", -1, "Only grab this many packets, then exit")
var statsevery = flag.Int("stats", 1000, "Output statistics every N packets")
var lazy = flag.Bool("lazy", false, "If true, do lazy decoding")
var nodefrag = flag.Bool("nodefrag", false, "If true, do not do IPv4 defrag")
var checksum = flag.Bool("checksum", false, "Check TCP checksum")
var nooptcheck = flag.Bool("nooptcheck", true, "Do not check TCP options (useful to ignore MSS on captures with TSO)")
var ignorefsmerr = flag.Bool("ignorefsmerr", true, "Ignore TCP FSM errors")
var allowmissinginit = flag.Bool("allowmissinginit", true, "Support streams without SYN/SYN+ACK/ACK sequence")
var verbose = flag.Bool("verbose", false, "Be verbose")
var debug = flag.Bool("debug", false, "Display debug information")
var quiet = flag.Bool("quiet", false, "Be quiet regarding errors")

// capture
var iface = flag.String("i", "any", "Interface to read packets from")
var fname = flag.String("r", "", "Filename to read from, overrides -i")
var snaplen = flag.Int("s", 65536, "Snap length (number of bytes max to read per packet")
var tstype = flag.String("timestamp_type", "", "Type of timestamps to use")
var promisc = flag.Bool("promisc", true, "Set promiscuous mode")
var packetSource = flag.String("source", "pcap", "Packet source (defaults to pcap)")
var bpfFilter = flag.String("filter", "tcp", "BPF filter")

type config struct {
	Maxcount         int
	Statsevery       int
	Lazy             bool
	Nodefrag         bool
	Checksum         bool
	Nooptcheck       bool
	Ignorefsmerr     bool
	Allowmissinginit bool
	Verbose          bool
	Debug            bool
	Quiet            bool
	Interface        string
	FileName         string
	Snaplen          int
	TsType           string
	Promiscuous      bool
	CloseTimeout     time.Duration
	Timeout          time.Duration
	packetSource     string
	bpfFilter        string
}

func NewConfig() *config {
	c := &config{
		Maxcount:         *maxcount,
		Statsevery:       *statsevery,
		Lazy:             *lazy,
		Nodefrag:         *nodefrag,
		Checksum:         *checksum,
		Nooptcheck:       *nooptcheck,
		Ignorefsmerr:     *ignorefsmerr,
		Allowmissinginit: *allowmissinginit,
		Verbose:          *verbose,
		Debug:            *debug,
		Quiet:            *quiet,
		Interface:        *iface,
		FileName:         *fname,
		Snaplen:          *snaplen,
		TsType:           *tstype,
		Promiscuous:      *promisc,
		Timeout:          timeout,
		packetSource:     *packetSource,
		bpfFilter:        *bpfFilter,
	}

	if c.Debug {
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			log.Debug().Err(err).Msg("Failed to marshal agent config")
		} else {
			log.Debug().RawJSON("Agent config", b)
		}
	}
	return c
}

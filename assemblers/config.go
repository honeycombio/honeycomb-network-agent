package assemblers

import (
	"encoding/json"
	"flag"
	"strings"
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

	// reference links:
	// https://www.middlewareinventory.com/blog/tcpdump-capture-http-get-post-requests-apache-weblogic-websphere/
	// https://www.middlewareinventory.com/ascii-table/
	// filters := []string{}
	// GET, PUT, POST, DELETE are request start strings
	// HTTP 1.1 is the response start string
	// for _, method := range []string{"GET"} {
	// 	bytes := []byte(method)
	// 	encodedStr := hex.EncodeToString(bytes)
	// 	filters = append(filters, fmt.Sprintf("tcp[((tcp[12:1] & 0xf0) >> 2):%d] = 0x%s", len(method), string(encodedStr)))
	// }
	// c.bpfFilter = strings.Join(filters, " or ")
	filters := []string{
		// "not host me", // how do we get our current IP?
		// tcp[((tcp[12:1] & 0xf0) >> 2):<num> means skip the ip & tcp headers, then get the next <num> bytes and match hex
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x47455420", // 'GET '
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x504F5354", // 'POST'
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x50555420", // 'PUT '
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x44454C45", // 'DELE'TE
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x48545450", // 'HTTP' 1.1
	}
	c.bpfFilter = strings.Join(filters, " or ")

	// write me a bpf filter that matches http packets starting with get, post, put, delete
	// tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x47455420 or tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x504F5354 or tcp[((tcp[12:1] & 0xf0) >> 2):3] = 0x505554 or tcp[((tcp[12:1] & 0xf0) >> 2):6] = 0x44454C455445 or tcp[((tcp[12:1] & 0xf0) >> 2):8] = 0x485454502F312E31

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

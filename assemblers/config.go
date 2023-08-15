package assemblers

import (
	"flag"
	"time"
)

const closeTimeout time.Duration = time.Hour * 24
const timeout time.Duration = time.Minute * 5

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

type config struct {
	maxcount int
	statsevery int
	lazy bool
	nodefrag bool
	checksum bool
	nooptcheck bool
	ignorefsmerr bool
	allowmissinginit bool
	verbose bool
	debug bool
	quiet bool

	iface string
	fname string
	snaplen int
	tstype string
	promisc bool

	closeTimeout time.Duration
	timeout time.Duration
}

func NewConfig() *config {
	return &config{
		maxcount: *maxcount,
		statsevery: *statsevery,
		lazy: *lazy,
		nodefrag: *nodefrag,
		checksum: *checksum,
		nooptcheck: *nooptcheck,
		ignorefsmerr: *ignorefsmerr,
		allowmissinginit: *allowmissinginit,
		verbose: *verbose,
		debug: *debug,
		quiet: *quiet,

		iface: *iface,
		fname: *fname,
		snaplen: *snaplen,
		tstype: *tstype,
		promisc: *promisc,

		closeTimeout: closeTimeout,
		timeout: timeout,
	}
}

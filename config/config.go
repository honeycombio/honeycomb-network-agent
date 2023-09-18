package config

import (
	"errors"
	"flag"
	"strings"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/utils"
)

var apiKey = flag.String("api_key", utils.LookupEnvOrString("HONEYCOMB_API_KEY", ""), "Honeycomb API key")
var endpoint = flag.String("endpoint", utils.LookupEnvOrString("HONEYCOMB_API_ENDPOINT", "https://api.honeycomb.io"), "Honeycomb API endpoint")
var dataset = flag.String("dataset", utils.LookupEnvOrString("HONEYCOMB_DATASET", "hny-network-agent"), "Honeycomb dataset name")
var statsDataset = flag.String("stats_dataset", utils.LookupEnvOrString("HONEYCOMB_STATS_DATASET", "hny-network-agent-stats"), "Honeycomb dataset name for stats")
var logLevel = flag.String("log_level", utils.LookupEnvOrString("LOG_LEVEL", "INFO"), "Log level (defaults to INFO)")
var debug = flag.Bool("debug", utils.LookupEnvOrBool("DEBUG", false), "Runs the agent in debug mode including setting up debug service")
var debugAddress = flag.String("debug_address", utils.LookupEnvOrString("DEBUG_ADDRESS", "0.0.0.0:6060"), "Debug service address")

// unsure if we want to expose these as env vars or even flags yet -- maybe just defaults?
var lazy = flag.Bool("lazy", false, "If true, do lazy decoding")
var nodefrag = flag.Bool("nodefrag", false, "If true, do not do IPv4 defrag")
var checksum = flag.Bool("checksum", false, "Check TCP checksum")
var nooptcheck = flag.Bool("nooptcheck", true, "Do not check TCP options (useful to ignore MSS on captures with TSO)")
var ignorefsmerr = flag.Bool("ignorefsmerr", true, "Ignore TCP FSM errors")
var allowmissinginit = flag.Bool("allowmissinginit", true, "Support streams without SYN/SYN+ACK/ACK sequence")
var iface = flag.String("i", "any", "Interface to read packets from")
var snaplen = flag.Int("s", 262144, "Snap length (number of bytes max to read per packet") // 262144 is the default snaplen for tcpdump
var promisc = flag.Bool("promisc", true, "Set promiscuous mode")
var packetSource = flag.String("source", "pcap", "Packet source (defaults to pcap)")
var channelBufferSize = flag.Int("channel_buffer_size", 1000, "Channel buffer size (defaults to 1000)")
var streamFlushTimeout = flag.Int("stream_flush_timeout", 10, "Stream flush timeout in seconds (defaults to 10)")
var streamCloseTimeout = flag.Int("stream_close_timeout", 90, "Stream close timeout in seconds (defaults to 90)")
var maxBufferedPagesTotal = flag.Int("gopacket_pages", 150_000, "Maximum number of TCP reassembly pages to allocate per interface")
var maxBufferedPagesPerConnection = flag.Int("gopacket_per_conn", 4000, "Maximum number of TCP reassembly pages per connection")

type Config struct {
	APIKey                        string
	Endpoint                      string
	Dataset                       string
	StatsDataset                  string
	LogLevel                      string
	Debug                         bool
	DebugAddress                  string
	Lazy                          bool
	Nodefrag                      bool
	Checksum                      bool
	Nooptcheck                    bool
	Ignorefsmerr                  bool
	Allowmissinginit              bool
	Interface                     string
	Snaplen                       int
	Promiscuous                   bool
	StreamFlushTimeout            time.Duration
	StreamCloseTimeout            time.Duration
	PacketSource                  string
	BpfFilter                     string
	ChannelBufferSize             int
	MaxBufferedPagesTotal         int
	MaxBufferedPagesPerConnection int
}

func NewConfig() Config {
	return Config{
		APIKey:                        *apiKey,
		Endpoint:                      *endpoint,
		Dataset:                       *dataset,
		StatsDataset:                  *statsDataset,
		LogLevel:                      *logLevel,
		Debug:                         *debug,
		DebugAddress:                  *debugAddress,
		Lazy:                          *lazy,
		Nodefrag:                      *nodefrag,
		Checksum:                      *checksum,
		Nooptcheck:                    *nooptcheck,
		Ignorefsmerr:                  *ignorefsmerr,
		Allowmissinginit:              *allowmissinginit,
		Interface:                     *iface,
		Snaplen:                       *snaplen,
		Promiscuous:                   *promisc,
		StreamFlushTimeout:            time.Duration(*streamFlushTimeout) * time.Second,
		StreamCloseTimeout:            time.Duration(*streamCloseTimeout) * time.Second,
		PacketSource:                  *packetSource,
		BpfFilter:                     buildBpfFilter(),
		ChannelBufferSize:             *channelBufferSize,
		MaxBufferedPagesTotal:         *maxBufferedPagesTotal,
		MaxBufferedPagesPerConnection: *maxBufferedPagesPerConnection,
	}
}

// GetMaskedAPIKey masks the API key for logging purposes
// if the API key is less than 4 characters, it will be completely masked
func (c *Config) GetMaskedAPIKey() string {
	if len(c.APIKey) <= 4 {
		return strings.Repeat("*", len(c.APIKey))
	}
	return strings.Repeat("*", len(c.APIKey)-4) + c.APIKey[len(c.APIKey)-4:]
}

// buildBpfFilter builds a BPF filter to only capture HTTP traffic
// TODO: Move somewhere more appropriate
func buildBpfFilter() string {
	// Add filters to only capture common HTTP methods
	// TODO "not host me", // how do we get our current IP?
	// reference links:
	// https://www.middlewareinventory.com/blog/tcpdump-capture-http-get-post-requests-apache-weblogic-websphere/
	// https://www.middlewareinventory.com/ascii-table/
	filters := []string{
		// tcp[((tcp[12:1] & 0xf0) >> 2):<num> means skip the ip & tcp headers, then get the next <num> bytes and match hex
		// bpf insists that we must use 1, 2, or 4 bytes
		// HTTP Methods are request start strings
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x47455420", // 'GET '
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x504F5354", // 'POST'
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x50555420", // 'PUT '
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x44454C45", // 'DELE'TE
		// HTTP 1.1 is the response start string
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x48545450", // 'HTTP' 1.1
	}
	return strings.Join(filters, " or ")
}

type MissingAPIKeyError struct{}

func (e *MissingAPIKeyError) Error() string {
	return "Missing API key"
}

type InvalidAPIKeyError struct{}

func (e *InvalidAPIKeyError) Error() string {
	return "Invalid API key"
}

// Validate checks that the config is valid
func (c *Config) Validate() error {
	e := []error{}
	if c.APIKey == "" {
		e = append(e, &MissingAPIKeyError{})
	}
	if len(c.APIKey) < 22 {
		e = append(e, &InvalidAPIKeyError{})
	}
	// returns nil if no errors in slice
	return errors.Join(e...)
}

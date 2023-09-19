package config

import (
	"errors"
	"flag"
	"strings"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/honeycombio/libhoney-go"
)

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
		APIKey:                        *flag.String("api_key", utils.LookupEnvOrString("HONEYCOMB_API_KEY", ""), "Honeycomb API key"),
		Endpoint:                      *flag.String("endpoint", utils.LookupEnvOrString("HONEYCOMB_API_ENDPOINT", "https://api.honeycomb.io"), "Honeycomb API endpoint"),
		Dataset:                       *flag.String("dataset", utils.LookupEnvOrString("HONEYCOMB_DATASET", "hny-network-agent"), "Honeycomb dataset name"),
		StatsDataset:                  *flag.String("stats_dataset", utils.LookupEnvOrString("HONEYCOMB_STATS_DATASET", "hny-network-agent-stats"), "Honeycomb dataset name for stats"),
		LogLevel:                      *flag.String("log_level", utils.LookupEnvOrString("LOG_LEVEL", "INFO"), "Log level (defaults to INFO)"),
		Debug:                         *flag.Bool("debug", utils.LookupEnvOrBool("DEBUG", false), "Runs the agent in debug mode including setting up debug service"),
		DebugAddress:                  *flag.String("debug_address", utils.LookupEnvOrString("DEBUG_ADDRESS", "0.0.0.0:6060"), "Debug service address"),
		Lazy:                          *flag.Bool("lazy", false, "If true, do lazy decoding"),
		Nodefrag:                      *flag.Bool("nodefrag", false, "If true, do not do IPv4 defrag"),
		Checksum:                      *flag.Bool("checksum", false, "Check TCP checksum"),
		Nooptcheck:                    *flag.Bool("nooptcheck", true, "Do not check TCP options (useful to ignore MSS on captures with TSO)"),
		Ignorefsmerr:                  *flag.Bool("ignorefsmerr", true, "Ignore TCP FSM errors"),
		Allowmissinginit:              *flag.Bool("allowmissinginit", true, "Support streams without SYN/SYN+ACK/ACK sequence"),
		Interface:                     *flag.String("i", "any", "Interface to read packets from"),
		Snaplen:                       *flag.Int("s", 262144, "Snap length (number of bytes max to read per packet"), // 262144 is the default snaplen for tcpdump
		Promiscuous:                   *flag.Bool("promisc", true, "Set promiscuous mode"),
		StreamFlushTimeout:            time.Duration(*flag.Int("stream_flush_timeout", 10, "Stream flush timeout in seconds (defaults to 10)")) * time.Second,
		StreamCloseTimeout:            time.Duration(*flag.Int("stream_close_timeout", 90, "Stream close timeout in seconds (defaults to 90)")) * time.Second,
		PacketSource:                  *flag.String("source", "pcap", "Packet source (defaults to pcap)"),
		BpfFilter:                     buildBpfFilter(),
		ChannelBufferSize:             *flag.Int("channel_buffer_size", 1000, "Channel buffer size (defaults to 1000)"),
		MaxBufferedPagesTotal:         *flag.Int("gopacket_pages", 150_000, "Maximum number of TCP reassembly pages to allocate per interface"),
		MaxBufferedPagesPerConnection: *flag.Int("gopacket_per_conn", 4000, "Maximum number of TCP reassembly pages per connection"),
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
	libhoneyConfig := libhoney.Config{APIKey: c.APIKey}
	if _, err := libhoney.VerifyAPIKey(libhoneyConfig); err != nil {
		e = append(e, &InvalidAPIKeyError{})
	}
	// returns nil if no errors in slice
	return errors.Join(e...)
}

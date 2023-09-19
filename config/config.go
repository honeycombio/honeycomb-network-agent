package config

import (
	"errors"
	"strings"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/honeycombio/libhoney-go"
)

// Config holds the configuration for the agent
type Config struct {
	// Honeycomb API key used to send events.
	// Set via HONEYCOMB_API_KEY environment variable.
	APIKey string

	// Honeycomb API endpoint events are sent to.
	// Set via HONEYCOMB_API_ENDPOINT environment variable.
	Endpoint string

	// Honeycomb dataset name where events are stored.
	// Set via HONEYCOMB_DATASET environment variable.
	Dataset string

	// Honeycomb dataset name where stats events are stored.
	// Set via HONEYCOMB_STATS_DATASET environment variable.
	StatsDataset string

	// Log level used by the agent.
	// Set via LOG_LEVEL environment variable
	LogLevel string

	// Run the agent in debug mode.
	// Set via DEBUG environment variable.
	Debug bool

	// Debug service address.
	// Set via DEBUG_ADDRESS environment variable.
	DebugAddress string

	// Do lazy decoding of layers when processing packets.
	Lazy bool

	// Do not do IPv4 defragmentation when processing packets.
	Nodefrag bool

	// Check TCP checksum when processing packets.
	Checksum bool

	// Do not check TCP options (useful to ignore MSS on captures with TSO).
	Nooptcheck bool

	// Ignore TCP FSM errors.
	Ignorefsmerr bool

	// Support streams without SYN/SYN+ACK/ACK sequence.
	Allowmissinginit bool

	// Interface to read packets from.
	Interface string

	// Snap length (number of bytes max to read per packet (defaults to 262144 which is the default snaplen tcpdump).
	Snaplen int

	// Set promiscuous mode on the interface.
	Promiscuous bool

	// Stream flush timeout in seconds (defaults to 10 seconds).
	StreamFlushTimeout time.Duration

	// Stream close timeout in seconds (defaults to 90 seconds).
	StreamCloseTimeout time.Duration

	// Packet source (defaults to pcap).
	PacketSource string

	// Channel buffer size (defaults to 1000).
	BpfFilter string

	// Maximum number of HTTP events waiting to be processed to buffer before dropping.
	ChannelBufferSize int

	// Maximum number of TCP reassembly pages to allocate per interface.
	MaxBufferedPagesTotal int

	// Maximum number of TCP reassembly pages per connection.
	MaxBufferedPagesPerConnection int
}

// NewConfig returns a new Config struct.
// Values are set from environment variables if they exist, otherwise they are set to default
func NewConfig() Config {
	return Config{
		APIKey:                        utils.LookupEnvOrString("HONEYCOMB_API_KEY", ""),
		Endpoint:                      utils.LookupEnvOrString("HONEYCOMB_API_ENDPOINT", "https://api.honeycomb.io"),
		Dataset:                       utils.LookupEnvOrString("HONEYCOMB_DATASET", "hny-network-agent"),
		StatsDataset:                  utils.LookupEnvOrString("HONEYCOMB_STATS_DATASET", "hny-network-agent-stats"),
		LogLevel:                      utils.LookupEnvOrString("LOG_LEVEL", "INFO"),
		Debug:                         utils.LookupEnvOrBool("DEBUG", false),
		DebugAddress:                  utils.LookupEnvOrString("DEBUG_ADDRESS", "0.0.0.0:6060"),
		Lazy:                          false,
		Nodefrag:                      false,
		Checksum:                      false,
		Nooptcheck:                    true,
		Ignorefsmerr:                  true,
		Allowmissinginit:              true,
		Interface:                     "any",
		Snaplen:                       262144,
		Promiscuous:                   true,
		StreamFlushTimeout:            time.Duration(10 * time.Second),
		StreamCloseTimeout:            time.Duration(90 * time.Second),
		PacketSource:                  "pcap",
		BpfFilter:                     buildBpfFilter(),
		ChannelBufferSize:             1000,
		MaxBufferedPagesTotal:         150_000,
		MaxBufferedPagesPerConnection: 4000,
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

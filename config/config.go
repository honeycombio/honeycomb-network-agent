package config

import (
	"encoding/hex"
	"errors"
	"fmt"
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

	// Honeycomb destination dataset for events.
	// Set via HONEYCOMB_DATASET environment variable.
	Dataset string

	// Honeycomb destination dataset for agent performance stats.
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

	// The IP address of the node the agent is running on.
	AgentNodeIP string

	// The name of the node the agent is running on.
	AgentNodeName string

	// The name of the service account the agent is running as.
	AgentServiceAccount string

	// The IP address of the pod the agent is running on.
	AgentPodIP string

	// The name of the pod the agent is running on.
	AgentPodName string

	// Additional attributes to add to all events.
	AdditionalAttributes map[string]string

	// Include the request URL in the event.
	IncludeRequestURL bool

	// The list of HTTP headers to extract from a HTTP request/response.
	HTTPHeadersToExtract []string
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
		AgentNodeIP:                   utils.LookupEnvOrString("AGENT_NODE_IP", ""),
		AgentNodeName:                 utils.LookupEnvOrString("AGENT_NODE_NAME", ""),
		AgentServiceAccount:           utils.LookupEnvOrString("AGENT_SERVICE_ACCOUNT_NAME", ""),
		AgentPodIP:                    utils.LookupEnvOrString("AGENT_POD_IP", ""),
		AgentPodName:                  utils.LookupEnvOrString("AGENT_POD_NAME", ""),
		AdditionalAttributes:          utils.LookupEnvAsStringMap("ADDITIONAL_ATTRIBUTES"),
		IncludeRequestURL:             utils.LookupEnvOrBool("INCLUDE_REQUEST_URL", true),
		HTTPHeadersToExtract:          getHTTPHeadersToExtract(),
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

// HTTP Payloads start with one of these strings.
// Four characters are given because this feeds into a BPF filter
// and BPF really wants to match on 1, 2, or 4 byte boundaries.
var httpPayloadsStartWith = []string{
	// HTTP Methods are request start
	"GET ", "POST", "PUT ", "DELE", "HEAD", "OPTI", "PATC", "TRAC", "CONN",
	// HTTP/1.x is the response start
	"HTTP",
}

// pcapComputeTcpHeaderOffset is a [pcap filter] sub-string for pcap
// to figure out the TCP header length for a given packet.
//
// We use this to find the start of the TCP payload. See a [breakdown of this filter].
//
// [pcap filter]: https://www.tcpdump.org/manpages/pcap-filter.7.html
// [breakdown of this filter]: https://security.stackexchange.com/a/121013
const pcapComputeTcpHeaderOffset = "((tcp[12:1] & 0xf0) >> 2)"

// pcapTcpPayloadStartsWith returns a [pcap filter] string.
// The filter matches a given string against the first bytes of a TCP payload.
// Deeper details this filter can be found at [capturing HTTP requests with tcpdump].
//
// [pcap filter]: https://www.tcpdump.org/manpages/pcap-filter.7.html
// [capturing HTTP requests with tcpdump]: https://www.middlewareinventory.com/blog/tcpdump-capture-http-get-post-requests-apache-weblogic-websphere/
func pcapTcpPayloadStartsWith(s string) (filter string, err error) {
	if len(s) != 4 {
		return "", fmt.Errorf("pcapTcpPayloadStartsWith: string must be 4 characters long, got %d", len(s))
	}

	// tcp[O:N] - from TCP traffic, get the N bytes that appear after the offset O
	return fmt.Sprintf("tcp[%s:4] = 0x%s", pcapComputeTcpHeaderOffset, hex.EncodeToString([]byte(s))), nil
}

// buildBpfFilter builds a BPF filter to only capture HTTP traffic
func buildBpfFilter() string {
	// TODO: Move this logic somewhere more HTTP-flavored
	// TODO "not host me", // how do we get our current IP?

	filters := []string{}
	for _, method := range httpPayloadsStartWith {
		filter, err := pcapTcpPayloadStartsWith(method)
		if err == nil {
			filters = append(filters, filter)
		}
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
	// if endpoint doesn't match default, don't validate API key
	// this is primarily used for testing so no config options are provided
	if c.Endpoint != "https://api.honeycomb.io" {
		return nil
	}
	libhoneyConfig := libhoney.Config{APIKey: c.APIKey}
	if _, err := libhoney.VerifyAPIKey(libhoneyConfig); err != nil {
		e = append(e, &InvalidAPIKeyError{})
	}
	// returns nil if no errors in slice
	return errors.Join(e...)
}

var defaultHeadersToExtract = []string{
	"User-Agent",
}

// getHTTPHeadersToExtract returns the list of HTTP headers to extract from a HTTP request/response
func getHTTPHeadersToExtract() []string {
	if headers, found := utils.LookupEnvAsStringSlice("HTTP_HEADERS"); found {
		return headers
	}
	return defaultHeadersToExtract
}

package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIMask(t *testing.T) {
	testCases := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "empty api key",
			apiKey:   "",
			expected: "",
		},
		{
			name:     "short api key - 4 chars",
			apiKey:   "1234",
			expected: "****",
		},
		{
			name:     "short api key - 8 chars",
			apiKey:   "12345678",
			expected: "****5678",
		},
		{
			name:     "valid api key - 22 chars",
			apiKey:   "1234567890123456789012",
			expected: "******************9012",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				APIKey: tc.apiKey,
			}

			masked := config.GetMaskedAPIKey()
			assert.Equal(t, tc.expected, masked)
		})
	}
}

func TestEnvVars(t *testing.T) {
	t.Setenv("HONEYCOMB_API_KEY", "1234567890123456789012")
	t.Setenv("HONEYCOMB_API_ENDPOINT", "https://api.example.com")
	t.Setenv("HONEYCOMB_DATASET", "test-dataset")
	t.Setenv("HONEYCOMB_STATS_DATASET", "test-stats-dataset")
	t.Setenv("LOG_LEVEL", "DEBUG")
	t.Setenv("DEBUG", "true")
	t.Setenv("DEBUG_ADDRESS", "1.2.3.4:5678")
	t.Setenv("AGENT_NODE_IP", "node_ip")
	t.Setenv("AGENT_NODE_NAME", "node_name")
	t.Setenv("AGENT_SERVICE_ACCOUNT_NAME", "service_account_name")
	t.Setenv("AGENT_POD_IP", "pod_ip")
	t.Setenv("AGENT_POD_NAME", "pod_name")
	t.Setenv("ADDITIONAL_ATTRIBUTES", "key1=value1,key2=value2")
	t.Setenv("INCLUDE_REQUEST_URL", "false")
	t.Setenv("HTTP_HEADERS", "header1,header2")

	config := NewConfig()
	assert.Equal(t, "1234567890123456789012", config.APIKey)
	assert.Equal(t, "https://api.example.com", config.Endpoint)
	assert.Equal(t, "test-dataset", config.Dataset)
	assert.Equal(t, "test-stats-dataset", config.StatsDataset)
	assert.Equal(t, "DEBUG", config.LogLevel)
	assert.Equal(t, true, config.Debug)
	assert.Equal(t, "1.2.3.4:5678", config.DebugAddress)
	assert.Equal(t, "node_ip", config.AgentNodeIP)
	assert.Equal(t, "node_name", config.AgentNodeName)
	assert.Equal(t, "service_account_name", config.AgentServiceAccount)
	assert.Equal(t, "pod_ip", config.AgentPodIP)
	assert.Equal(t, "pod_name", config.AgentPodName)
	assert.Equal(t, map[string]string{"key1": "value1", "key2": "value2"}, config.AdditionalAttributes)
	assert.Equal(t, false, config.IncludeRequestURL)
	assert.Equal(t, []string{"header1", "header2"}, config.HTTPHeadersToExtract)
}

func TestEmptyHeadersEnvVar(t *testing.T) {
	t.Setenv("HTTP_HEADERS", "")

	config := NewConfig()
	assert.Equal(t, []string{}, config.HTTPHeadersToExtract)
}

func TestEnvVarsDefault(t *testing.T) {
	// clear all env vars
	// this doesn't reset the env vars for the test suite
	// we could change to use os.Unsetenv() but that would require us to know and maontain
	// all the env vars in an array
	os.Clearenv()

	config := NewConfig()
	assert.Equal(t, "", config.APIKey)
	assert.Equal(t, "https://api.honeycomb.io", config.Endpoint)
	assert.Equal(t, "hny-network-agent", config.Dataset)
	assert.Equal(t, "hny-network-agent-stats", config.StatsDataset)
	assert.Equal(t, "INFO", config.LogLevel)
	assert.Equal(t, false, config.Debug)
	assert.Equal(t, "0.0.0.0:6060", config.DebugAddress)
	assert.Equal(t, "", config.AgentNodeIP)
	assert.Equal(t, "", config.AgentNodeName)
	assert.Equal(t, "", config.AgentServiceAccount)
	assert.Equal(t, "", config.AgentPodIP)
	assert.Equal(t, "", config.AgentPodName)
	assert.Equal(t, map[string]string{}, config.AdditionalAttributes)
	assert.Equal(t, true, config.IncludeRequestURL)
	assert.Equal(t, []string{"User-Agent"}, config.HTTPHeadersToExtract)
}

func Test_Config_buildBpfFilter(t *testing.T) {
	captureFilter := buildBpfFilter()

	assert.Equal(t,
		len(httpPayloadsStartWith)-1,
		strings.Count(captureFilter, " or "),
		"complete filter joins all defined HTTP-matching filters with 'or'",
	)

	for _, httpStart := range httpPayloadsStartWith {
		httpStartHex := hex.EncodeToString([]byte(httpStart))
		description := fmt.Sprintf("includes %s (%s)", httpStartHex, httpStart)
		t.Run(description, func(t *testing.T) {
			filter, err := pcapTcpPayloadStartsWith(httpStart)
			require.NoError(t, err)
			assert.Contains(t, captureFilter, filter)
		})
	}
}

func Test_Config_pcapTcpPayloadStartsWith(t *testing.T) {
	testCases := []struct {
		startsWith     string
		expectSuccess  bool
		expectedFilter string
	}{
		{
			startsWith:     "GET",
			expectSuccess:  false,
			expectedFilter: "",
		},
		{
			startsWith:     "GET ",
			expectSuccess:  true,
			expectedFilter: "tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x47455420",
		},
		{
			startsWith:     "HEAD",
			expectSuccess:  true,
			expectedFilter: "tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x48454144",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.startsWith, func(t *testing.T) {
			filter, err := pcapTcpPayloadStartsWith(tC.startsWith)

			if tC.expectSuccess {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "string must be 4 characters long")
			}

			assert.Equal(t, tC.expectedFilter, filter)
		})
	}
}

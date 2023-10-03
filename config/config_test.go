package config_test

import (
	"testing"

	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/stretchr/testify/assert"
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
			config := config.Config{
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
	t.Setenv("AGENT_ATTRIBUTES", "key1=value1,key2=value2")

	config := config.NewConfig()
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
	assert.Equal(t, map[string]string{"key1": "value1", "key2": "value2"}, config.AgentAttributes)
}

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

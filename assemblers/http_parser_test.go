package assemblers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractHeader(t *testing.T) {
	testCases := []struct {
		name             string
		headersToExtract []string
		header           http.Header
		expected         http.Header
	}{
		{
			name:             "nil header",
			headersToExtract: nil,
			header:           nil,
			expected:         http.Header{},
		},
		{
			name:             "empty header",
			headersToExtract: nil,
			header:           http.Header{},
			expected:         http.Header{},
		},
		{
			name:             "only extracts headers we want to keep",
			headersToExtract: []string{"User-Agent", "X-Test"},
			header: http.Header{
				"Accept":     []string{"test"},
				"Host":       []string{"test"},
				"Cookie":     []string{"test"},
				"User-Agent": []string{"test"},
				"X-Test":     []string{"test"},
			},
			expected: http.Header{
				"User-Agent": []string{"test"},
				"X-Test":     []string{"test"},
			},
		},
		{
			name:             "header names are case-sensitive",
			headersToExtract: []string{"X-TEST"},
			header: http.Header{
				"x-test": []string{"test"},
			},
			expected: http.Header{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parser := newHttpParser(tc.headersToExtract)
			result := parser.extractHeaders(tc.header)
			assert.Equal(t, tc.expected, result)
		})
	}
}

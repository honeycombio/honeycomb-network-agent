package assemblers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractHeader(t *testing.T) {
	testCases := []struct {
		name     string
		header   http.Header
		headers  []string
		expected http.Header
	}{
		{
			name:     "nil header",
			header:   nil,
			headers:  nil,
			expected: http.Header{},
		},
		{
			name:     "empty header",
			header:   http.Header{},
			headers:  nil,
			expected: http.Header{},
		},
		{
			name: "only extracts headers we want to keep",
			header: http.Header{
				"Accept":     []string{"test"},
				"Host":       []string{"test"},
				"Cookie":     []string{"test"},
				"User-Agent": []string{"test"},
				"X-Test":     []string{"test"},
			},
			headers: []string{"User-Agent", "X-Test"},
			expected: http.Header{
				"User-Agent": []string{"test"},
				"X-Test":     []string{"test"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parser := newHttpParser(tc.headers)
			result := parser.extractHeaders(tc.header)
			assert.Equal(t, tc.expected, result)
		})
	}
}

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
		expected http.Header
	}{
		{
			name:     "nil header",
			header:   nil,
			expected: http.Header{},
		},
		{
			name:     "empty header",
			header:   http.Header{},
			expected: http.Header{},
		},
		{
			name: "only extracts headers we want to keep",
			header: http.Header{
				"Accept":     []string{"test"},
				"Host":       []string{"test"},
				"Cookie":     []string{"test"},
				"User-Agent": []string{"test"},
			},
			expected: http.Header{
				"User-Agent": []string{"test"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractHeaders(tc.header)
			assert.Equal(t, tc.expected, result)
		})
	}
}

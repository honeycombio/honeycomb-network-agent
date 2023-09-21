package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
)

func Test_sendHttpEventToHoneycomb(t *testing.T) {
	mockTransmission := setupTestLibhoney(t)

	testReqTime := time.Now()

	httpEvent := assemblers.HttpEvent{
		RequestId: "c->s:1->2",
		Request: &http.Request{
			Method:        "GET",
			RequestURI:    "/check?teapot=true",
			ContentLength: 42,
			Header:        http.Header{"User-Agent": []string{"teapot-checker/1.0"}},
		},
		Response: &http.Response{
			StatusCode:    418,
			ContentLength: 84,
		},
		RequestTimestamp:  testReqTime,
		ResponseTimestamp: testReqTime.Add(3 * time.Millisecond),
		SrcIp:             "1.2.3.4",
		DstIp:             "5.6.7.8",
	}

	sendHttpEventToHoneycomb(
		httpEvent,
		utils.NewCachedK8sClient(&kubernetes.Clientset{}), // TODO: mock the k8s metadata, silence for now
	)

	events := mockTransmission.Events()
	assert.Equal(t, 1, len(events), "Expected 1 and only 1 event to be sent")

	attrs := events[0].Data
	// remove dynamic time-based data before comparing
	delete(attrs, "meta.httpEvent_handled_at")
	delete(attrs, "meta.httpEvent_request_handled_latency_ms")
	delete(attrs, "meta.httpEvent_response_handled_latency_ms")

	expectedAttrs := map[string]interface{}{
		"name":                    "HTTP GET",
		"net.sock.host.addr":      "1.2.3.4",
		"destination.address":     "5.6.7.8",
		"http.request.id":         "c->s:1->2",
		"http.method":             "GET",
		"http.url":                "/check?teapot=true",
		"http.request.body.size":  int64(42),
		"http.request.timestamp":  testReqTime,
		"http.response.timestamp": testReqTime.Add(3 * time.Millisecond),
		"http.status_code":        418,
		"http.response.body.size": int64(84),
		"duration_ms":             int64(3),
		"user_agent.original":     "teapot-checker/1.0",
	}

	assert.Equal(t, expectedAttrs, attrs)
}

func setupTestLibhoney(t testing.TB) *transmission.MockSender {
	mockTransmission := &transmission.MockSender{}
	err := libhoney.Init(
		libhoney.Config{
			APIKey:       "placeholder",
			Dataset:      "placeholder",
			APIHost:      "placeholder",
			Transmission: mockTransmission,
		},
	)
	assert.NoError(t, err, "Failed to setup libhoney for testing")

	return mockTransmission
}

package handlers

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/honeycombio/honeycomb-network-agent/assemblers"
	"github.com/honeycombio/honeycomb-network-agent/config"
	"github.com/honeycombio/honeycomb-network-agent/utils"
	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_libhoneyEventHandler_handleEvent(t *testing.T) {
	// TEST SETUP

	// Test Data - an assembled HTTP Event
	testReqTime := time.Now()
	testReqPacketCount := 2
	testRespPacketCount := 3
	httpEvent := assemblers.HttpEvent{
		StreamIdent: "c->s:1->2",
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
		RequestTimestamp:    testReqTime,
		ResponseTimestamp:   testReqTime.Add(3 * time.Millisecond),
		RequestPacketCount:  testReqPacketCount,
		ResponsePacketCount: testRespPacketCount,
		SrcIp:               "1.2.3.4",
		DstIp:               "5.6.7.8",
	}

	// Test Data - k8s metadata
	srcPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "src-pod",
			Namespace: "unit-tests",
			UID:       "src-pod-uid",
		},
		Status: v1.PodStatus{
			PodIP: httpEvent.SrcIp,
		},
	}

	destPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dest-pod",
			Namespace: "unit-tests",
			UID:       "dest-pod-uid",
		},
		Status: v1.PodStatus{
			PodIP: httpEvent.DstIp,
		},
	}

	// create a fake k8s clientset with the test pod metadata and start the cached client with it
	fakeCachedK8sClient := utils.NewCachedK8sClient(fake.NewSimpleClientset(srcPod, destPod))
	cancelableCtx, done := context.WithCancel(context.Background())
	fakeCachedK8sClient.Start(cancelableCtx)

	// create event channel used to pass in events to the handler
	eventsChannel := make(chan assemblers.HttpEvent, 1)

	wgTest := sync.WaitGroup{} // used to wait for the event handler to finish

	// create the event handler with default config, fake k8s client & event channel then start it
	handler := NewLibhoneyEventHandler(config.Config{}, fakeCachedK8sClient, eventsChannel, "test")
	wgTest.Add(1)
	go handler.Start(cancelableCtx, &wgTest)

	// Setup libhoney for testing, use mock transmission to retrieve events "sent"
	// must be done after the event handler is created
	mockTransmission := setupTestLibhoney(t)

	// TEST ACTION: pass in httpEvent to handler
	eventsChannel <- httpEvent
	time.Sleep(10 * time.Millisecond) // give the handler time to process the event

	done()
	wgTest.Wait()
	handler.Close()

	// VALIDATE
	events := mockTransmission.Events()
	assert.Equal(t, 1, len(events), "Expected 1 and only 1 event to be sent")

	attrs := events[0].Data
	// remove dynamic time-based data before comparing
	delete(attrs, "meta.httpEvent_handled_at")
	delete(attrs, "meta.httpEvent_request_handled_latency_ms")
	delete(attrs, "meta.httpEvent_response_handled_latency_ms")

	expectedAttrs := map[string]interface{}{
		"name":                           "HTTP GET",
		"client.socket.address":          "1.2.3.4",
		"server.socket.address":          "5.6.7.8",
		"meta.stream.ident":              "c->s:1->2",
		"meta.seqack":                    int64(0),
		"meta.request.packet_count":      int(2),
		"meta.response.packet_count":     int(3),
		"http.request.method":            "GET",
		"url.path":                       "/check?teapot=true",
		"http.request.body.size":         int64(42),
		"http.request.timestamp":         testReqTime,
		"http.response.timestamp":        testReqTime.Add(3 * time.Millisecond),
		"http.response.status_code":      418,
		"http.response.body.size":        int64(84),
		"error":                          "HTTP client error",
		"duration_ms":                    int64(3),
		"user_agent.original":            "teapot-checker/1.0",
		"source.k8s.namespace.name":      "unit-tests",
		"source.k8s.pod.name":            "src-pod",
		"source.k8s.pod.uid":             srcPod.UID,
		"destination.k8s.namespace.name": "unit-tests",
		"destination.k8s.pod.name":       "dest-pod",
		"destination.k8s.pod.uid":        destPod.UID,
	}

	assert.Equal(t, expectedAttrs, attrs)
}

// setupTestLibhoney configures a Libhoney with a mock transmission for testing.
//
// Events sent can be found on the mock transmission:
//
//	events := mockTransmission.Events() // returns []*transmission.Event
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

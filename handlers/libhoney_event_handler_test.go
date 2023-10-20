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
	requestTimestamp := time.Now()
	responseTimestamp := requestTimestamp.Add(3 * time.Millisecond)
	event := createTestHttpEvent(requestTimestamp, responseTimestamp)

	// Test Data - k8s metadata
	srcPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "src-pod",
			Namespace: "unit-tests",
			UID:       "src-pod-uid",
		},
		Status: v1.PodStatus{
			PodIP: event.SrcIp(),
		},
	}

	destPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dest-pod",
			Namespace: "unit-tests",
			UID:       "dest-pod-uid",
		},
		Status: v1.PodStatus{
			PodIP: event.DstIp(),
		},
	}

	// create a fake k8s clientset with the test pod metadata and start the cached client with it
	fakeCachedK8sClient := utils.NewCachedK8sClient(fake.NewSimpleClientset(srcPod, destPod))
	cancelableCtx, done := context.WithCancel(context.Background())
	fakeCachedK8sClient.Start(cancelableCtx)

	// create event channel used to pass in events to the handler
	eventsChannel := make(chan assemblers.Event, 1)

	wgTest := sync.WaitGroup{} // used to wait for the event handler to finish

	testConfig := config.Config{
		IncludeRequestURL: true,
	}

	// create the event handler with default config, fake k8s client & event channel then start it
	handler := NewLibhoneyEventHandler(testConfig, fakeCachedK8sClient, eventsChannel, "test")
	wgTest.Add(1)
	go handler.Start(cancelableCtx, &wgTest)

	// Setup libhoney for testing, use mock transmission to retrieve events "sent"
	// must be done after the event handler is created
	mockTransmission := setupTestLibhoney(t)

	// TEST ACTION: pass in httpEvent to handler
	eventsChannel <- event
	time.Sleep(10 * time.Millisecond) // give the handler time to process the event

	done()
	wgTest.Wait()
	handler.Close()

	// VALIDATE
	events := mockTransmission.Events()
	assert.Equal(t, 1, len(events), "Expected 1 and only 1 event to be sent")

	attrs := events[0].Data
	// remove dynamic time-based data before comparing
	delete(attrs, "meta.event_handled_at")
	delete(attrs, "meta.request.capture_to_handle.latency_ms")
	delete(attrs, "meta.response.capture_to_handle.latency_ms")

	expectedAttrs := map[string]interface{}{
		"name":                           "HTTP GET",
		"client.socket.address":          "1.2.3.4",
		"server.socket.address":          "5.6.7.8",
		"meta.stream.ident":              "c->s:1->2",
		"meta.seqack":                    int64(0),
		"meta.request.packet_count":      int(2),
		"meta.response.packet_count":     int(3),
		"http.request.method":            "GET",
		"url.path":                       "/check",
		"http.request.body.size":         int64(42),
		"http.request.headers":           http.Header{"User-Agent": []string{"teapot-checker/1.0"}, "Connection": []string{"keep-alive"}},
		"http.response.headers":          http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}, "X-Custom-Header": []string{"tea-party"}},
		"http.request.timestamp":         requestTimestamp,
		"http.response.timestamp":        responseTimestamp,
		"http.response.status_code":      418,
		"http.response.body.size":        int64(84),
		"error":                          "HTTP client error",
		"duration_ms":                    int64(3),
		"user_agent.original":            "teapot-checker/1.0",
		"source.k8s.resource.type":       "pod",
		"source.k8s.namespace.name":      "unit-tests",
		"source.k8s.pod.name":            "src-pod",
		"source.k8s.pod.uid":             string(srcPod.UID),
		"destination.k8s.resource.type":  "pod",
		"destination.k8s.namespace.name": "unit-tests",
		"destination.k8s.pod.name":       "dest-pod",
		"destination.k8s.pod.uid":        string(destPod.UID),
	}

	assert.Equal(t, expectedAttrs, attrs)
}

func Test_libhoneyEventHandler_handleEvent_doesNotSetUrlPath(t *testing.T) {
	// Test Data - an assembled HTTP Event
	requestTimestamp := time.Now()
	responseTimestamp := requestTimestamp.Add(3 * time.Millisecond)
	event := createTestHttpEvent(requestTimestamp, responseTimestamp)

	// create a fake k8s clientset with the test pod metadata and start the cached client with it
	fakeCachedK8sClient := utils.NewCachedK8sClient(fake.NewSimpleClientset())
	cancelableCtx, done := context.WithCancel(context.Background())
	fakeCachedK8sClient.Start(cancelableCtx)

	// create event channel used to pass in events to the handler
	eventsChannel := make(chan assemblers.Event, 1)

	wgTest := sync.WaitGroup{} // used to wait for the event handler to finish

	defaultConfig := config.Config{
		IncludeRequestURL: false,
	}
	// create the event handler with default config, fake k8s client & event channel then start it
	handler := NewLibhoneyEventHandler(defaultConfig, fakeCachedK8sClient, eventsChannel, "test")
	wgTest.Add(1)
	go handler.Start(cancelableCtx, &wgTest)

	// Setup libhoney for testing, use mock transmission to retrieve events "sent"
	// must be done after the event handler is created
	mockTransmission := setupTestLibhoney(t)

	// TEST ACTION: pass in httpEvent to handler
	eventsChannel <- event
	time.Sleep(10 * time.Millisecond) // give the handler time to process the event

	done()
	wgTest.Wait()
	handler.Close()

	// VALIDATE
	events := mockTransmission.Events()
	assert.Equal(t, 1, len(events), "Expected 1 and only 1 event to be sent")

	attrs := events[0].Data

	assert.NotContains(t, attrs, "url.path")
}

func Test_libhoneyEventHandler_handleEvent_routed_to_service(t *testing.T) {
	// TEST SETUP

	// Test Data - an assembled HTTP Event
	requestTimestamp := time.Now()
	responseTimestamp := requestTimestamp.Add(3 * time.Millisecond)
	event := createTestHttpEvent(requestTimestamp, responseTimestamp)

	// Test Data - k8s metadata
	srcPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "src-pod",
			Namespace: "unit-tests",
			UID:       "src-pod-uid",
		},
		Status: v1.PodStatus{
			PodIP: event.SrcIp(),
		},
	}

	destService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dest-service",
			Namespace: "unit-tests",
			UID:       "dest-service-uid",
		},
		Spec: v1.ServiceSpec{
			ClusterIP: event.DstIp(),
		},
	}

	// create a fake k8s clientset with the test pod metadata and start the cached client with it
	fakeCachedK8sClient := utils.NewCachedK8sClient(fake.NewSimpleClientset(srcPod, destService))
	cancelableCtx, done := context.WithCancel(context.Background())
	fakeCachedK8sClient.Start(cancelableCtx)

	// create event channel used to pass in events to the handler
	eventsChannel := make(chan assemblers.Event, 1)

	wgTest := sync.WaitGroup{} // used to wait for the event handler to finish

	testConfig := config.Config{
		IncludeRequestURL: true,
	}
	// create the event handler with default config, fake k8s client & event channel then start it
	handler := NewLibhoneyEventHandler(testConfig, fakeCachedK8sClient, eventsChannel, "test")
	wgTest.Add(1)
	go handler.Start(cancelableCtx, &wgTest)

	// Setup libhoney for testing, use mock transmission to retrieve events "sent"
	// must be done after the event handler is created
	mockTransmission := setupTestLibhoney(t)

	// TEST ACTION: pass in httpEvent to handler
	eventsChannel <- event
	time.Sleep(10 * time.Millisecond) // give the handler time to process the event

	done()
	wgTest.Wait()
	handler.Close()

	// VALIDATE
	events := mockTransmission.Events()
	assert.Equal(t, 1, len(events), "Expected 1 and only 1 event to be sent")

	attrs := events[0].Data
	// remove dynamic time-based data before comparing
	delete(attrs, "meta.event_handled_at")
	delete(attrs, "meta.request.capture_to_handle.latency_ms")
	delete(attrs, "meta.response.capture_to_handle.latency_ms")

	expectedAttrs := map[string]interface{}{
		"name":                           "HTTP GET",
		"client.socket.address":          "1.2.3.4",
		"server.socket.address":          "5.6.7.8",
		"meta.stream.ident":              "c->s:1->2",
		"meta.seqack":                    int64(0),
		"meta.request.packet_count":      int(2),
		"meta.response.packet_count":     int(3),
		"http.request.method":            "GET",
		"url.path":                       "/check",
		"http.request.body.size":         int64(42),
		"http.request.headers":           http.Header{"User-Agent": []string{"teapot-checker/1.0"}, "Connection": []string{"keep-alive"}},
		"http.response.headers":          http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}, "X-Custom-Header": []string{"tea-party"}},
		"http.request.timestamp":         requestTimestamp,
		"http.response.timestamp":        responseTimestamp,
		"http.response.status_code":      418,
		"http.response.body.size":        int64(84),
		"error":                          "HTTP client error",
		"duration_ms":                    int64(3),
		"user_agent.original":            "teapot-checker/1.0",
		"source.k8s.resource.type":       "pod",
		"source.k8s.namespace.name":      "unit-tests",
		"source.k8s.pod.name":            "src-pod",
		"source.k8s.pod.uid":             string(srcPod.UID),
		"destination.k8s.resource.type":  "service",
		"destination.k8s.namespace.name": "unit-tests",
		"destination.k8s.service.name":   "dest-service",
		"destination.k8s.service.uid":    string(destService.UID),
	}

	assert.Equal(t, expectedAttrs, attrs)
}

func Test_reportingTimesAndDurations(t *testing.T) {
	// Do you remember the 21st night of September?
	var aRealRequestTime time.Time = time.Date(1978, time.September, 21, 11, 30, 0, 0, time.UTC)
	// ... a response little bit later ...
	var aRealResponseTime time.Time = aRealRequestTime.Add(3 * time.Millisecond)
	// an expectation of 'nowish' for scenarios where the code under test defaults to time.Now()
	var nowish time.Time = time.Now()

	testCases := []struct {
		desc                string
		reqTime             time.Time
		respTime            time.Time
		expectToSetDuration bool
		// empty if duration is expected, list of missing timestamps otherwise
		expectedTimestampsMissing string
		expectedDuration          int64
		expectedTelemetryTime     time.Time
	}{
		{
			desc:                  "happy path!",
			reqTime:               aRealRequestTime,
			respTime:              aRealResponseTime,
			expectToSetDuration:   true,
			expectedDuration:      3,
			expectedTelemetryTime: aRealRequestTime,
		},
		{
			desc:                      "missing request timestamp",
			reqTime:                   time.Time{},
			respTime:                  aRealResponseTime,
			expectToSetDuration:       false,
			expectedTimestampsMissing: "request",
			expectedTelemetryTime:     aRealResponseTime,
		},
		{
			desc:                      "missing response timestamp",
			reqTime:                   aRealRequestTime,
			respTime:                  time.Time{},
			expectToSetDuration:       false,
			expectedTimestampsMissing: "response",
			expectedTelemetryTime:     aRealRequestTime,
		},
		{
			desc:                      "missing both timestamps",
			reqTime:                   time.Time{},
			respTime:                  time.Time{},
			expectToSetDuration:       false,
			expectedTimestampsMissing: "request, response",
			expectedTelemetryTime:     nowish,
		},
	}
	handler := &libhoneyEventHandler{}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ev := libhoney.NewEvent()
			event := createTestHttpEvent(tC.reqTime, tC.respTime)

			handler.setTimestampsAndDurationIfValid(ev, event)

			if tC.expectedTelemetryTime != nowish {
				assert.Equal(t, tC.expectedTelemetryTime, ev.Timestamp)
			} else {
				assert.WithinDuration(
					t, tC.expectedTelemetryTime, ev.Timestamp, 10*time.Millisecond,
					"a real failure should be wildly wrong, close failures might be a slow test suite and this assertion could use a rethink",
				)
			}

			if tC.expectToSetDuration {
				assert.Equal(t, ev.Fields()["duration_ms"], tC.expectedDuration)
			} else {
				assert.Equal(t, ev.Fields()["meta.timestamps_missing"], tC.expectedTimestampsMissing)
				assert.Nil(t, ev.Fields()["duration_ms"])
			}
		})
	}
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

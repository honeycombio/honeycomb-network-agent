package assemblers

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_HttpMatcher_StoringARequest(t *testing.T) {
	matcher := newRequestResponseMatcher()

	requestId := int64(12345)
	reqTimestamp := time.Now()
	req := &http.Request{}

	entry, found := matcher.GetOrStoreRequest(requestId, reqTimestamp, req)

	assert.False(t, found, "Expect no entry when first storing a request with no response seen yet.")
	assert.Nil(t, entry)
}

func Test_HttpMatcher_StoringAResponse(t *testing.T) {
	matcher := newRequestResponseMatcher()

	requestId := int64(12345)
	resTimestamp := time.Now()
	res := &http.Response{}

	entry, found := matcher.GetOrStoreResponse(requestId, resTimestamp, res)

	assert.False(t, found, "Expect no entry found when first storing a response with no request seen yet.")
	assert.Nil(t, entry)
}

func Test_HttpMatcher_GetResponseThatMatchesRequest(t *testing.T) {
	matcher := newRequestResponseMatcher()
	requestId := int64(12345)
	unmatchRequestId := int64(54321)

	// store a response that won't match
	_, found := matcher.GetOrStoreResponse(unmatchRequestId, time.Now(), &http.Response{})
	assert.False(t, found, "Expect no matching request when storing a matchless response.")

	// store the response that will match
	resp := &http.Response{}
	_, found = matcher.GetOrStoreResponse(requestId, time.Now(), resp)
	assert.False(t, found, "Expect no matching request when storing a response first.")

	// get the response that matches a request's ident
	req := &http.Request{}
	foundEntry, found := matcher.GetOrStoreRequest(requestId, time.Now(), req)
	assert.True(t, found, "Expect the matching response was found.")
	assert.Equal(t, resp, foundEntry.response)
	assert.Equal(t, req, foundEntry.request)
}

func Test_HttpMatcher_GetRequestThatMatchesResponse(t *testing.T) {
	matcher := newRequestResponseMatcher()
	requestId := int64(12345)
	unmatchRequestId := int64(54321)

	// store a request that won't match
	_, found := matcher.GetOrStoreRequest(unmatchRequestId, time.Now(), &http.Request{})
	assert.False(t, found, "Expect no matching response when storing a matchless request.")

	// store the request that will match
	req := &http.Request{}
	_, found = matcher.GetOrStoreRequest(requestId, time.Now(), req)
	assert.False(t, found, "Expect no matching response when storing a request first.")

	// get the request that matches a response's ident
	resp := &http.Response{}
	foundEntry, found := matcher.GetOrStoreResponse(requestId, time.Now(), resp)
	assert.True(t, found, "Expect the matching request was found.")
	assert.Equal(t, req, foundEntry.request)
	assert.Equal(t, resp, foundEntry.response)
}

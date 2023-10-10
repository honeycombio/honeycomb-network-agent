package assemblers

import (
	"net/http"
	"time"
)

// HttpEvent represents a HTTP request/response pair
type HttpEvent struct {
	eventBase
	request  *http.Request
	response *http.Response
}

// Make sure HttpEvent implements Event interface
var _ Event = (*HttpEvent)(nil)

func NewHttpEvent(
	streamIdent string,
	requestId int64,
	requestTimestamp time.Time,
	responseTimestamp time.Time,
	requestPacketCount int,
	responsePacketCount int,
	srcIp string,
	dstIp string,
	request *http.Request,
	response *http.Response) *HttpEvent {
	return &HttpEvent{
		eventBase: eventBase{
			streamIdent:         streamIdent,
			requestId:           requestId,
			requestTimestamp:    requestTimestamp,
			responseTimestamp:   responseTimestamp,
			requestPacketCount:  requestPacketCount,
			responsePacketCount: responsePacketCount,
			srcIp:               srcIp,
			dstIp:               dstIp,
		},
		request:  request,
		response: response,
	}
}

// Request returns the captured HTTP request
func (event *HttpEvent) Request() *http.Request {
	return event.request
}

// Response returns the captured HTTP response
func (event *HttpEvent) Response() *http.Response {
	return event.response
}

package assemblers

import (
	"net/http"
	"time"
)

type HttpEvent struct {
	eventBase
	request  *http.Request
	response *http.Response
}

var _ Event = (*HttpEvent)(nil)

func NewHttpEvent(streamIdent string, requestId int64, requestTimestamp time.Time, responseTimestamp time.Time, requestPacketCount int, responsePacketCount int, srcIp string, dstIp string, request *http.Request, response *http.Response) *HttpEvent {
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

func (event *HttpEvent) Request() *http.Request {
	return event.request
}

func (event *HttpEvent) Response() *http.Response {
	return event.response
}

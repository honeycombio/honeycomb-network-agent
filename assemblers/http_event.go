package assemblers

import (
	"net/http"
	"time"
)

type HttpEvent struct {
	StreamId          uint64
	RequestId         int64
	Request           *http.Request
	Response          *http.Response
	RequestTimestamp  time.Time
	ResponseTimestamp time.Time
	SrcIp             string
	DstIp             string
}

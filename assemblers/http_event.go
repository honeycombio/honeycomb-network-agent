package assemblers

import (
	"net/http"
	"time"
)

type HttpEvent struct {
	RequestId         string
	Request           *http.Request
	Response          *http.Response
	RequestTimestamp  time.Time
	ResponseTimestamp time.Time
	SrcIp             string
	DstIp             string
}

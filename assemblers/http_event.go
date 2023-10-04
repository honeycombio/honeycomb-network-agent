package assemblers

import (
	"net/http"
	"time"
)

type HttpEvent struct {
	StreamIdent         string
	RequestId           int64
	Request             *http.Request
	Response            *http.Response
	RequestTimestamp    time.Time
	ResponseTimestamp   time.Time
	RequestPacketCount  int
	ResponsePacketCount int
	SrcIp               string
	DstIp               string
}

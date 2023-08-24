package assemblers

import (
	"net/http"
	"time"
)

type HttpEvent struct {
	RequestId string
	Request   *http.Request
	Response  *http.Response
	Start     time.Time
	End       time.Time
	Duration  time.Duration
	SrcIp     string
	DstIp     string
}

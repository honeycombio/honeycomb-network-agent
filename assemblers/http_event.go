package assemblers

import (
	"net/http"
	"time"
)

type HttpEvent struct {
	RequestId string
	Request   *http.Request
	Response  *http.Response
	Duration  time.Duration
	SrcIp     string
	DstIp     string
}

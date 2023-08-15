package assemblers

import (
	"net/http"
	"time"
)

type httpEvent struct {
	requestId string
	request *http.Request
	response *http.Response
	duration time.Duration
}

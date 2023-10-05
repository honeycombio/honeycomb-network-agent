package assemblers

import (
	"bufio"
	"time"
)

// parser parses a request or response
type parser interface {
	parse(stream *tcpStream, requestId int64, timestamp time.Time, isClient bool, buffer *bufio.Reader, packetCount int) (bool, error)
}

package assemblers

import (
	"bufio"
	"time"
)

// Parser parses a request or response
type Parser interface {
	parse(stream *tcpStream, requestId int64, timestamp time.Time, isClient bool, buffer *bufio.Reader, packetCount int) (bool, error)
}

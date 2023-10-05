package assemblers

import (
	"bufio"
)

type Parser interface {
	parse(stream *tcpStream, ctx *Context, isClient bool, buffer *bufio.Reader, packetCount int) (bool, error)
}

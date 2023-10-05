package assemblers

import (
	"time"
)

type Event interface {
	// StreamIdent returns a string that uniquely identifies the stream that captured this event
	StreamIdent() string

	// RequestId returns a unique identifier for the request/response cycle
	RequestId() int64

	// RequestTimestamp returns the timestamp of the request
	RequestTimestamp() time.Time

	// ResponseTimestamp returns the timestamp of the response
	ResponseTimestamp() time.Time

	// RequestPacketCount returns the number of packets in the request
	RequestPacketCount() int

	// ResponsePacketCount returns the number of packets in the response
	ResponsePacketCount() int

	// SrcIp returns the source IP address
	SrcIp() string

	// DstIp returns the destination IP address
	DstIp() string
}

type eventBase struct {
	streamIdent         string
	requestId           int64
	requestTimestamp    time.Time
	responseTimestamp   time.Time
	requestPacketCount  int
	responsePacketCount int
	srcIp               string
	dstIp               string
}

func (event *eventBase) StreamIdent() string {
	return event.streamIdent
}

func (event *eventBase) RequestId() int64 {
	return event.requestId
}

func (event *eventBase) RequestTimestamp() time.Time {
	return event.requestTimestamp
}

func (event *eventBase) ResponseTimestamp() time.Time {
	return event.responseTimestamp
}

func (event *eventBase) RequestPacketCount() int {
	return event.requestPacketCount
}

func (event *eventBase) ResponsePacketCount() int {
	return event.responsePacketCount
}

func (event *eventBase) SrcIp() string {
	return event.srcIp
}

func (event *eventBase) DstIp() string {
	return event.dstIp
}

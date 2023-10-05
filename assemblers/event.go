package assemblers

import (
	"time"
)

type Event interface {
	StreamIdent() string
	RequestId() int64
	RequestTimestamp() time.Time
	ResponseTimestamp() time.Time
	RequestPacketCount() int
	ResponsePacketCount() int
	SrcIp() string
	DstIp() string
}

type EventBase struct {
	streamIdent         string
	requestId           int64
	requestTimestamp    time.Time
	responseTimestamp   time.Time
	requestPacketCount  int
	responsePacketCount int
	srcIp               string
	dstIp               string
}

func (event *EventBase) StreamIdent() string {
	return event.streamIdent
}

func (event *EventBase) RequestId() int64 {
	return event.requestId
}

func (event *EventBase) RequestTimestamp() time.Time {
	return event.requestTimestamp
}

func (event *EventBase) ResponseTimestamp() time.Time {
	return event.responseTimestamp
}

func (event *EventBase) RequestPacketCount() int {
	return event.requestPacketCount
}

func (event *EventBase) ResponsePacketCount() int {
	return event.responsePacketCount
}

func (event *EventBase) SrcIp() string {
	return event.srcIp
}

func (event *EventBase) DstIp() string {
	return event.dstIp
}

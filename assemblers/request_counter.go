package assemblers

import "sync"

type requestCounter struct {
	requests  uint64
	responses uint64
	sync.Mutex
}

func (c *requestCounter) incrementRequest() uint64 {
	c.Lock()
	defer c.Unlock()

	c.requests++
	return c.requests
}

func (c *requestCounter) incrementResponse() uint64 {
	c.Lock()
	defer c.Unlock()

	c.responses++
	return c.responses
}

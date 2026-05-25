package model

import (
	"sync"
	"time"
)

const usageCounterCoalesceDelay = time.Millisecond

type usageCounterFlushFunc func(id int, quota int, count int)

type usageCounterCoalescer struct {
	mu         sync.Mutex
	groups     map[int]*usageCounterGroup
	flushDelay time.Duration
	flush      usageCounterFlushFunc
}

type usageCounterGroup struct {
	quota int
	count int
	done  chan struct{}
}

func newUsageCounterCoalescer(delay time.Duration, flush usageCounterFlushFunc) *usageCounterCoalescer {
	return &usageCounterCoalescer{
		groups:     make(map[int]*usageCounterGroup),
		flushDelay: delay,
		flush:      flush,
	}
}

func (c *usageCounterCoalescer) add(id int, quota int, count int) {
	if c == nil || c.flush == nil {
		return
	}
	if quota == 0 && count == 0 {
		return
	}
	c.mu.Lock()
	group := c.groups[id]
	start := false
	if group == nil {
		group = &usageCounterGroup{done: make(chan struct{})}
		c.groups[id] = group
		start = true
	}
	group.quota += quota
	group.count += count
	done := group.done
	c.mu.Unlock()

	if start {
		go c.run(id, group)
	}
	<-done
}

func (c *usageCounterCoalescer) drain() {
	for {
		c.mu.Lock()
		doneChans := make([]chan struct{}, 0, len(c.groups))
		for _, group := range c.groups {
			doneChans = append(doneChans, group.done)
		}
		c.mu.Unlock()
		if len(doneChans) == 0 {
			return
		}
		for _, done := range doneChans {
			<-done
		}
	}
}

func (c *usageCounterCoalescer) run(id int, group *usageCounterGroup) {
	if c.flushDelay > 0 {
		time.Sleep(c.flushDelay)
	}
	for {
		c.mu.Lock()
		quota := group.quota
		count := group.count
		done := group.done
		group.quota = 0
		group.count = 0
		group.done = make(chan struct{})
		c.mu.Unlock()

		c.flush(id, quota, count)
		close(done)

		c.mu.Lock()
		if group.quota == 0 && group.count == 0 {
			delete(c.groups, id)
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

var userUsageCounterCoalescer = newUsageCounterCoalescer(usageCounterCoalesceDelay, updateUserUsedQuotaAndRequestCount)
var channelUsageCounterCoalescer = newUsageCounterCoalescer(usageCounterCoalesceDelay, func(id int, quota int, _ int) {
	updateChannelUsedQuota(id, quota)
})

func FlushUsageCounterUpdates() {
	userUsageCounterCoalescer.drain()
	channelUsageCounterCoalescer.drain()
}

package model

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const consumeLogCoalesceDelay = 2 * time.Millisecond

type consumeLogCoalescerState struct {
	mu         sync.Mutex
	batch      *consumeLogBatch
	flushDelay time.Duration
}

type consumeLogBatch struct {
	requests []*consumeLogRequest
	done     chan struct{}
}

type consumeLogRequest struct {
	log  *Log
	done chan error
}

func newConsumeLogCoalescer(delay time.Duration) *consumeLogCoalescerState {
	return &consumeLogCoalescerState{flushDelay: delay}
}

func (c *consumeLogCoalescerState) add(log *Log) error {
	if c == nil {
		return insertConsumeLogDirect(log)
	}
	request := &consumeLogRequest{log: log, done: make(chan error, 1)}
	c.mu.Lock()
	batch := c.batch
	start := false
	if batch == nil {
		batch = &consumeLogBatch{done: make(chan struct{})}
		c.batch = batch
		start = true
	}
	batch.requests = append(batch.requests, request)
	c.mu.Unlock()

	if start {
		go c.run(batch)
	}
	return <-request.done
}

func (c *consumeLogCoalescerState) drain() {
	for {
		c.mu.Lock()
		batch := c.batch
		var done chan struct{}
		if batch != nil {
			done = batch.done
		}
		c.mu.Unlock()
		if done == nil {
			return
		}
		<-done
	}
}

func (c *consumeLogCoalescerState) run(batch *consumeLogBatch) {
	if c.flushDelay > 0 {
		time.Sleep(c.flushDelay)
	}
	for {
		c.mu.Lock()
		requests := batch.requests
		batch.requests = nil
		c.mu.Unlock()

		flushConsumeLogRequests(requests)

		c.mu.Lock()
		if len(batch.requests) == 0 {
			if c.batch == batch {
				c.batch = nil
			}
			close(batch.done)
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

func flushConsumeLogRequests(requests []*consumeLogRequest) {
	if len(requests) == 0 {
		return
	}
	logs := make([]*Log, 0, len(requests))
	for _, request := range requests {
		logs = append(logs, request.log)
	}
	err := insertConsumeLogsDirect(logs)
	if err == nil || len(requests) == 1 {
		for _, request := range requests {
			request.done <- err
		}
		return
	}
	for _, request := range requests {
		request.done <- insertConsumeLogDirect(request.log)
	}
}

func insertConsumeLogDirect(log *Log) error {
	fillLogDerivedFields(log)
	if err := LOG_DB.Create(log).Error; err != nil {
		return err
	}
	if err := queueLogAggregationEventsForLogs([]*Log{log}); err != nil {
		common.SysError("failed to queue log aggregation events: " + err.Error())
		requestMissingLogAggregationReplay()
	}
	return nil
}

func insertConsumeLogsDirect(logs []*Log) error {
	for _, log := range logs {
		fillLogDerivedFields(log)
	}
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&logs).Error
	})
	if err != nil {
		return err
	}
	if err := queueLogAggregationEventsForLogs(logs); err != nil {
		common.SysError("failed to queue log aggregation events: " + err.Error())
		requestMissingLogAggregationReplay()
	}
	return nil
}

var consumeLogCoalescer = newConsumeLogCoalescer(consumeLogCoalesceDelay)

func FlushConsumeLogUpdates() {
	consumeLogCoalescer.drain()
}

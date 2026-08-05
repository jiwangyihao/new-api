package model

import (
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const subscriptionTokenDeltaCoalesceDelay = time.Millisecond

type subscriptionTokenDeltaCoalescerState struct {
	mu                  sync.Mutex
	groups              map[int]*subscriptionTokenDeltaGroup
	requestTargetGroups map[int]*subscriptionRequestTargetGroup
	flushDelay          time.Duration
}

type subscriptionTokenDeltaGroup struct {
	requests []*subscriptionTokenDeltaRequest
	done     chan struct{}
}

type subscriptionTokenDeltaRequest struct {
	delta int64
	done  chan error
}

type subscriptionRequestTargetGroup struct {
	requests []*subscriptionRequestTarget
	done     chan struct{}
}

type subscriptionRequestTarget struct {
	requestId              string
	originalSubscriptionId int
	targetCredit           int64
	final                  bool
	done                   chan error
}

func newSubscriptionTokenDeltaCoalescer(delay time.Duration) *subscriptionTokenDeltaCoalescerState {
	return &subscriptionTokenDeltaCoalescerState{
		groups:              make(map[int]*subscriptionTokenDeltaGroup),
		requestTargetGroups: make(map[int]*subscriptionRequestTargetGroup),
		flushDelay:          delay,
	}
}

func (c *subscriptionTokenDeltaCoalescerState) add(id int, delta int64) error {
	if c == nil {
		return postConsumeUserSubscriptionTokenDeltaDirect(id, delta)
	}
	request := &subscriptionTokenDeltaRequest{delta: delta, done: make(chan error, 1)}
	c.mu.Lock()
	group := c.groups[id]
	start := false
	if group == nil {
		group = &subscriptionTokenDeltaGroup{done: make(chan struct{})}
		c.groups[id] = group
		start = true
	}
	group.requests = append(group.requests, request)
	c.mu.Unlock()

	if start {
		go c.run(id, group)
	}
	return <-request.done
}

func (c *subscriptionTokenDeltaCoalescerState) addRequestTarget(requestId string, originalSubscriptionId int, targetCredit int64, final bool) error {
	if c == nil {
		return settleUserSubscriptionRequestTargetDirect(requestId, originalSubscriptionId, targetCredit, final)
	}
	request := &subscriptionRequestTarget{
		requestId:              requestId,
		originalSubscriptionId: originalSubscriptionId,
		targetCredit:           targetCredit,
		final:                  final,
		done:                   make(chan error, 1),
	}
	c.mu.Lock()
	group := c.requestTargetGroups[originalSubscriptionId]
	start := false
	if group == nil {
		group = &subscriptionRequestTargetGroup{done: make(chan struct{})}
		c.requestTargetGroups[originalSubscriptionId] = group
		start = true
	}
	group.requests = append(group.requests, request)
	c.mu.Unlock()

	if start {
		go c.runRequestTargets(originalSubscriptionId, group)
	}
	return <-request.done
}

func (c *subscriptionTokenDeltaCoalescerState) drain() {
	for {
		c.mu.Lock()
		doneChans := make([]chan struct{}, 0, len(c.groups)+len(c.requestTargetGroups))
		for _, group := range c.groups {
			doneChans = append(doneChans, group.done)
		}
		for _, group := range c.requestTargetGroups {
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

func (c *subscriptionTokenDeltaCoalescerState) run(id int, group *subscriptionTokenDeltaGroup) {
	if c.flushDelay > 0 {
		time.Sleep(c.flushDelay)
	}
	for {
		c.mu.Lock()
		requests := group.requests
		group.requests = nil
		c.mu.Unlock()

		flushSubscriptionTokenDeltaRequests(id, requests)

		c.mu.Lock()
		if len(group.requests) == 0 {
			delete(c.groups, id)
			close(group.done)
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

func (c *subscriptionTokenDeltaCoalescerState) runRequestTargets(originalSubscriptionId int, group *subscriptionRequestTargetGroup) {
	if c.flushDelay > 0 {
		time.Sleep(c.flushDelay)
	}
	for {
		c.mu.Lock()
		requests := group.requests
		group.requests = nil
		c.mu.Unlock()

		results := flushSubscriptionRequestTargets(requests)
		for index, request := range requests {
			request.done <- results[index]
		}

		c.mu.Lock()
		if len(group.requests) == 0 {
			delete(c.requestTargetGroups, originalSubscriptionId)
			close(group.done)
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

func flushSubscriptionRequestTargets(requests []*subscriptionRequestTarget) []error {
	results := make([]error, len(requests))
	if len(requests) == 0 {
		return results
	}
	failureIndex := -1
	err := DB.Transaction(func(tx *gorm.DB) error {
		for index, request := range requests {
			var route SubscriptionPreConsumeRecord
			if err := tx.Where("request_id = ?", request.requestId).First(&route).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					err = ErrCreditValuationRequestNotFound
				}
				failureIndex = index
				results[index] = err
				return err
			}
			if route.UserSubscriptionId != request.originalSubscriptionId {
				failureIndex = index
				results[index] = ErrCreditValuationMappingConflict
				return ErrCreditValuationMappingConflict
			}
			if err := SettleCreditRequestTargetTx(tx, &route, request.targetCredit, request.final); err != nil {
				failureIndex = index
				results[index] = err
				return err
			}
		}
		return nil
	})
	if err == nil {
		return results
	}
	for index := range results {
		if index != failureIndex {
			results[index] = ErrCreditValuationBatchRolledBack
		}
	}
	return results
}

func flushSubscriptionTokenDeltaRequests(id int, requests []*subscriptionTokenDeltaRequest) {
	if len(requests) == 0 {
		return
	}
	var total int64
	for _, request := range requests {
		total += request.delta
	}
	err := postConsumeUserSubscriptionTokenDeltaDirect(id, total)
	if err == nil {
		for _, request := range requests {
			request.done <- nil
		}
		return
	}
	if !strings.Contains(err.Error(), "exceeds limit") {
		for _, request := range requests {
			request.done <- err
		}
		return
	}
	for _, request := range requests {
		request.done <- postConsumeUserSubscriptionTokenDeltaDirect(id, request.delta)
	}
}

var subscriptionTokenDeltaCoalescer = newSubscriptionTokenDeltaCoalescer(subscriptionTokenDeltaCoalesceDelay)

func FlushSubscriptionTokenDeltaUpdates() {
	subscriptionTokenDeltaCoalescer.drain()
}

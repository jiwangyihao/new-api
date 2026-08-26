package model

import "sync"

type effectiveGroupRowsCall struct {
	done       chan struct{}
	rows       []effectiveGroupForChannelRow
	err        error
	generation uint64
}

type effectiveGroupRowsCache struct {
	mu         sync.Mutex
	generation uint64
	items      map[int][]effectiveGroupForChannelRow
	inFlight   map[int]*effectiveGroupRowsCall
}

func newEffectiveGroupRowsCache() *effectiveGroupRowsCache {
	return &effectiveGroupRowsCache{
		items:    make(map[int][]effectiveGroupForChannelRow),
		inFlight: make(map[int]*effectiveGroupRowsCall),
	}
}

func cloneEffectiveGroupRows(rows []effectiveGroupForChannelRow) []effectiveGroupForChannelRow {
	return append([]effectiveGroupForChannelRow(nil), rows...)
}

func (cache *effectiveGroupRowsCache) get(channelID int, load func(int) ([]effectiveGroupForChannelRow, error)) ([]effectiveGroupForChannelRow, error) {
	cache.mu.Lock()
	if rows, ok := cache.items[channelID]; ok {
		rows = cloneEffectiveGroupRows(rows)
		cache.mu.Unlock()
		return rows, nil
	}
	if call, ok := cache.inFlight[channelID]; ok {
		cache.mu.Unlock()
		<-call.done
		return cloneEffectiveGroupRows(call.rows), call.err
	}
	call := &effectiveGroupRowsCall{done: make(chan struct{}), generation: cache.generation}
	cache.inFlight[channelID] = call
	cache.mu.Unlock()

	call.rows, call.err = load(channelID)

	cache.mu.Lock()
	if cache.inFlight[channelID] == call {
		delete(cache.inFlight, channelID)
	}
	if call.err == nil && call.generation == cache.generation {
		cache.items[channelID] = cloneEffectiveGroupRows(call.rows)
	}
	close(call.done)
	cache.mu.Unlock()
	return cloneEffectiveGroupRows(call.rows), call.err
}

func (cache *effectiveGroupRowsCache) flush() {
	cache.mu.Lock()
	cache.generation++
	cache.items = make(map[int][]effectiveGroupForChannelRow)
	cache.inFlight = make(map[int]*effectiveGroupRowsCall)
	cache.mu.Unlock()
}

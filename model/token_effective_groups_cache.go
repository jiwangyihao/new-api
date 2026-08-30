package model

import "sync"

type tokenEffectiveGroupsCall struct {
	done       chan struct{}
	groups     []string
	err        error
	generation uint64
}

type tokenEffectiveGroupsCache struct {
	mu         sync.Mutex
	generation uint64
	items      map[int][]string
	inFlight   map[int]*tokenEffectiveGroupsCall
}

func newTokenEffectiveGroupsCache() *tokenEffectiveGroupsCache {
	return &tokenEffectiveGroupsCache{
		items:    make(map[int][]string),
		inFlight: make(map[int]*tokenEffectiveGroupsCall),
	}
}

func cloneTokenEffectiveGroups(groups []string) []string {
	return append([]string(nil), groups...)
}

func (cache *tokenEffectiveGroupsCache) get(tokenID int, load func(int) ([]string, error)) ([]string, error) {
	cache.mu.Lock()
	if groups, ok := cache.items[tokenID]; ok {
		groups = cloneTokenEffectiveGroups(groups)
		cache.mu.Unlock()
		return groups, nil
	}
	if call, ok := cache.inFlight[tokenID]; ok {
		cache.mu.Unlock()
		<-call.done
		return cloneTokenEffectiveGroups(call.groups), call.err
	}
	call := &tokenEffectiveGroupsCall{done: make(chan struct{}), generation: cache.generation}
	cache.inFlight[tokenID] = call
	cache.mu.Unlock()

	call.groups, call.err = load(tokenID)

	cache.mu.Lock()
	if cache.inFlight[tokenID] == call {
		delete(cache.inFlight, tokenID)
	}
	if call.err == nil && call.generation == cache.generation {
		cache.items[tokenID] = cloneTokenEffectiveGroups(call.groups)
	}
	close(call.done)
	cache.mu.Unlock()
	return cloneTokenEffectiveGroups(call.groups), call.err
}

func (cache *tokenEffectiveGroupsCache) delete(tokenID int) {
	cache.mu.Lock()
	delete(cache.items, tokenID)
	if call, ok := cache.inFlight[tokenID]; ok {
		delete(cache.inFlight, tokenID)
		call.generation = ^uint64(0)
	}
	cache.mu.Unlock()
}

func (cache *tokenEffectiveGroupsCache) flush() {
	cache.mu.Lock()
	cache.generation++
	cache.items = make(map[int][]string)
	cache.inFlight = make(map[int]*tokenEffectiveGroupsCall)
	cache.mu.Unlock()
}

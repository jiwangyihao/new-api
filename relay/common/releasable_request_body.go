package common

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"

	appcommon "github.com/QuantumNous/new-api/common"
)

var errReleasableRequestBodyReleased = errors.New("request body has been released")

var inFlightReplayMemoryBytes atomic.Int64

func InFlightReplayMemoryBytes() int64 {
	return inFlightReplayMemoryBytes.Load()
}

// ReleasableRequestBody keeps the upstream request payload replayable while a
// request is being sent, then drops the backing byte slice after the response
// headers arrive and every transport reader has closed.
type ReleasableRequestBody struct {
	mu               sync.RWMutex
	data             []byte
	accountedBytes   int64
	readers          int
	releaseRequested bool
}

func NewReleasableRequestBody(data []byte) *ReleasableRequestBody {
	size := int64(len(data))
	inFlightReplayMemoryBytes.Add(size)
	return &ReleasableRequestBody{data: data, accountedBytes: size}
}

func newReservedReleasableRequestBody(data []byte) *ReleasableRequestBody {
	return &ReleasableRequestBody{data: data, accountedBytes: int64(len(data))}
}

func tryReserveReplayMemory(size, limit int64) bool {
	if size < 0 || limit <= 0 {
		return false
	}
	for {
		current := inFlightReplayMemoryBytes.Load()
		if size > limit-current {
			return false
		}
		if inFlightReplayMemoryBytes.CompareAndSwap(current, current+size) {
			return true
		}
	}
}

func newDiskReplayReader(data []byte) (ReplayableRequestBodyReader, bool) {
	body, err := NewDiskReleasableRequestBody(data)
	if err != nil {
		return nil, false
	}
	reader, err := body.Reader()
	if err != nil {
		body.Release()
		return nil, false
	}
	return reader, true
}

// NewAdaptiveReplayableRequestBody keeps small aggregate replay payloads in
// memory and spills to disk once either the per-request threshold or the same
// aggregate in-flight memory budget is exceeded. Disk failures safely fall
// back to the in-memory owner.
func NewAdaptiveReplayableRequestBody(data []byte) ReplayableRequestBodyReader {
	size := int64(len(data))
	if appcommon.ShouldUseDiskCache(size) {
		if reader, ok := newDiskReplayReader(data); ok {
			return reader
		}
		return NewReleasableRequestBody(data).Reader()
	}
	if appcommon.IsDiskCacheEnabled() {
		memoryBudget := appcommon.GetDiskCacheThresholdBytes()
		if memoryBudget > 0 {
			if tryReserveReplayMemory(size, memoryBudget) {
				return newReservedReleasableRequestBody(data).Reader()
			}
			if appcommon.IsDiskCacheAvailable(size) {
				if reader, ok := newDiskReplayReader(data); ok {
					return reader
				}
			}
		}
	}
	return NewReleasableRequestBody(data).Reader()
}

func (b *ReleasableRequestBody) releaseDataLocked() {
	if b.data == nil {
		return
	}
	b.data = nil
	if b.accountedBytes != 0 {
		inFlightReplayMemoryBytes.Add(-b.accountedBytes)
		b.accountedBytes = 0
	}
}

func (b *ReleasableRequestBody) Reader() *ReleasableRequestBodyReader {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.releaseRequested || b.data == nil {
		return &ReleasableRequestBodyReader{owner: b, closed: true}
	}
	b.readers++
	return &ReleasableRequestBodyReader{owner: b}
}

func (b *ReleasableRequestBody) Release() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.releaseRequested = true
	if b.readers == 0 {
		b.releaseDataLocked()
	}
	b.mu.Unlock()
}

func (b *ReleasableRequestBody) closeReader() {
	b.mu.Lock()
	if b.readers > 0 {
		b.readers--
	}
	if b.releaseRequested && b.readers == 0 {
		b.releaseDataLocked()
	}
	b.mu.Unlock()
}

// ReleasableRequestBodyReader provides replay metadata to net/http without
// placing the payload directly inside bytes.Reader/GetBody closures.
type ReleasableRequestBodyReader struct {
	owner  *ReleasableRequestBody
	offset int64
	mu     sync.Mutex
	closed bool
}

func (r *ReleasableRequestBodyReader) Read(p []byte) (int, error) {
	if r == nil || r.owner == nil {
		return 0, io.EOF
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.EOF
	}
	r.owner.mu.RLock()
	defer r.owner.mu.RUnlock()
	if r.offset >= int64(len(r.owner.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.owner.data[r.offset:])
	r.offset += int64(n)
	return n, nil
}

func (r *ReleasableRequestBodyReader) Close() error {
	if r == nil || r.owner == nil {
		return nil
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	r.owner.closeReader()
	return nil
}

func (r *ReleasableRequestBodyReader) ContentLength() int64 {
	if r == nil || r.owner == nil {
		return 0
	}
	r.owner.mu.RLock()
	defer r.owner.mu.RUnlock()
	return int64(len(r.owner.data))
}

func (r *ReleasableRequestBodyReader) GetBody() (io.ReadCloser, error) {
	if r == nil || r.owner == nil {
		return nil, errReleasableRequestBodyReleased
	}
	r.owner.mu.Lock()
	defer r.owner.mu.Unlock()
	if r.owner.releaseRequested || r.owner.data == nil {
		return nil, errReleasableRequestBodyReleased
	}
	r.owner.readers++
	return &ReleasableRequestBodyReader{owner: r.owner}, nil
}

func (r *ReleasableRequestBodyReader) Release() {
	if r != nil && r.owner != nil {
		r.owner.Release()
	}
}

// ReplayableRequestBodyReader is the small interface required by net/http
// request construction and response-header release.
type ReplayableRequestBodyReader interface {
	io.ReadCloser
	ContentLength() int64
	GetBody() (io.ReadCloser, error)
	Release()
}

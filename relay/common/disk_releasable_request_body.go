package common

import (
	"bufio"
	"io"
	"os"
	"sync"

	appcommon "github.com/QuantumNous/new-api/common"
)

// DiskReleasableRequestBody keeps a large upstream payload replayable on disk
// while net/http may need GetBody, then deletes it after Release and the last
// transport reader closes.
type DiskReleasableRequestBody struct {
	mu               sync.Mutex
	path             string
	size             int64
	readers          int
	releaseRequested bool
}

type DiskReleasableRequestBodyReader struct {
	owner  *DiskReleasableRequestBody
	file   *os.File
	closed bool
	mu     sync.Mutex
}

func NewDiskReleasableRequestBody(data []byte) (*DiskReleasableRequestBody, error) {
	path, err := appcommon.WriteDiskCacheFile(appcommon.DiskCacheTypeBody, data)
	if err != nil {
		return nil, err
	}
	appcommon.IncrementDiskFiles(int64(len(data)))
	return &DiskReleasableRequestBody{path: path, size: int64(len(data))}, nil
}

func NewDiskReleasableRequestBodyFromJSON(value any) (*DiskReleasableRequestBody, error) {
	path, file, err := appcommon.CreateDiskCacheFile(appcommon.DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(path)
	}
	if streaming, ok := value.(interface{ WriteJSON(io.Writer) error }); ok {
		buffered := bufio.NewWriter(file)
		if err := streaming.WriteJSON(buffered); err != nil {
			cleanup()
			return nil, err
		}
		if err := buffered.Flush(); err != nil {
			cleanup()
			return nil, err
		}
	} else if err := appcommon.EncodeJson(file, value); err != nil {
		cleanup()
		return nil, err
	}
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		cleanup()
		return nil, err
	}
	if size > 0 {
		var last [1]byte
		if _, err := file.ReadAt(last[:], size-1); err != nil {
			cleanup()
			return nil, err
		}
		if last[0] == '\n' {
			size--
			if err := file.Truncate(size); err != nil {
				cleanup()
				return nil, err
			}
		}
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	appcommon.IncrementDiskFiles(size)
	return &DiskReleasableRequestBody{path: path, size: size}, nil
}

func (b *DiskReleasableRequestBody) Reader() (*DiskReleasableRequestBodyReader, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.releaseRequested || b.path == "" {
		return nil, errReleasableRequestBodyReleased
	}
	file, err := os.Open(b.path)
	if err != nil {
		return nil, err
	}
	b.readers++
	return &DiskReleasableRequestBodyReader{owner: b, file: file}, nil
}

func (b *DiskReleasableRequestBody) releaseLocked() {
	if b.readers != 0 || b.path == "" {
		return
	}
	path := b.path
	size := b.size
	b.path = ""
	b.size = 0
	_ = os.Remove(path)
	appcommon.DecrementDiskFiles(size)
}

func (b *DiskReleasableRequestBody) Release() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.releaseRequested = true
	b.releaseLocked()
	b.mu.Unlock()
}

func (r *DiskReleasableRequestBodyReader) Read(p []byte) (int, error) {
	if r == nil || r.file == nil {
		return 0, io.EOF
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.EOF
	}
	return r.file.Read(p)
}

func (r *DiskReleasableRequestBodyReader) WriteTo(w io.Writer) (int64, error) {
	if r == nil || r.file == nil {
		return 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.file == nil {
		return 0, nil
	}
	return r.file.WriteTo(w)
}

func (r *DiskReleasableRequestBodyReader) Close() error {
	if r == nil || r.owner == nil {
		return nil
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	file := r.file
	r.file = nil
	r.mu.Unlock()
	if file != nil {
		_ = file.Close()
	}
	r.owner.mu.Lock()
	if r.owner.readers > 0 {
		r.owner.readers--
	}
	if r.owner.releaseRequested {
		r.owner.releaseLocked()
	}
	r.owner.mu.Unlock()
	return nil
}

func (r *DiskReleasableRequestBodyReader) ContentLength() int64 {
	if r == nil || r.owner == nil {
		return 0
	}
	r.owner.mu.Lock()
	defer r.owner.mu.Unlock()
	return r.owner.size
}

func (r *DiskReleasableRequestBodyReader) GetBody() (io.ReadCloser, error) {
	if r == nil || r.owner == nil {
		return nil, errReleasableRequestBodyReleased
	}
	return r.owner.Reader()
}

func (r *DiskReleasableRequestBodyReader) Release() {
	if r != nil && r.owner != nil {
		r.owner.Release()
	}
}

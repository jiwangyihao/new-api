package common

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// BodyStorage 请求体存储接口
type BodyStorage interface {
	io.ReadSeeker
	io.Closer
	// Bytes 获取全部内容
	Bytes() ([]byte, error)
	// Size 获取数据大小
	Size() int64
	// IsDisk 是否是磁盘存储
	IsDisk() bool
}

// ErrStorageClosed 存储已关闭错误
var ErrStorageClosed = fmt.Errorf("body storage is closed")

// memoryStorage 内存存储实现
type memoryStorage struct {
	data   []byte
	reader *bytes.Reader
	size   int64
	closed int32
	mu     sync.Mutex
}

func newMemoryStorage(data []byte) *memoryStorage {
	size := int64(len(data))
	IncrementMemoryBuffers(size)
	return &memoryStorage{
		data:   data,
		reader: bytes.NewReader(data),
		size:   size,
	}
}

func (m *memoryStorage) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Read(p)
}

func (m *memoryStorage) Seek(offset int64, whence int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Seek(offset, whence)
}

func (m *memoryStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		DecrementMemoryBuffers(m.size)
		m.data = nil
		m.reader = nil
		m.size = 0
	}
	return nil
}

func (m *memoryStorage) Bytes() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, ErrStorageClosed
	}
	return m.data, nil
}

func (m *memoryStorage) Size() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.size
}

func (m *memoryStorage) IsDisk() bool {
	return false
}

// diskStorage 磁盘存储实现
type diskStorage struct {
	file     *os.File
	filePath string
	size     int64
	closed   int32
	mu       sync.Mutex
}

func newDiskStorage(data []byte, cachePath string) (*diskStorage, error) {
	// 使用统一的缓存目录管理
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}

	// 写入数据
	n, err := file.Write(data)
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	// 重置文件指针
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	size := int64(n)
	IncrementDiskFiles(size)

	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     size,
	}, nil
}

func newDiskStorageFromReader(reader io.Reader, maxBytes int64, cachePath string) (*diskStorage, error) {
	// 使用统一的缓存目录管理
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}

	// 从 reader 读取并写入文件
	written, err := io.Copy(file, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	if written > maxBytes {
		file.Close()
		os.Remove(filePath)
		return nil, ErrRequestBodyTooLarge
	}

	// 重置文件指针
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	IncrementDiskFiles(written)

	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     written,
	}, nil
}

func (d *diskStorage) Read(p []byte) (n int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Read(p)
}

func (d *diskStorage) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Seek(offset, whence)
}

func (d *diskStorage) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.CompareAndSwapInt32(&d.closed, 0, 1) {
		file := d.file
		filePath := d.filePath
		size := d.size
		d.file = nil
		d.filePath = ""
		d.size = 0
		if file != nil {
			_ = file.Close()
		}
		if filePath != "" {
			_ = os.Remove(filePath)
		}
		DecrementDiskFiles(size)
	}
	return nil
}

func (d *diskStorage) Bytes() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if atomic.LoadInt32(&d.closed) == 1 {
		return nil, ErrStorageClosed
	}

	// 保存当前位置
	currentPos, err := d.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// 移动到开头
	if _, err := d.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	// 读取全部内容
	data := make([]byte, d.size)
	_, err = io.ReadFull(d.file, data)
	if err != nil {
		return nil, err
	}

	// 恢复位置
	if _, err := d.file.Seek(currentPos, io.SeekStart); err != nil {
		return nil, err
	}

	return data, nil
}

func (d *diskStorage) Size() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.size
}

func (d *diskStorage) IsDisk() bool {
	return true
}

// CreateBodyStorage 根据数据大小创建合适的存储
func CreateBodyStorage(data []byte) (BodyStorage, error) {
	size := int64(len(data))
	threshold := GetDiskCacheThresholdBytes()

	// 检查是否应该使用磁盘缓存
	if IsDiskCacheEnabled() &&
		size >= threshold &&
		IsDiskCacheAvailable(size) {
		storage, err := newDiskStorage(data, GetDiskCachePath())
		if err != nil {
			// 如果磁盘存储失败，回退到内存存储
			SysError(fmt.Sprintf("failed to create disk storage, falling back to memory: %v", err))
			return newMemoryStorage(data), nil
		}
		return storage, nil
	}

	return newMemoryStorage(data), nil
}

// CreateBodyStorageFromReader 从 Reader 创建存储（用于大请求的流式处理）
func CreateBodyStorageFromReader(reader io.Reader, contentLength int64, maxBytes int64) (BodyStorage, error) {
	threshold := GetDiskCacheThresholdBytes()
	if IsDiskCacheEnabled() && contentLength > 0 && contentLength >= threshold && IsDiskCacheAvailable(contentLength) {
		storage, err := newDiskStorageFromReader(reader, maxBytes, GetDiskCachePath())
		if err != nil {
			if IsRequestBodyTooLargeError(err) {
				return nil, err
			}
			return nil, fmt.Errorf("disk storage creation failed: %w", err)
		}
		IncrementDiskCacheHits()
		return storage, nil
	}

	if IsDiskCacheEnabled() && contentLength <= 0 && threshold > 0 && threshold <= maxBytes && IsDiskCacheAvailable(threshold) {
		storage, smallBody, err := newProgressiveDiskStorageFromReader(reader, threshold, maxBytes)
		if err != nil {
			return nil, err
		}
		if storage != nil {
			IncrementDiskCacheHits()
			return storage, nil
		}
		IncrementMemoryCacheHits()
		return newMemoryStorage(smallBody), nil
	}

	data, err := readBodyStorageBytes(reader, contentLength, maxBytes)
	if err != nil {
		return nil, err
	}
	storage, err := CreateBodyStorage(data)
	if err != nil {
		return nil, err
	}
	if storage.IsDisk() {
		IncrementDiskCacheHits()
	} else {
		IncrementMemoryCacheHits()
	}
	return storage, nil
}

func newProgressiveDiskStorageFromReader(reader io.Reader, threshold, maxBytes int64) (*diskStorage, []byte, error) {
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*diskStorage, []byte, error) {
		_ = file.Close()
		_ = os.Remove(filePath)
		return nil, nil, err
	}

	written, err := io.CopyN(file, reader, threshold)
	if err != nil && err != io.EOF {
		return fail(err)
	}
	if written < threshold {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return fail(err)
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return fail(err)
		}
		_ = file.Close()
		_ = os.Remove(filePath)
		return nil, data, nil
	}

	if _, err := io.Copy(file, io.LimitReader(reader, maxBytes-threshold+1)); err != nil {
		return fail(err)
	}
	stat, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if stat.Size() > maxBytes {
		return fail(ErrRequestBodyTooLarge)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	IncrementDiskFiles(stat.Size())
	return &diskStorage{file: file, filePath: filePath, size: stat.Size()}, nil, nil
}

func readBodyStorageBytes(reader io.Reader, contentLength, maxBytes int64) ([]byte, error) {
	if contentLength <= 0 || contentLength > maxBytes || contentLength > int64(int(^uint(0)>>1))-1 {
		data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > maxBytes {
			return nil, ErrRequestBodyTooLarge
		}
		return data, nil
	}

	data := make([]byte, int(contentLength), int(contentLength)+1)
	n, err := io.ReadFull(reader, data)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return data[:n], nil
		}
		return nil, err
	}

	var extra [1]byte
	n, err = io.ReadFull(reader, extra[:])
	if err == io.EOF {
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	data = append(data, extra[:n]...)
	if int64(len(data)) > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}

	rest, err := io.ReadAll(io.LimitReader(reader, maxBytes-int64(len(data))+1))
	if err != nil {
		return nil, err
	}
	data = append(data, rest...)
	if int64(len(data)) > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}
	return data, nil
}

// ReaderOnly wraps an io.Reader to hide io.Closer, preventing http.NewRequest
// from type-asserting io.ReadCloser and closing the underlying BodyStorage.
func ReaderOnly(r io.Reader) io.Reader {
	return struct{ io.Reader }{r}
}

// CleanupOldCacheFiles 清理旧的缓存文件（用于启动时清理残留）
func CleanupOldCacheFiles() {
	// 使用统一的缓存管理
	CleanupOldDiskCacheFiles(5 * time.Minute)
}

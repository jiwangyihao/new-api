package helper

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 8 << 10  // 8KB; grows on demand for larger SSE lines
	DefaultMaxScannerBufferSize = 64 << 20 // 64MB (64*1024*1024) default SSE buffer size
	DefaultPingInterval         = 10 * time.Second
)

type streamScannerItem struct {
	data    string
	payload []byte
	holder  *[]byte
	done    bool
}

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

const maxPooledStreamPayloadBytes = 16 << 10

var streamPayloadPool = sync.Pool{
	New: func() any {
		payload := make([]byte, 0)
		return &payload
	},
}

func acquireStreamPayload(size int) (*[]byte, []byte) {
	holder := streamPayloadPool.Get().(*[]byte)
	payload := (*holder)[:0]
	if cap(payload) < size {
		payload = make([]byte, 0, size)
	}
	return holder, payload
}

func releaseStreamPayload(holder *[]byte, payload []byte) {
	if holder == nil {
		return
	}
	if cap(payload) > maxPooledStreamPayloadBytes {
		*holder = nil
	} else {
		*holder = payload[:0]
	}
	streamPayloadPool.Put(holder)
}

// StreamScannerHandler preserves the historical string callback contract.
// Callbacks may retain data after returning.
func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string, sr *StreamResult)) {
	if dataHandler == nil {
		return
	}
	streamScannerHandler(c, resp, info, dataHandler, nil)
}

// StreamScannerBytesHandler avoids the per-event string copy for Responses.
// data is immutable and valid only during dataHandler; callers must copy it to retain.
func StreamScannerBytesHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data []byte, sr *StreamResult)) {
	if dataHandler == nil {
		return
	}
	streamScannerHandler(c, resp, info, nil, dataHandler)
}

func streamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, stringHandler func(string, *StreamResult), bytesHandler func([]byte, *StreamResult)) {

	if resp == nil || (stringHandler == nil && bytesHandler == nil) {
		return
	}

	// 无条件保证 StreamStatus 存在；保留调用方提前记录的状态。
	if info.StreamStatus == nil {
		info.StreamStatus = relaycommon.NewStreamStatus()
	}

	// 确保响应体总是被关闭
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	if streamingTimeout <= 0 {
		streamingTimeout = time.Duration(common.RelayTimeout) * time.Second
		if streamingTimeout <= 0 {
			streamingTimeout = DefaultPingInterval
		}
	}

	var (
		stopChan   = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanner    = bufio.NewScanner(resp.Body)
		ticker     = time.NewTicker(streamingTimeout)
		pingTicker *time.Ticker
		writeMutex sync.Mutex     // Mutex to protect concurrent writes
		wg         sync.WaitGroup // 用于等待所有 goroutine 退出
	)

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	if common.DebugEnabled {
		// print timeout and ping interval for debugging
		println("relay timeout seconds:", common.RelayTimeout)
		println("relay max idle conns:", common.RelayMaxIdleConns)
		println("relay max idle conns per host:", common.RelayMaxIdleConnsPerHost)
		println("streaming timeout seconds:", int64(streamingTimeout.Seconds()))
		println("ping interval seconds:", int64(pingInterval.Seconds()))
	}

	// 改进资源清理，确保所有 goroutine 正确退出
	defer func() {
		// 通知所有 goroutine 停止
		common.SafeSendBool(stopChan, true)

		ticker.Stop()
		if pingTicker != nil {
			pingTicker.Stop()
		}

		// 等待所有 goroutine 退出，最多等待5秒
		done := make(chan struct{})
		gopool.Go(func() {
			wg.Wait()
			close(done)
		})

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			logger.LogError(c, "timeout waiting for goroutines to exit")
		}

		close(stopChan)
	}()

	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	scanner.Split(bufio.ScanLines)
	SetEventStreamHeaders(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					logger.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("ping panic: %v", r))
					common.SafeSendBool(stopChan, true)
				}
				if common.DebugEnabled {
					println("ping goroutine exited")
				}
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					// 使用超时机制防止写操作阻塞
					done := make(chan error, 1)
					gopool.Go(func() {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						done <- PingData(c)
					})

					select {
					case err := <-done:
						if err != nil {
							logger.LogError(c, "ping data error: "+err.Error())
							info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
							return
						}
						if common.DebugEnabled {
							println("ping data sent")
						}
					case <-time.After(10 * time.Second):
						logger.LogError(c, "ping data send timeout")
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, fmt.Errorf("ping send timeout"))
						return
					case <-ctx.Done():
						return
					case <-stopChan:
						return
					}
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					logger.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	dataChan := make(chan streamScannerItem, 128)

	wg.Add(1)
	gopool.Go(func() {
		defer func() {
			cancel()
			common.SafeSendBool(stopChan, true)
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			for item := range dataChan {
				releaseStreamPayload(item.holder, item.payload)
			}
			wg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("data handler goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", r))
			}
		}()
		sr := newStreamResult(info.StreamStatus)
		for item := range dataChan {
			if item.done {
				writeMutex.Lock()
				err := Done(c)
				writeMutex.Unlock()
				if err != nil {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
					return
				}
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				continue
			}
			sr.reset()
			writeMutex.Lock()
			func() {
				defer releaseStreamPayload(item.holder, item.payload)
				if bytesHandler != nil {
					bytesHandler(item.payload, sr)
				} else {
					stringHandler(item.data, sr)
				}
			}()
			writeMutex.Unlock()
			if sr.IsStopped() {
				return
			}
		}
	})

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			close(dataChan)
			wg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("scanner panic: %v", r))
			}
			cancel()
			common.SafeSendBool(stopChan, true)
			if common.DebugEnabled {
				println("scanner goroutine exited")
			}
		}()

		sawDone := false
		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			case <-c.Request.Context().Done():
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
				return
			default:
			}

			ticker.Reset(streamingTimeout)
			line := scanner.Bytes()
			if common.DebugEnabled {
				println(string(line))
			}

			if sawDone {
				continue
			}
			if len(line) < len("data:") || !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(line[len("data:"):])
			if len(payload) == 0 {
				continue
			}
			if !bytes.HasPrefix(payload, []byte("[DONE]")) {
				info.SetFirstResponseTime()
				info.ReceivedResponseCount++
				item := streamScannerItem{}
				if bytesHandler != nil {
					holder, owned := acquireStreamPayload(len(payload))
					owned = append(owned, payload...)
					item.payload = owned
					item.holder = holder
				} else {
					item.data = string(payload)
				}
				select {
				case dataChan <- item:
				case <-ctx.Done():
					releaseStreamPayload(item.holder, item.payload)
					return
				case <-stopChan:
					releaseStreamPayload(item.holder, item.payload)
					return
				}
			} else {
				select {
				case dataChan <- streamScannerItem{done: true}:
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
				sawDone = true
				if common.DebugEnabled {
					println("received [DONE], draining stream for trailers")
				}
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
				return
			}
		}
		info.StreamStatus.FinalizeEOF()
	})

	// 主循环等待完成或超时
	select {
	case <-ticker.C:
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, fmt.Errorf("stream timeout after %s", streamingTimeout))
	case <-stopChan:
		// EndReason already set by the goroutine that triggered stopChan
	case <-ctx.Done():
		// EndReason already set by the goroutine that cancelled the stream.
	case <-c.Request.Context().Done():
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
	}

	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, fmt.Sprintf("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
}

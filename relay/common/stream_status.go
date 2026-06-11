package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type StreamEndReason string

const (
	StreamEndReasonNone        StreamEndReason = ""
	StreamEndReasonDone        StreamEndReason = "done"
	StreamEndReasonTimeout     StreamEndReason = "timeout"
	StreamEndReasonClientGone  StreamEndReason = "client_gone"
	StreamEndReasonScannerErr  StreamEndReason = "scanner_error"
	StreamEndReasonHandlerStop StreamEndReason = "handler_stop"
	StreamEndReasonEOF         StreamEndReason = "eof"
	StreamEndReasonPanic       StreamEndReason = "panic"
	StreamEndReasonPingFail    StreamEndReason = "ping_fail"
)

const maxStreamErrorEntries = 20

type StreamErrorEntry struct {
	Message   string
	Timestamp time.Time
}

type StreamStatus struct {
	mu sync.Mutex

	EndReason    StreamEndReason
	EndError     error
	Completed    bool
	DrainedToEOF bool

	Errors     []StreamErrorEntry
	ErrorCount int
}

func NewStreamStatus() *StreamStatus {
	return &StreamStatus{}
}

func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.EndReason != StreamEndReasonNone {
		if s.EndReason == StreamEndReasonEOF && reason == StreamEndReasonHandlerStop {
			if err != nil {
				s.recordErrorLocked(err.Error())
			}
			s.EndReason = reason
			s.EndError = err
			return
		}
		if reason == StreamEndReasonDone {
			s.Completed = true
			if s.EndReason == StreamEndReasonEOF {
				s.EndReason = reason
				s.EndError = err
			}
			return
		}
		if err != nil && s.EndReason == StreamEndReasonDone {
			s.recordErrorLocked(err.Error())
		}
		return
	}
	if err != nil {
		s.recordErrorLocked(err.Error())
	}
	if reason == StreamEndReasonDone {
		s.Completed = true
	}
	s.EndReason = reason
	s.EndError = err
}

// FinalizeEOF records transport EOF only when no protocol-level or error end
// reason has already been observed. Because scanning and handler processing are
// asynchronous, SetEndReason still permits a later protocol completion to
// upgrade EOF to done.
func (s *StreamStatus) FinalizeEOF() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.DrainedToEOF = true
	if s.EndReason == StreamEndReasonNone {
		s.EndReason = StreamEndReasonEOF
	}
}

func (s *StreamStatus) RecordError(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordErrorLocked(msg)
}

func (s *StreamStatus) recordErrorLocked(msg string) {
	if msg == "" {
		return
	}
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrorEntries {
		s.Errors = append(s.Errors, StreamErrorEntry{
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
}

func (s *StreamStatus) HasErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

func (s *StreamStatus) TotalErrorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount
}

func (s *StreamStatus) IsNormalEnd() bool {
	if s == nil {
		return true
	}
	return s.EndReason == StreamEndReasonDone ||
		s.EndReason == StreamEndReasonHandlerStop
}

func (s *StreamStatus) Summary() string {
	if s == nil {
		return "StreamStatus<nil>"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "reason=%s", s.EndReason)
	if s.EndError != nil {
		fmt.Fprintf(b, " end_error=%q", s.EndError.Error())
	}
	s.mu.Lock()
	if s.ErrorCount > 0 {
		fmt.Fprintf(b, " soft_errors=%d", s.ErrorCount)
	}
	s.mu.Unlock()
	return b.String()
}

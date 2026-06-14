package service

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

type TokenLimitSession struct {
	relayInfo           *relaycommon.RelayInfo
	requestId           string
	tokenId             int
	userId              int
	enabled             bool
	preConsumed         int64
	incrementSeq        int64
	committedIncrements map[int64]bool
	settled             bool
	refunded            bool
	settleFailed        bool
	failureCode         string
	actualTokens        int64
	mu                  sync.Mutex
}

func NewTokenLimitSession(relayInfo *relaycommon.RelayInfo) *TokenLimitSession {
	s := &TokenLimitSession{relayInfo: relayInfo}
	if relayInfo == nil {
		return s
	}
	s.requestId = strings.TrimSpace(relayInfo.RequestId)
	s.tokenId = relayInfo.TokenId
	s.userId = relayInfo.UserId
	if s.requestId == "" {
		s.requestId = fmt.Sprintf("token-limit:%d:%d", s.userId, s.tokenId)
	}
	if token, err := model.GetTokenByIds(s.tokenId, s.userId); err == nil && token.TokenLimitEnabled && token.TokenLimit > 0 {
		s.enabled = true
	}
	return s
}

func newAPIKeyTokenLimitError(err error) *types.NewAPIError {
	if err == nil {
		err = errors.New("api key token limit exhausted")
	}
	return types.NewOpenAIError(err, types.ErrorCodeAPIKeyTokenLimitExhausted, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}

func isTokenLimitExceededError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *types.NewAPIError
	if errors.As(err, &apiErr) {
		return apiErr.GetErrorCode() == types.ErrorCodeAPIKeyTokenLimitExhausted
	}
	return errors.Is(err, model.ErrTokenLimitExceeded)
}

func (s *TokenLimitSession) PreConsume(tokens int64) *types.NewAPIError {
	if s == nil || !s.enabled || tokens <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded || s.preConsumed > 0 {
		return nil
	}
	ok, err := model.PreConsumeTokenLimit(s.tokenId, s.userId, s.requestId, tokens)
	if err != nil {
		return newAPIKeyTokenLimitError(err)
	}
	if !ok {
		return newAPIKeyTokenLimitError(nil)
	}
	s.preConsumed = tokens
	return nil
}

func (s *TokenLimitSession) Settle(actualTokens int64) error {
	if s == nil || !s.enabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded || s.settleFailed {
		return nil
	}
	if actualTokens < 0 {
		actualTokens = 0
	}
	err := model.SettleTokenLimitPreConsume(s.requestId, actualTokens)
	if err != nil {
		if isTokenLimitExceededError(err) {
			_ = model.RefundTokenLimitPreConsume(s.requestId, string(types.ErrorCodeAPIKeyTokenLimitExhausted))
			s.refunded = true
			return newAPIKeyTokenLimitError(err)
		}
		return err
	}
	s.actualTokens = actualTokens
	s.settled = true
	return nil
}

func (s *TokenLimitSession) SettleForAudit(actualTokens int64, reason string) error {
	if s == nil || !s.enabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded || s.settleFailed {
		return nil
	}
	if actualTokens < 0 {
		actualTokens = 0
	}
	if strings.TrimSpace(reason) == "" {
		reason = "api_key_token_limit_settle_failed"
	}
	if err := model.SettleTokenLimitPreConsume(s.requestId, actualTokens); err != nil {
		if !isTokenLimitExceededError(err) {
			return err
		}
		if markErr := model.MarkTokenLimitSettleFailed(s.requestId, actualTokens, reason); markErr != nil {
			return markErr
		}
		s.actualTokens = actualTokens
		s.failureCode = reason
		s.settleFailed = true
		return nil
	}
	s.actualTokens = actualTokens
	s.settled = true
	return nil
}

func (s *TokenLimitSession) MarkSettleFailed(actualTokens int64, reason string) error {
	if s == nil || !s.enabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refunded || s.settled || s.settleFailed {
		return nil
	}
	if actualTokens < 0 {
		actualTokens = 0
	}
	if strings.TrimSpace(reason) == "" {
		reason = "api_key_token_limit_settle_failed"
	}
	if err := model.MarkTokenLimitSettleFailed(s.requestId, actualTokens, reason); err != nil {
		return err
	}
	s.actualTokens = actualTokens
	s.failureCode = reason
	s.settleFailed = true
	return nil
}

func (s *TokenLimitSession) Refund(reason string) {
	if s == nil || !s.enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refunded || s.settleFailed || s.preConsumed <= 0 {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "request_failed"
	}
	if err := model.RefundTokenLimitPreConsume(s.requestId, reason); err != nil {
		common.SysLog("error refunding api key token limit preconsume: " + err.Error())
		return
	}
	s.refunded = true
	s.failureCode = reason
}

func (s *TokenLimitSession) ConsumeIncrement(tokens int64) (int64, *types.NewAPIError) {
	if s == nil || !s.enabled || tokens <= 0 {
		return 0, nil
	}
	s.mu.Lock()
	s.incrementSeq++
	sequence := s.incrementSeq
	key := s.incrementKeyLocked(sequence)
	s.mu.Unlock()
	ok, err := model.ConsumeTokenLimitIncrement(s.tokenId, s.userId, key, tokens)
	if err != nil {
		return sequence, newAPIKeyTokenLimitError(err)
	}
	if !ok {
		return sequence, newAPIKeyTokenLimitError(nil)
	}
	return sequence, nil
}

func (s *TokenLimitSession) CommitIncrement(sequence int64) {
	if s == nil || !s.enabled || sequence <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.committedIncrements == nil {
		s.committedIncrements = make(map[int64]bool, 1)
	}
	s.committedIncrements[sequence] = true
	if sequence > s.incrementSeq {
		s.incrementSeq = sequence
	}
}

func (s *TokenLimitSession) RefundIncrement(sequence int64, reason string) {
	if s == nil || !s.enabled || sequence <= 0 {
		return
	}
	s.mu.Lock()
	committed := s.committedIncrements[sequence]
	s.mu.Unlock()
	if committed {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "increment_refund"
	}
	key := s.incrementKey(sequence)
	if err := model.RefundTokenLimitPreConsume(key, reason); err != nil {
		common.SysLog("error refunding api key token limit increment: " + err.Error())
	}
}

func (s *TokenLimitSession) PreConsumedTokens() int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preConsumed
}

func (s *TokenLimitSession) AuditInfo() (failed bool, actualTokens int64, preConsumed int64, failureCode string) {
	if s == nil {
		return false, 0, 0, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settleFailed, s.actualTokens, s.preConsumed, s.failureCode
}

func (s *TokenLimitSession) incrementKey(sequence int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.incrementKeyLocked(sequence)
}

func (s *TokenLimitSession) incrementKeyLocked(sequence int64) string {
	return s.requestId + ":realtime:" + strconv.FormatInt(sequence, 10)
}

func RefundTokenLimitOnRelayFailure(relayInfo *relaycommon.RelayInfo, reason string) {
	if relayInfo == nil || relayInfo.TokenLimit == nil {
		return
	}
	relayInfo.TokenLimit.Refund(reason)
}

func MarkTokenLimitAfterResponseFailure(relayInfo *relaycommon.RelayInfo, reason string) {
	if relayInfo == nil || relayInfo.TokenLimit == nil {
		return
	}
	actual := relayInfo.TokenLimit.PreConsumedTokens()
	if actual <= 0 {
		actual = relayInfo.SubscriptionPreConsumedTokens()
	}
	if err := relayInfo.TokenLimit.MarkSettleFailed(actual, reason); err != nil {
		common.SysLog("error marking api key token limit settle failed: " + err.Error())
	}
}

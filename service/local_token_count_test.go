package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/tiktoken-go/tokenizer"
)

func TestDisabledLocalTokenCountingShortCircuitsAllEntrypoints(t *testing.T) {
	oldCountToken := constant.CountToken
	tokenEncoderMutex.Lock()
	oldDefaultEncoder := defaultTokenEncoder
	oldEncoderMap := tokenEncoderMap
	defaultTokenEncoder = nil
	tokenEncoderMap = make(map[string]tokenizer.Codec)
	tokenEncoderMutex.Unlock()
	t.Cleanup(func() {
		constant.CountToken = oldCountToken
		tokenEncoderMutex.Lock()
		defaultTokenEncoder = oldDefaultEncoder
		tokenEncoderMap = oldEncoderMap
		tokenEncoderMutex.Unlock()
	})

	constant.CountToken = false
	InitTokenEncoders()
	tokenEncoderMutex.RLock()
	initializedEncoder := defaultTokenEncoder
	tokenEncoderMutex.RUnlock()
	if initializedEncoder != nil {
		t.Fatal("disabled local token counting initialized the default encoder")
	}
	if got := CountTextToken("must not be tokenized", "gpt-4o"); got != 0 {
		t.Fatalf("CountTextToken() = %d, want 0 while disabled", got)
	}
	if got := EstimateTokenByModel("claude-3-5-sonnet", "must not be estimated"); got != 0 {
		t.Fatalf("EstimateTokenByModel() = %d, want 0 while disabled", got)
	}
	textTokens, audioTokens, err := CountTokenRealtime(nil, dto.RealtimeEvent{
		Type:  dto.RealtimeEventResponseAudioDelta,
		Delta: "invalid audio that must not be parsed",
	}, "gpt-4o-realtime-preview")
	if err != nil {
		t.Fatalf("CountTokenRealtime() error = %v while disabled", err)
	}
	if textTokens != 0 || audioTokens != 0 {
		t.Fatalf("CountTokenRealtime() = (%d, %d), want (0, 0) while disabled", textTokens, audioTokens)
	}
	if usage := ResponseText2Usage(nil, "must not be estimated", "gpt-4o", 123); usage != nil {
		t.Fatalf("ResponseText2Usage() = %#v, want nil while disabled", usage)
	}
}

package model

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsPreservesOtherWithoutAdminFields(t *testing.T) {
	other := common.MapToJsonStr(map[string]any{
		"billing_source": "subscription",
		"frt":            1234,
		"nested":         map[string]any{"admin_info": "visible nested value"},
	})
	log := &Log{Id: 99, ChannelId: 42, ChannelName: "private", Other: other}
	originalData := unsafe.StringData(log.Other)

	formatUserLogs([]*Log{log}, 7)

	require.Equal(t, other, log.Other)
	require.Equal(t, originalData, unsafe.StringData(log.Other), "safe fast path must not copy Other")
	require.Equal(t, 8, log.Id)
	require.Zero(t, log.ChannelId)
	require.Empty(t, log.ChannelName)
}

func TestFormatUserLogsRemovesAdminOnlyTopLevelFields(t *testing.T) {
	log := &Log{Other: `{"visible":1,"admin_info":{"channel":9},"stream_status":{"status":"ok"},"nested":{"stream_status":"keep"}}`}

	formatUserLogs([]*Log{log}, 0)

	require.JSONEq(t, `{"visible":1,"nested":{"stream_status":"keep"}}`, log.Other)
}

func TestFormatUserLogsPreservesLegacyInvalidOtherBehavior(t *testing.T) {
	for _, other := range []string{"", "not-json", `[]`, `"text"`, `null`} {
		t.Run(other, func(t *testing.T) {
			log := &Log{Other: other}
			formatUserLogs([]*Log{log}, 0)
			require.Equal(t, "null", log.Other)
		})
	}
}

func TestFormatUserLogsRemovesEscapedAndDuplicateAdminFields(t *testing.T) {
	log := &Log{Other: `{"\u0061dmin_info":{"first":1},"admin_info":{"last":2},"visible":true}`}

	formatUserLogs([]*Log{log}, 0)

	require.JSONEq(t, `{"visible":true}`, log.Other)
}

func BenchmarkFormatUserLogsLargeOtherWithoutAdminFields(b *testing.B) {
	other := common.MapToJsonStr(map[string]any{
		"payload": strings.Repeat("x", 1<<20),
		"frt":     1234,
	})
	logs := []*Log{{Other: other}}
	b.ReportAllocs()
	b.SetBytes(int64(len(other)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logs[0].Other = other
		formatUserLogs(logs, 0)
	}
}

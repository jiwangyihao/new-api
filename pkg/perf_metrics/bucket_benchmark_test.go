package perfmetrics

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

func BenchmarkRecordHotBucket(b *testing.B) {
	original := perf_metrics_setting.GetSetting()
	oldEnabled, oldRDB := common.RedisEnabled, common.RDB
	b.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldEnabled, oldRDB
		if err := config.GlobalConfig.LoadFromDB(map[string]string{
			"perf_metrics_setting.enabled":     strconv.FormatBool(original.Enabled),
			"perf_metrics_setting.bucket_time": original.BucketTime,
		}); err != nil {
			b.Fatal(err)
		}
	})
	if err := config.GlobalConfig.LoadFromDB(map[string]string{
		"perf_metrics_setting.enabled": "true", "perf_metrics_setting.bucket_time": "hour",
	}); err != nil {
		b.Fatal(err)
	}
	common.RedisEnabled, common.RDB = false, nil
	for _, relay := range []bool{false, true} {
		name := "Record"
		if relay {
			name = "RecordRelaySample"
		}
		b.Run(name, func(b *testing.B) {
			modelName := b.Name()
			b.Cleanup(func() {
				hotBuckets.Range(func(key, _ any) bool {
					if key.(bucketKey).model == modelName {
						hotBuckets.Delete(key)
					}
					return true
				})
			})
			sample := Sample{Model: modelName, LatencyMs: 100, Success: true, HasTtft: true, TtftMs: 20, OutputTokens: 30, GenerationMs: 80}
			info := &relaycommon.RelayInfo{OriginModelName: modelName, StartTime: time.Now().Add(-time.Second)}
			Record(sample)
			b.ReportAllocs()
			if relay {
				for b.Loop() {
					RecordRelaySample(info, true, 30)
				}
			} else {
				for b.Loop() {
					Record(sample)
				}
			}
		})
	}
}

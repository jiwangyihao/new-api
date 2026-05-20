# new-api 本地受控压测 SOP

## 前置条件

1. 仅使用本机 loopback PostgreSQL、Redis、mock upstream 和 `new-api`。
2. 若本机没有隔离 PostgreSQL 或 Redis，不运行 smoke，不伪造通过；在报告中记录：`未运行，原因：缺少本地 PostgreSQL/Redis 前置条件`。
3. 不使用生产 `.env`，运行目录必须是 `.loadtest/runtime/new-api` 且不得包含 `.env`。

## S0/S1 smoke 命令顺序

S1 smoke 必须覆盖以下 4 个入口/用户组合，任一组合失败或未运行，报告必须标为失败/未运行，不得用其他组合结果替代：

| 组合 | Path | API key | Token profile | Run context | Sweep artifact | Report artifact |
|------|------|---------|---------------|-------------|----------------|-----------------|
| Responses + subscription token | `/v1/responses` | `sk-loadtestsub` | `subscription` | `.loadtest/baseline/s1-responses-sub-run-context.json` | `.loadtest/baseline/s1-responses-sub-sweep.json` | `.loadtest/reports/s1-responses-sub-smoke.md` |
| Responses + compat token | `/v1/responses` | `sk-loadtestcompat` | `compat` | `.loadtest/baseline/s1-responses-compat-run-context.json` | `.loadtest/baseline/s1-responses-compat-sweep.json` | `.loadtest/reports/s1-responses-compat-smoke.md` |
| Chat completions + subscription token | `/v1/chat/completions` | `sk-loadtestsub` | `subscription` | `.loadtest/baseline/s1-chat-sub-run-context.json` | `.loadtest/baseline/s1-chat-sub-sweep.json` | `.loadtest/reports/s1-chat-sub-smoke.md` |
| Chat completions + compat token | `/v1/chat/completions` | `sk-loadtestcompat` | `compat` | `.loadtest/baseline/s1-chat-compat-run-context.json` | `.loadtest/baseline/s1-chat-compat-sweep.json` | `.loadtest/reports/s1-chat-compat-smoke.md` |

mock upstream 复用 `s1-smoke` profile 和同一个 `.loadtest/baseline/mock-stats.json`。该 stats 文件是进程级累积快照；每条 sweep 必须使用同一个 `--mock-stats` 路径，并由 sweep 基于 before/after delta 做 per-point 判断。

```bash
mkdir -p .loadtest/config .loadtest/logs .loadtest/baseline .loadtest/runtime/new-api .loadtest/reports
cp config.loadtest.yaml .loadtest/config/config.yaml
.loadtest/bin/loadtest-check-config --config .loadtest/config/config.yaml --out-env .loadtest/config/new-api.env --out-run-context .loadtest/run-context.base.json
.loadtest/bin/loadtest-run-new-api --binary .loadtest/bin/new-api --env .loadtest/config/new-api.env --work-dir .loadtest/runtime/new-api --pid-file .loadtest/new-api.pid --stdout-log .loadtest/logs/new-api.stdout.log --stderr-log .loadtest/logs/new-api.stderr.log --bootstrap-only
.loadtest/bin/loadtest-seed --config .loadtest/config/config.yaml --run-context .loadtest/run-context.base.json --out .loadtest/baseline/seed.json --out-run-context .loadtest/run-context.seeded.json
.loadtest/bin/loadtest-concurrency-sweep --derive-run-context-only --config .loadtest/config/config.yaml --run-context .loadtest/run-context.seeded.json --scenario s1-smoke --token-profile subscription --path /v1/responses --api-key sk-loadtestsub --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --out-run-context .loadtest/baseline/s1-responses-sub-run-context.json
.loadtest/bin/loadtest-concurrency-sweep --derive-run-context-only --config .loadtest/config/config.yaml --run-context .loadtest/run-context.seeded.json --scenario s1-smoke --token-profile compat --path /v1/responses --api-key sk-loadtestcompat --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --out-run-context .loadtest/baseline/s1-responses-compat-run-context.json
.loadtest/bin/loadtest-concurrency-sweep --derive-run-context-only --config .loadtest/config/config.yaml --run-context .loadtest/run-context.seeded.json --scenario s1-smoke --token-profile subscription --path /v1/chat/completions --api-key sk-loadtestsub --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --out-run-context .loadtest/baseline/s1-chat-sub-run-context.json
.loadtest/bin/loadtest-concurrency-sweep --derive-run-context-only --config .loadtest/config/config.yaml --run-context .loadtest/run-context.seeded.json --scenario s1-smoke --token-profile compat --path /v1/chat/completions --api-key sk-loadtestcompat --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --out-run-context .loadtest/baseline/s1-chat-compat-run-context.json
.loadtest/bin/loadtest-mock-openai --addr 127.0.0.1:19080 --run-context .loadtest/baseline/s1-responses-sub-run-context.json --first-token-delay 50ms --stream-duration 500ms --chunk-interval 50ms --output-bytes 128 --prompt-tokens 11 --completion-tokens 17 --status-rate 429=0,502=0 --seed 1 --stats-out .loadtest/baseline/mock-stats.json & echo $! > .loadtest/mock-openai.pid
.loadtest/bin/loadtest-run-new-api --binary .loadtest/bin/new-api --env .loadtest/config/new-api.env --work-dir .loadtest/runtime/new-api --pid-file .loadtest/new-api.pid --stdout-log .loadtest/logs/new-api.stdout.log --stderr-log .loadtest/logs/new-api.stderr.log
.loadtest/bin/loadtest-client --health-check --url http://127.0.0.1:13080 --valid-api-key sk-loadtestsub --invalid-api-key sk-loadtestinvalid --runtime-url http://127.0.0.1:13080/debug/loadtest/runtime --pprof-url 'http://127.0.0.1:8005/debug/pprof/goroutine?debug=1' --out .loadtest/baseline/s0-health.json
.loadtest/bin/loadtest-concurrency-sweep --config .loadtest/config/config.yaml --url http://127.0.0.1:13080 --api-key sk-loadtestsub --token-profile subscription --path /v1/responses --model gpt-5.5 --scenario s1-smoke --points 2 --rps 1 --duration 5s --max-requests-per-point 10 --ramp-step 2 --ramp-interval 1s --timeout 30s --input-bytes 128 --output-bytes 128 --cooldown 2s --pid-file .loadtest/new-api.pid --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --mock-stats .loadtest/baseline/mock-stats.json --run-context .loadtest/baseline/s1-responses-sub-run-context.json --artifact-dir .loadtest/baseline --out .loadtest/baseline/s1-responses-sub-sweep.json
.loadtest/bin/loadtest-concurrency-sweep --config .loadtest/config/config.yaml --url http://127.0.0.1:13080 --api-key sk-loadtestcompat --token-profile compat --path /v1/responses --model gpt-5.5 --scenario s1-smoke --points 2 --rps 1 --duration 5s --max-requests-per-point 10 --ramp-step 2 --ramp-interval 1s --timeout 30s --input-bytes 128 --output-bytes 128 --cooldown 2s --pid-file .loadtest/new-api.pid --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --mock-stats .loadtest/baseline/mock-stats.json --run-context .loadtest/baseline/s1-responses-compat-run-context.json --artifact-dir .loadtest/baseline --out .loadtest/baseline/s1-responses-compat-sweep.json
.loadtest/bin/loadtest-concurrency-sweep --config .loadtest/config/config.yaml --url http://127.0.0.1:13080 --api-key sk-loadtestsub --token-profile subscription --path /v1/chat/completions --model gpt-5.5 --scenario s1-smoke --points 2 --rps 1 --duration 5s --max-requests-per-point 10 --ramp-step 2 --ramp-interval 1s --timeout 30s --input-bytes 128 --output-bytes 128 --cooldown 2s --pid-file .loadtest/new-api.pid --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --mock-stats .loadtest/baseline/mock-stats.json --run-context .loadtest/baseline/s1-chat-sub-run-context.json --artifact-dir .loadtest/baseline --out .loadtest/baseline/s1-chat-sub-sweep.json
.loadtest/bin/loadtest-concurrency-sweep --config .loadtest/config/config.yaml --url http://127.0.0.1:13080 --api-key sk-loadtestcompat --token-profile compat --path /v1/chat/completions --model gpt-5.5 --scenario s1-smoke --points 2 --rps 1 --duration 5s --max-requests-per-point 10 --ramp-step 2 --ramp-interval 1s --timeout 30s --input-bytes 128 --output-bytes 128 --cooldown 2s --pid-file .loadtest/new-api.pid --seed-output .loadtest/baseline/seed.json --mock-profile s1-smoke --mock-stats .loadtest/baseline/mock-stats.json --run-context .loadtest/baseline/s1-chat-compat-run-context.json --artifact-dir .loadtest/baseline --out .loadtest/baseline/s1-chat-compat-sweep.json
.loadtest/bin/loadtest-report --sweep .loadtest/baseline/s1-responses-sub-sweep.json --out .loadtest/reports/s1-responses-sub-smoke.md
.loadtest/bin/loadtest-report --sweep .loadtest/baseline/s1-responses-compat-sweep.json --out .loadtest/reports/s1-responses-compat-smoke.md
.loadtest/bin/loadtest-report --sweep .loadtest/baseline/s1-chat-sub-sweep.json --out .loadtest/reports/s1-chat-sub-smoke.md
.loadtest/bin/loadtest-report --sweep .loadtest/baseline/s1-chat-compat-sweep.json --out .loadtest/reports/s1-chat-compat-smoke.md
```

## 清理

smoke 结束后停止 `new-api`、mock upstream、PostgreSQL 和 Redis，并确认 15432、16379、13080/18081、19080、8005 端口关闭。若任一端口仍开放，不得开始下一轮对比。

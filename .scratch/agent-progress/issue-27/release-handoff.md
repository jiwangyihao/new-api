# Issue #27 → Issue #28 发布交接

## 验收提交与边界

- Issue：`jiwangyihao/new-api#27`。
- Worker 基线：`b45bc8694e7e7a2b15be9e2447b46e140090191f`；该提交是 `credit-operational-value-integration` 的祖先，并已包含 #20–#26 的前向 writer 合同。
- 本文件中的 Worker HEAD 必须在最终提交后由协调器以 `git rev-parse HEAD` 复核；不得以未提交工作树、根 `main` 或生产旧 SHA 代替。
- #27 只交付历史价格回填、Credit/timed 历史重建、marker/CAS、维护命令、ready/suspended 门禁和三数据库验收；未构建生产镜像、未访问或修改生产数据库、未切换流量，也未执行 #28。
- 估值/初始迁移规则版本：`1`。#28 必须先只读读取生产最高 marker；若不是预期的“无 marker/可执行 version 1”状态，停止并诊断，禁止覆盖或伪造 marker。

## 维护命令合同

根二进制在任何 HTTP、Redis、定时器或业务 worker 初始化前识别 `credit-valuation-migrate`，只初始化维护数据库连接，输出单行结构化 JSON 后退出。

```text
new-api credit-valuation-migrate --dry-run --version 1
new-api credit-valuation-migrate --apply --version 1 --batch-size 100
new-api credit-valuation-migrate --verify --version 1
new-api credit-valuation-migrate --repair-missing-as-unknown --version <更高新版本>
new-api credit-valuation-migrate --suspend --version <当前 ready 版本> --reason <非空审计原因>
```

- 五种模式严格互斥；`--version` 必须为正整数。
- `--batch-size` 仅允许用于 `--apply`；默认值为 100。
- `--reason` 仅允许且必须用于 `--suspend`。
- 参数/合同错误退出 `2`；数据库、迁移或输出失败退出 `1`；成功退出 `0`。
- dry-run 与 verify 完全只读。#28 必须在同一生产快照、同一目标 digest、同一 version 上连续执行两次 dry-run，并要求完整业务 JSON/checksum 相同。
- apply 使用稳定主键批次、marker CAS 和可重跑 upsert；报告中的批次边界、原因和 blocker 使用稳定排序。checksum 不包含时间、耗时、路径或进程信息。
- 同版本 `ready` apply 重放是可观察 no-op；不同 checksum、不同冻结 FX、活动 lease、过期版本或更高 non-ready marker 不得被静默覆盖。

## Marker、CAS 与恢复

状态集合固定为 `pending`、`running`、`ready`、`failed`、`suspended`。

- apply 先冻结估值币种、严格有理数 FX 和输入 checksum，再以 CAS 取得/恢复 version marker。
- 活动 `running` lease 拒绝第二执行者；仅过期 lease 且 checksum/冻结输入完全相同时允许恢复。
- verify 原子检查历史精确价格、每份 Credit 恰有一行状态、数量/币种/非负/unknown 上界/规则与迁移版本、来源唯一性、timed grant 唯一性和 checksum；任一失败不得部分 ready。
- `repair-missing-as-unknown` 只用于停写维护窗口、显式更高 migration version；它只保守补缺失 Credit 状态为 unknown，不能修非法套餐价格，之后仍须 apply/verify。
- `suspend` 只允许停写窗口内从最高 `ready` marker 携带非空 reason 原子转换；suspended 时普通业务写保持 fail-closed。
- ready 后所有 Credit 数量 writer 与估值状态同锁同事务；状态 missing/mismatch 整笔回滚，禁止热路径补 unknown 或退回 legacy delta。

## 停写 blocker 与稳定原因

进入 ready 前必须清零并保留结构化证据：

- `non_terminal_preconsume`：存在非终态订阅预扣记录。
- `active_subscription_task_missing_request_identity`：仍可能回调的历史异步任务缺少稳定请求身份。
- `legacy_writer_session_active`：仍有旧 writer 会话可写数据库。
- `credit_plan_missing` / `credit_plan_ambiguous`：全局 Credit 计划缺失或不唯一。
- `valuation_currency_invalid`：Credit 估值币种不是唯一受支持的 CNY/USD。
- `fx_option_missing` / `fx_option_invalid`：跨币种迁移所需原始汇率配置缺失或无法严格解析。

blocker 非零时 dry-run 可报告，但 #28 不得 apply/ready；不得删除在途记录、伪造 Task request ID 或篡改 marker 来清零。

## 三数据库零 SKIP 证据

唯一入口：

```text
go test ./model -run 'TestCreditValuationExternalMatrix$' -count=1 -v -timeout 60m
```

最终结果：退出 `0`，`ok github.com/QuantumNous/new-api/model 2437.133s`，无 `SKIP`。

- SQLite `3.50.4`：PASS。
- MySQL `5.7.44`：PASS。
- PostgreSQL `9.6.24`：PASS。
- 每库通过 12 个相同行为阶段，共 36 个阶段：schema/命名唯一约束、真实锁、原始十进制价格、双 dry-run/apply/verify/ready replay、购买消费退款分析、转换、破坏性恢复，以及五组并发合法串行化。
- 外部 DSN 只存在仓库外受限环境；测试缺 DSN 时 Fatal，不 Skip。交付证据不含 DSN、账号或凭据。

MySQL 首轮 consume+restore 暴露了缓存候选陈旧问题。最终实现中缓存只提供候选 ID；`forUpdate=true` 时当前事务按 ID 重查并加 `FOR UPDATE`，再校验 status、时间、余额与 `requiredTokens`。定向 MySQL 复跑、MySQL 完整子矩阵和唯一三库入口均已转绿。

## 冻结 32 CNY tracer

共享矩阵通过现有领域入口建立以下 fixture，而不是直接插入最终估值状态：

- 充值档位：`40 CNY / 1,000 Credit`。
- 持有权益：显式 `credit_balance`、零价格全局 Credit 容器、`end_time = 0`。
- 消费：200 Credit；最终 `available_credit = 800`。
- 结果：`exact_cost_micros = 32,000,000`、币种 CNY、`active_paid_subscription_count = 1`、estimated=0、unknown=0。
- summary/users/subscriptions/plans/sources 五个运营分析视图读取同一结果。

#28 必须在最新生产一致备份的隔离克隆上重跑同版本 fixture；只有已存在且明确授权的受控生产账号才允许生产写探针。禁止为验收在生产创建临时用户、套餐、订单、订阅或权益。

## 请求、旧 Task 与回滚边界

- 新请求以持久 `request_id` 和目标累计 Credit 结算；相同目标重放 no-op，追加消费按当时池平均值，减少目标按本请求快照恢复。
- 新异步 Task 持久化 `subscription_request_id`；旧 Task 缺该字段时使用持久 Task 主键生成确定性兼容身份，仍必须经过估值深模块。
- 预扣、正 delta 合并、退款和转换结算都保留请求身份；禁止匿名 Credit delta。
- ready 前可在附加 schema 兼容前提下 image-only rollback。
- ready 后但外部写未开放：先停服，保留所有附加结构/marker，才允许回到旧镜像并随后重新迁移验证。
- 强制双写接受生产流量后禁止 image-only rollback。必须回滚时：停止全部写 → 原子 suspend 并记录 reason → 创建新一致备份 → 使用新 migration version 重建 → verify 后恢复。
- 任何阶段都不得删除估值 state、immutable grant、ledger、请求快照，或把 marker 伪装为 pending。

## #28 必须重新观察，不能继承为生产结论

以下项目只有 #27 本地/隔离数据库证据，#28 必须在 `netcup-ows-migrate` 按发布合同重新取证：

1. 当前生产 release/commit/image digest、容器、反代、PostgreSQL/Redis、资源与健康。
2. 当前最高 marker、历史价格诊断、estimated/unknown、unsupported currency、歧义和所有 blocker。
3. 同一目标 digest 的两次在线 dry-run 及稳定 checksum。
4. 停写后 HTTP 写、后台任务、非终态预扣、异步回调和旧 writer DB 会话确实清零。
5. 一致 PostgreSQL 备份绝对路径、大小、UTC、SHA-256 与可读/可恢复检查。
6. 同一 digest/version/checksum 的 apply 与 verify，封闭启动、ready fail-closed 与旧分析兼容。
7. 生产只读不变量；隔离克隆或明确授权账号上的 32 CNY/五接口/disabled-plan 行为。
8. 真实生产前端经已认证 API 展示 32 CNY、exact/estimated/unknown、Credit 时间价值“不适用”和 warning。
9. 开放写的精确 UTC、双写已接受流量、观察窗口、锁等待/错误/unknown/FX/结算/coalescer/资源指标。
10. 三阶段回滚演练。静态页面、健康 200 或容器 running 不能替代数据库/API/业务证明。

## 敏感信息与交付文件

- 本交接、`status.md`、`evidence.md` 和 `contract.md` 不含 DSN、令牌、Cookie、私钥、dump 或可识别用户数据。
- 临时数据库探针、源码 tar.gz 和原始运行日志均已删除；不进入提交。
- #28 只允许使用已配置的 SSH 主机别名 `netcup-ows-migrate`，禁止裸 IP、其他主机或复制凭据。

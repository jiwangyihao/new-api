# Issue #20 Standards 评审修复状态

## 基线

- 验收候选 HEAD：`9e3329d0f4b509d1179c895c52f01af7a19f0ca4`。
- 分支：`jiwangyihao/issue-20-valuation-foundation`。
- 开始状态：staged 0、unstaged 0、untracked 0。

## 当前阶段

Finding 2 已 GREEN 并提交；Finding 3 调查中。已确认所有生产 Credit allocation 都汇聚到 `model.GrantCreditBalanceTx`，但该函数当前先锁用户、随后可信任未锁 `TargetPlanSnapshot`，未与 controller 的币种更新共享套餐行锁。

## RED / GREEN

| Finding | RED | GREEN | 状态 |
| --- | --- | --- | --- |
| 1. 禁止从 JavaScript Number 伪造历史 micros | 前端与后端 RED 已复现 | 前端 13/13、后端定向测试通过 | GREEN |
| 2. 关键 schema 变化 fail-closed | SQLite 伪装 MySQL 元数据失败时 `migrateDB` 曾继续 | 错误传播且历史 micros 保持 NULL | GREEN，已提交 |
| 3. 币种冻结与首个 Credit 权益共享线性化接缝 | 入口与锁序已定位 | 待 RED / GREEN | 调查中 |

## 下一条命令

继续读取订单完成事务锁序，定义 model 计划级 guard，并先写并发/事务 RED 测试。

## 阻塞与工具卡点

无业务阻塞。读取 `model/subscription.go:1260-1335` 时工具 socket 曾断开一次；工作树当时 clean，重试即可。

## 最近安全提交

最近安全提交：`929cedb60`（Finding 2）；此前 `947b412ba`（Finding 1）、`f160e8a10`（恢复记录）。当前 HEAD `929cedb6092da7305a48b1b91728f9f73e2fbbbd`。

## 未提交现场

持久化前 `git status --short` 无输出：staged 0、unstaged 0、untracked 0。本次仅修改三份 review-fix 进度文件以保存调查进展。

## 范围边界

本轮只修复三项 Standards finding；不回填历史价格，不修改 migration marker，不决定或阻止 `ready`，不启用 Credit 数量/估值强制双写，不实现 #21–#28。

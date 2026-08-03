# Issue #20 Standards 评审修复状态

## 基线

- 验收候选 HEAD：`9e3329d0f4b509d1179c895c52f01af7a19f0ca4`。
- 分支：`jiwangyihao/issue-20-valuation-foundation`。
- 开始状态：staged 0、unstaged 0、untracked 0。

## 当前阶段

Finding 1 调查：准备建立“历史 `price_amount_micros = NULL` 套餐无关编辑不得生成 micros”的 RED 行为测试。

## RED / GREEN

| Finding | RED | GREEN | 状态 |
| --- | --- | --- | --- |
| 1. 禁止从 JavaScript Number 伪造历史 micros | 待运行 | 待实现 | 调查中 |
| 2. 关键 schema 变化 fail-closed | 待运行 | 待实现 | 未开始 |
| 3. 币种冻结与首个 Credit 权益共享线性化接缝 | 待运行 | 待实现 | 未开始 |

## 下一条命令

从 `web/default` 运行现有 `plan-form` 定向测试，确认历史 NULL 精确值的当前污染路径，再持久化最小 RED 用例。

## 阻塞

无。

## 最近安全提交

开始本轮修复前：`9e3329d0f4b509d1179c895c52f01af7a19f0ca4`。

## 范围边界

本轮只修复三项 Standards finding；不回填历史价格，不修改 migration marker，不决定或阻止 `ready`，不启用 Credit 数量/估值强制双写，不实现 #21–#28。

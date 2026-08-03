# Issue #22 验证证据

## 基线
- `git rev-parse HEAD`：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。
- `git status --porcelain`：空。
- 分支：`jiwangyihao/issue-22-credit-tracer`。
- `git merge-base HEAD jiwangyihao/credit-operational-value-integration`：`53c91e6e3a795b01b4c426c9a69ff532cd8712c8`。

## RED/GREEN
- 尚未开始代码 RED；下一项必须是通过真实数据库/领域入口证明目标行为缺失的定向测试。

## 约束证据
- 金额权威字段为十进制 micros；后端内部使用整数，前端使用 BigInt/字符串优先。
- Credit 分析显式按 `entitlement_type=credit_balance` 分流；不读取零价容器价格、不看 `end_time`，来源固定 `credit_balance_pool/moving_weighted_pool`。
- 状态缺失/不一致、币种、溢出、档位和幂等问题必须稳定错误码并整体回滚。

## 运行记录
后续每个命令、失败根因、修复与精确结果追加于此文件。

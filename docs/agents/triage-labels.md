# 分诊标签

工程技能使用五种规范分诊角色。下表将这些角色映射到本仓库 Issue 跟踪器中的实际标签字符串。

| 规范角色 | 跟踪器标签 | 含义 |
| --- | --- | --- |
| `needs-triage` | `needs-triage` | 等待维护者评估 |
| `needs-info` | `needs-info` | 等待报告者补充信息 |
| `ready-for-agent` | `ready-for-agent` | 规格完整，AFK Agent 无需额外人工上下文即可处理 |
| `ready-for-human` | `ready-for-human` | 需要人工实现或决策 |
| `wontfix` | `wontfix` | 不会处理 |

技能提到某个规范角色时，必须应用表中对应的标签字符串。不要用现有的 `question` 替代 `needs-info`，也不要用 `help wanted` 替代 `ready-for-human`；它们表达的是不同维度。

# Issue #21 宽回归修复 E 状态

## 当前阶段

验证完成：目标重复测试、关联回归和完整 controller 包均已通过，正在提交最小隔离修复。

## 冻结现场

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-21-wide-controller-fix`
- 分支：`jiwangyihao/issue-21-wide-controller-fix`
- 共同父 HEAD：`3e74a2928f7e4b7c3d5c6eae3fbc8362172a4c5d`
- 当前安全 HEAD：`3e93b4782`（诊断前恢复文档安全点）
- 交付范围：DB timestamp 测试清理接缝、目标 controller 测试 setup/cleanup 与本进度记录。

## 已完成

- 本地 `-count=10 -v` 保存 RED：第 1–3 轮 PASS，第 4–10 轮在秒边界后稳定失败于 `service.PreConsumeBilling`，code 为 `subscription_required`。
- 定位根因：每轮切换测试 DB，但进程级 DB timestamp cache 未清；新 Credit 权益 `StartTime` 来自新 DB 时间，预扣选择读取上一轮缓存时间，秒边界后暂时得到 `StartTime > now`。
- 只导出 `ClearDBTimestampCacheForTest` 薄包装，并在目标测试 setup 后、cleanup 中清理；真实 StartTime、固定 user ID 与全部业务断言保持不变。
- 目标测试 `-count=25` PASS；关联三测试 `-count=10` PASS；`go test ./controller -count=1` PASS。
- LSP references 与仓库搜索均证明该导出函数没有生产调用：除定义外，仅目标 controller 测试两处调用。

## 下一步

由协调器按共享合同合入 #21 父分支并运行三包宽回归。

## 阻塞项

无。

## 最近安全提交

- `3e93b4782 docs(agents): 记录 controller 隔离诊断基线`
- 本轮 GREEN 修复：随当前交付提交。

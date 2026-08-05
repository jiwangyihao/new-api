# Issue #23 最终 Spec F1/F2 修复证据

## 冻结现场

命令：

```text
git branch --show-current
git rev-parse HEAD
git status --short
git merge-base HEAD ec1858fec89509bdec9a90a230a8496047c5becd
```

结果：

```text
jiwangyihao/issue-23-request-settlement
8cdfd4acb78b502af4c0232460baf7df852b7b2c
<git status --short 无输出>
ec1858fec89509bdec9a90a230a8496047c5becd
```

## 最终 Spec FAIL 复现依据

最终复评报告：`C:/Users/34404/AppData/Local/Temp/new-api-issue23-spec-final-rereview.md`。

- F1：既有 `request_id` 分支仅拒绝 refunded，随后返回旧结果，未比较本次调用的不可变参数。
- F2：`PostConsumeQuota` 仍可对 subscription 调用匿名 token delta，且导出 helper 未拒绝 Credit target。

## F1 根因

- 位置：`model/subscription.go` 的 `SubscriptionPreConsumeRecord` 与 `preConsumeUserSubscriptionByUnits`。
- 观察：记录没有版本化请求指纹；命中既有 request_id 后未核对 user/model/quota_type/distributor amount。
- 反馈循环：将通过公开 `PreConsumeUserSubscriptionByUnits` 与真实 SQLite 构造同 request_id 异参重放，断言稳定冲突与所有状态零写入。

## RED / GREEN / 回归

待每次实际命令完成后追加精确命令、输出和分类；不得把未运行项写成 PASS。

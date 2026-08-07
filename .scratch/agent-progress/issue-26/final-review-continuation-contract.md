# Issue #26 最终复评续作合同

- 冻结基线：`44009213cb8e4a582de34f884deecd5a8d687b2c`。
- 当前 phase：`M2_GREEN_COMPLETE`；最近安全提交 `0e40a74fe`。
- H1 `3feb091159aef26731c1698647791acc03c29c0a`、#24 H2 与路由校准保持在祖先链。
- M1/M3 已由 RED `9ffade1ac`、GREEN `0f98f18ed` 完成。
- M2 RED `81b3f1d9d` 固化报价身份与 stale 失败合同；GREEN `0e40a74fe` 实现服务端持久化 `quote_id`、DB 权威时间、版本化事实快照与 SHA-256 fingerprint。
- Confirm 在同一事务内按 user/source 锁定报价并重算权威事实；过期、Plan 改价、remaining Credit、basis 或资格漂移均返回 `ErrConversionQuoteStale`，失败零 conversion、ledger、Credit 或 source mutation。
- 同 idempotency key + 同 quote 重放已提交 conversion；不同 quote 或 source 冲突返回稳定幂等冲突 code。
- 固定数量公式、31 天业务月、conversion 非新增收款/邀请归因、H1 request-first 锁序均未改变。
- 未修改 #24 ingress/UI/幂等，未实现 #25/#27/#28；MySQL/PostgreSQL 实机矩阵仍归 #27。
- 当前仅三份 progress 文档待提交；随后确认 clean 并交付。

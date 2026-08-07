# Issue #26 最终复评续作合同

- 冻结基线：`44009213cb8e4a582de34f884deecd5a8d687b2c`。
- 当前 phase：`M2_RED_HANDOFF_READY`；最近安全提交 `81b3f1d9d`。
- H1 `3feb091159aef26731c1698647791acc03c29c0a`、#24 H2 与路由校准保持在祖先链。
- M1/M3 已由 RED `9ffade1ac`、GREEN `0f98f18ed` 完成。
- M2 RED `81b3f1d9d` 固化：quote 必须返回 `quote_id`、`created_at`、`expires_at`、版本化 authoritative facts fingerprint；Plan/Option/FX/basis/target mapping/资格事实漂移后 confirm 必须返回 `ErrConversionQuoteStale` 与稳定 code，且 conversion、ledger、Credit、source 零写入。
- 服务端必须持久化或以同等强度验证 identity、所有者、source、有效期及权威事实；不得信任客户端 fingerprint，不得新增隐式旧 API fallback。
- 固定数量公式、31 天业务月、conversion 非新增收款/邀请归因、H1 request-first 锁序均不得改变。
- 禁止修改 #24 ingress/UI/幂等，禁止实现 #25/#27/#28。
- 当前交付仅为 RED handoff；M2 GREEN 未开始。
- RED 后重复编辑已恢复到 `81b3f1d9d`；当前仅三份 handoff 进度文档待提交。

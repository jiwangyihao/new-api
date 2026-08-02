# Issue 跟踪器：GitHub

本仓库的 Issue 与 PRD 存放在 GitHub 仓库 `jiwangyihao/new-api` 中。所有操作使用 `gh` CLI，并显式传入 `--repo jiwangyihao/new-api`，避免仓库中 `origin` 与 `deploy` 两个远端造成歧义。

## 操作约定

- 创建：`gh issue create --repo jiwangyihao/new-api --title "..." --body "..."`。多行正文使用 `--body-file <file>`。
- 读取：`gh issue view <number> --repo jiwangyihao/new-api --comments --json number,title,body,state,labels,comments,url`。
- 列出：`gh issue list --repo jiwangyihao/new-api --state open --json number,title,body,labels,comments`，按任务需要增加 `--label` 与 `--state` 筛选。
- 评论：`gh issue comment <number> --repo jiwangyihao/new-api --body "..."`。
- 添加或移除标签：`gh issue edit <number> --repo jiwangyihao/new-api --add-label "..."` 或 `--remove-label "..."`。
- 关闭：`gh issue close <number> --repo jiwangyihao/new-api --comment "..."`。

## PR 是否作为分诊请求入口

**否。** `/triage` 只处理 Issues，不把外部 PR 纳入同一分诊队列。协作者和外部贡献者的 PR 均沿用常规代码审查流程。

如果以后将此设置改为“是”，外部 PR 才使用与 Issues 相同的标签和状态；仅纳入 `authorAssociation` 为 `CONTRIBUTOR`、`FIRST_TIME_CONTRIBUTOR` 或 `NONE` 的 PR，不处理 `OWNER`、`MEMBER` 或 `COLLABORATOR` 的进行中 PR。

GitHub 的 Issue 与 PR 共用编号空间。需要解析裸编号（例如 `#42`）时，先运行 `gh pr view 42 --repo jiwangyihao/new-api`，失败后再运行对应的 `gh issue view`。

## 技能术语

- “发布到 Issue 跟踪器”：创建一个 GitHub Issue。
- “获取相关工单”：读取对应 GitHub Issue 及其评论与标签。

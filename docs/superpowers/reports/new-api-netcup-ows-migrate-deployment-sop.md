# new-api 生产部署 SOP（netcup-ows-migrate）

## 适用范围

本文档记录 `jiwangyihao/new-api` 在当前生产主机 `netcup-ows-migrate` 上的发布、健康检查和故障回滚流程。

当前只读核验事实：

- **SSH 主机别名：** `netcup-ows-migrate`
- **部署目录：** `/opt/new-api`
- **基础 Compose：** `/opt/new-api/compose.yml`
- **发布覆盖文件：** `/opt/new-api/compose.release.yml`
- **生产服务：** `new-api`
- **依赖服务：** `postgres`、`redis`
- **本机监听：** `127.0.0.1:13080` 和 `127.0.0.1:13081` 映射至容器 `3000/tcp`
- **镜像发布方式：** GHCR 不可变 digest
- **GitHub 仓库：** `jiwangyihao/new-api`

`/opt/new-api` 不是 Git 工作树。禁止在服务器执行源码级 `git pull`、热改源码或本地构建生产镜像。

## 当前拓扑

只读核验时，生产 Compose 包含：

```text
new-api:   healthy，使用 /opt/new-api/compose.release.yml 固定不可变 digest
postgres:  postgres:18.4-alpine，healthy
redis:     redis:8.8.0，healthy
network:   new-api-network
```

核验时发布覆盖文件固定为：

```text
ghcr.io/jiwangyihao/new-api@sha256:093c2b638a3a4e3c99f511257f62ebd9fa34e5d71b2cf43168c244edbe57be2f
```

该 digest 只描述核验时状态；每次发布必须重新从服务器读取实际 digest，不得把本文值当作待发布目标。

## 发布前检查

### 1. 固定源码提交

```bash
git status --short --branch
git rev-parse HEAD
```

要求：

- 待发布提交已经通过本次变更要求的后端、前端、构建和数据迁移门禁。
- 工作树没有未处理的目标发布改动。
- 不回滚、覆盖或提交其他人的无关改动。

### 2. 构建并固定不可变镜像

通过仓库既有 GHCR workflow 构建镜像：

```bash
gh run list --repo jiwangyihao/new-api --branch main --limit 5
gh run watch <run-id> --repo jiwangyihao/new-api --exit-status
```

构建成功后记录源码 commit SHA、workflow run ID、镜像 ID 和 GHCR immutable digest。禁止使用 `latest` 或其他漂移 tag 作为生产发布目标。

### 3. 只读生产基线

```bash
ssh netcup-ows-migrate
cd /opt/new-api
docker compose -f compose.yml -f compose.release.yml ps
docker compose -f compose.yml -f compose.release.yml config --services
docker inspect new-api
docker inspect new-api-postgres
docker inspect new-api-redis
wget -q -O - http://127.0.0.1:13080/api/status
```

记录当前 release、digest、容器健康、端口、数据库/Redis、磁盘和内存。不得把本地 HEAD 当作生产状态证据。

## 标准发布流程

### 1. 更新发布覆盖文件

将 `/opt/new-api/compose.release.yml` 中 `services.new-api.image` 更新为经过验收的 immutable digest。先保存原文件和原 digest，便于审计与安全回滚。

### 2. 拉取目标镜像

```bash
cd /opt/new-api
docker compose -f compose.yml -f compose.release.yml pull new-api
```

镜像拉取失败时不得重启当前健康容器。

### 3. 执行适用的数据安全步骤

如果版本包含维护迁移：

- 使用该版本定义的一次性发布脚本；
- 使用 `flock` 防止并发发布；
- 使用 `trap` 保守清理；
- 先执行只读 dry-run；
- 停写后创建一致备份并记录绝对路径、大小和 SHA-256；
- apply 与 verify 使用同一镜像 digest 和迁移版本；
- 任一步骤失败都保持写关闭并保留证据。

不得在终端临时拼接不可恢复的大段脚本。

### 4. 重建应用服务

```bash
docker compose -f compose.yml -f compose.release.yml up -d new-api
```

只操作 `new-api` 服务，除非本次经过审阅的发布合同明确要求变更 PostgreSQL 或 Redis。

## 部署后健康检查

```bash
cd /opt/new-api
for i in $(seq 1 60); do
  state=$(docker inspect -f '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}no-health{{end}}' new-api)
  echo "$state"
  case "$state" in *healthy*) break;; esac
  sleep 2
done

docker compose -f compose.yml -f compose.release.yml ps new-api
docker inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' new-api
wget -q -O - http://127.0.0.1:13080/api/status
docker stats --no-stream new-api
docker inspect -f '{{.State.StartedAt}} {{.RestartCount}}' new-api
docker logs --since 10m new-api
```

发布完成必须同时满足：

- `new-api` 为 `running` 且 `healthy`；
- 运行镜像 digest 等于本次验收 digest；
- revision 与待发布 commit 一致；
- `/api/status` 返回预期 JSON；
- 没有迁移失败、持续 5xx、panic、异常重启或关键业务探针失败。

## 回滚边界

### 无强制双写或不可逆迁移

在确认数据合同允许镜像回滚后：

1. 保存容器、日志和健康现场；
2. 将 `compose.release.yml` 恢复为上一份经过审阅的 immutable digest；
3. `pull` 并重建 `new-api`；
4. 重跑完整健康和业务检查。

### 强制双写或迁移门禁已经接受生产写

禁止 image-only rollback。必须遵循对应 release runbook：

1. 停止全部外部和后台写；
2. 原子进入规定的维护/暂停状态并记录原因；
3. 创建新的完整一致备份；
4. 使用新的迁移版本重建和 verify；
5. 只有验证完成后才恢复写流量。

不得删除迁移表、不可变记录、账本或请求快照，也不得伪造 marker 状态。

## 常见故障判断

| 现象 | 优先检查 | 处理原则 |
|---|---|---|
| 镜像拉取失败 | GHCR workflow、网络、immutable digest | 保留当前健康容器，不执行重建 |
| 容器 `unhealthy` | health log、应用日志、依赖健康 | 查明原因后决定向前修复或安全回滚 |
| `/api/status` 不通 | 容器健康、端口映射、反代 | 先查主机本地 `127.0.0.1:13080`，再查公网代理 |
| revision/digest 不匹配 | workflow、release override、镜像缓存 | 停止发布并重新核对，不使用漂移 tag |
| CPU/内存异常 | `docker stats`、进程、PostgreSQL、Redis | 区分应用、数据库、缓存和共享资源压力 |
| 迁移或 verify 失败 | marker、迁移日志、备份、写流量 | 保持写关闭，禁止自动放流或盲目重跑 |

## 禁止事项

- 禁止将任何非 `netcup-ows-migrate` 主机、alias 或 IP 当作当前生产目标。
- 禁止在服务器本地构建生产镜像或热改源码/二进制。
- 禁止使用漂移 tag。
- 禁止把容器启动成功冒充发布完成。
- 禁止在任何远端或生产环境运行本地压测。
- 禁止向仓库提交凭据、DSN、Cookie、数据库 dump 或敏感生产日志。

# RackNerd6C6G 生产部署 SOP

## 适用范围

本文档记录 `jiwangyihao/new-api` 迁移到新 VPS `RackNerd6C6G` 后的生产部署、健康检查和故障回滚流程。

- **当前生产主机：** `RackNerd6C6G`
- **部署目录：** `/opt/new-api`
- **Compose 文件：** `/opt/new-api/compose.yml`
- **生产服务：** `new-api`
- **镜像：** `ghcr.io/jiwangyihao/new-api:latest`
- **本机监听：** `127.0.0.1:13080 -> 3000/tcp`
- **代码分支：** `main`
- **推送远端：** `deploy`
- **GitHub 仓库：** `jiwangyihao/new-api`

> 注意：`/opt/new-api` 当前不是 Git 工作树，不要在 VPS 上执行源码级 `git pull` 部署。生产更新以 GHCR 镜像为准。

## 当前运行状态核验

最后一次核验时间：`2026-05-22T13:10:46+00:00`（VPS UTC 时间）。

### 基础环境

```text
Host: RackNerd6C6G
Workdir: /opt/new-api
Kernel: Linux racknerd-1b0b437 6.8.0-117-generic x86_64
Docker: Docker version 29.5.2, build 79eb04c
Docker Compose: Docker Compose version v5.1.3
```

### 服务状态

```text
new-api: running healthy
Revision: 6b628168a37daa9d8e5c8ac2fc2fb1db043502c6
StartedAt: 2026-05-21T15:25:23.703396378Z
RestartCount: 0
Port: 127.0.0.1:13080->3000/tcp
```

`/api/status` 已在 VPS 本机通过 `http://127.0.0.1:13080/api/status` 返回状态 JSON。

### 资源快照

```text
Memory: 5.8 GiB total, 1.7 GiB used, 4.1 GiB available
Swap: 3.0 GiB total, 73 MiB used
Disk /: 96 GiB total, 18 GiB used, 74 GiB available, 20% used
new-api container: 176.5 MiB / 5.786 GiB, 2.98% memory
```

新 VPS 同时运行 1Panel/OpenResty、PostgreSQL、Redis 和 `sub2api` 相关容器。检查 CPU 或内存异常时，应同时观察这些服务，避免把共享资源压力误判为 `new-api` 单点问题。

## 标准发布流程

### 1. 本地确认待发布提交

```bash
git fetch deploy main
git status --short --branch
git rev-parse HEAD
git rev-parse deploy/main
```

要求：

- `HEAD` 是准备发布的提交。
- 工作区没有未处理的目标发布变更。
- 若存在其他人的未提交改动，只提交和发布本次明确范围内的文件，禁止回滚或覆盖。

### 2. 推送到部署远端

```bash
git push deploy main
```

推送后确认 GitHub Actions 触发 `Build deployment image`。

### 3. 等待 GHCR 镜像构建完成

```bash
gh run list --repo jiwangyihao/new-api --branch main --limit 5
gh run watch <run-id> --repo jiwangyihao/new-api --exit-status
```

要求：

- 对应提交的 workflow 必须是 `completed success`。
- 不得在镜像构建完成前到 VPS 执行 `docker compose pull`。
- GitHub Actions 的 Node.js 20 deprecation annotation 当前只影响提示，不作为发布阻断项；若 job 失败，必须按失败处理。

### 4. 在新 VPS 拉取并重启服务

```bash
ssh RackNerd6C6G
cd /opt/new-api
docker compose -f compose.yml pull new-api
docker compose -f compose.yml up -d new-api
```

也可以从本地直接执行：

```bash
ssh RackNerd6C6G 'cd /opt/new-api && docker compose -f compose.yml pull new-api && docker compose -f compose.yml up -d new-api'
```

要求：

- 只操作 `new-api` 服务，避免误重启同机其他服务。
- 不在旧 VPS host alias `racknerd` 上执行生产部署。
- 不在 `/opt/new-api` 中写入源码、测试产物或临时压测文件。

### 5. 部署后健康检查

```bash
cd /opt/new-api
for i in $(seq 1 60); do
  state=$(docker inspect -f '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}no-health{{end}}' new-api)
  echo "$state"
  case "$state" in *healthy*) break;; esac
  sleep 2
done

docker compose -f compose.yml ps new-api
docker inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' new-api
wget -q -O - http://127.0.0.1:13080/api/status
```

发布完成必须同时满足：

- `docker compose ps` 显示 `new-api` 为 `Up ... (healthy)`。
- `org.opencontainers.image.revision` 等于本次发布的 Git commit。
- `/api/status` 返回 JSON，而不是连接失败、HTML 错误页或空响应。

### 6. 资源与日志抽查

```bash
docker stats --no-stream new-api
docker inspect -f '{{.State.StartedAt}} {{.RestartCount}} {{index .Config.Labels "org.opencontainers.image.revision"}}' new-api
docker logs --since 10m new-api
```

观察点：

- `RestartCount` 应保持为 `0` 或符合预期的单次重建。
- 健康检查日志退出码应为 `0`。
- 业务侧 `Invalid token`、客户端主动断开、上游参数校验错误不等于容器不健康；但如果出现 `panic`、进程退出、迁移失败或持续 5xx，应立即进入排障。

## 回滚流程

如最新镜像部署后无法恢复健康：

1. 先保留现场：

   ```bash
   cd /opt/new-api
   docker compose -f compose.yml ps new-api
   docker inspect new-api
   docker logs --tail=200 new-api
   ```

2. 若 GHCR 中上一版镜像仍可通过 tag 或 digest 获取，临时把 `compose.yml` 的 `new-api` 镜像指向上一版 digest，再执行：

   ```bash
   docker compose -f compose.yml pull new-api
   docker compose -f compose.yml up -d new-api
   ```

3. 回滚后仍必须执行完整健康检查，确认 revision、容器健康状态和 `/api/status`。

4. 本地仓库按正常 Git 流程提交修复或 revert，不在 VPS 上直接改源码。

## 常见故障判断

| 现象 | 优先检查 | 处理原则 |
|------|----------|----------|
| `docker compose pull` 失败 | GHCR workflow、网络、镜像 tag | 不重启现有健康容器，先确认镜像可拉取 |
| 容器 `unhealthy` | `docker inspect` health log、`docker logs --tail=200 new-api` | 找到健康检查失败原因后再决定回滚 |
| `/api/status` 不通 | 容器健康、端口映射、OpenResty 代理 | 先在 VPS 本机查 `127.0.0.1:13080`，再查公网代理 |
| revision 不匹配 | GHCR 构建 run、`docker compose pull` 输出、镜像缓存 | 重新 pull 最新镜像并重建 `new-api` |
| CPU 偏高 | `docker stats`、`ps`、同机 `sub2api`/PostgreSQL/Redis | 区分业务流量、上游异常和共享服务压力 |

## 禁止事项

- 禁止继续把旧 host alias `racknerd` 当作生产部署目标。
- 禁止绕过 GHCR workflow，在 VPS 上本地构建生产镜像。
- 禁止在 `/opt/new-api` 目录保存生产 `.env` 以外的临时调试文件、压测产物或源码补丁。
- 禁止把只通过 `docker compose up -d` 视为发布成功；必须核验 revision、health 和 `/api/status`。
- 禁止在生产 VPS 上运行本地压测 SOP。压测仍按本地受控压测流程执行，不使用 RackNerd6C6G 作为压测机。

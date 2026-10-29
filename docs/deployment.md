# 生产部署

本文对应仓库自带的 Dockerfile、compose.yaml 和 Caddyfile.example。默认拓扑是一个 combined 应用容器、一个 PostgreSQL 容器，以及宿主机上的 HTTPS 反向代理。正式账号域名为 `https://id.duoyu.link`。

## 部署前准备

准备域名 DNS、可签发该域名证书的 HTTPS 入口、Docker 与 Compose、真实 SMTP 配置，以及数据库和数据卷的持久存储。首次部署应从干净的源码工作目录构建，环境文件与备份独立保管。

当前 Dockerfile 使用 Node 24、Go 1.27.3 和 Alpine 镜像，具体版本以文件为准。目标主机必须能取得这些镜像和依赖；配置存在不等于该主机已完成镜像构建或生产验收。

## 首次部署

以下示例在 Linux shell、仓库根目录执行。

```sh
git clone https://github.com/izcw/DuoYu-ID.git
cd DuoYu-ID
cp .env.production.example .env
chmod 600 .env
```

在本机编辑 `.env`，至少设置：

- `DUOYU_ORIGIN=https://id.duoyu.link`、`DUOYU_DEV=false`。
- 独立随机 `POSTGRES_PASSWORD`，建议 hex，避免 DSN 字符转义问题。
- 64 位随机 hex 的 `DUOYU_SECRET`，生成后持久保管，不随发布更换。
- 部署者自己的管理员邮箱及强初始密码。
- 真实 QQ SMTP 发件账号、授权码和 `SMTP_PLAIN_LOCAL=false`。
- 按实际代理链填写 `DUOYU_TRUSTED_PROXIES`，没有核实前保持空值。

详细字段见 [配置参考](configuration.md)。不要使用模板占位值启动生产。

```sh
docker compose config --quiet
docker compose build app
docker compose up -d postgres
docker compose run --rm -e DUOYU_MIGRATE_ONLY=true app
docker compose up -d app
docker compose ps
docker compose logs --tail=100 app
```

迁移模式只迁移数据库，不初始化管理员和签名文件；普通 app 启动才完成初始化。Compose 等待数据库健康检查。数据库没有发布主机端口，应用仅映射 `127.0.0.1:5252`；对外流量应由 HTTPS 代理进入。

Compose 的 `postgres` 卷保存数据库，`app-data` 卷保存签名文件和头像。卷名前缀由 Compose 项目名决定，后续发布保持项目名稳定；不要把带 `-v` 的删除卷操作当成普通升级。

## HTTPS 与代理

仓库的 Caddy 示例适用于 **Caddy 在宿主机运行**：

```caddyfile
id.duoyu.link {
    reverse_proxy 127.0.0.1:5252
}
```

用部署环境的 Caddy 服务加载此配置，并验证证书确实覆盖 `id.duoyu.link`。如果 Caddy 在容器中，127.0.0.1 指向代理容器自身；需要将代理与应用接入同一网络，并改用应用服务地址，不能直接照搬宿主机示例。

`DUOYU_ORIGIN` 必须与用户实际访问的 HTTPS origin 一致。反向代理到应用可以是内部 HTTP，生产外部入口仍是 HTTPS；WebAuthn RP ID 从 origin 主机名产生。

只有实际连接到 Go 的代理地址可加入可信 CIDR。Docker 场景看到的可能是网桥地址，不应盲目填写 127.0.0.1 或整个公网。代理要正确处理转发头，禁止绕过代理直接访问应用端口。

## 首次上线检查

```sh
curl --fail https://id.duoyu.link/api/auth/session
curl --fail https://id.duoyu.link/.well-known/openid-configuration
```

匿名会话接口正常返回不代表数据库、邮件和后台任务全都健康；它不是完整 readiness 探针。还需验证管理员登录、运行状态、数据库与 worker 更新时间，并检查 Discovery 中 issuer 和端点地址。

初始化管理员成功后，在 `.env` 清空 `DUOYU_ADMIN_PASSWORD`，执行：

```sh
docker compose up -d --force-recreate app
```

这不会删除已创建的管理员。修改 bootstrap 密码不改变现有密码，普通 restart 也不会重载容器环境。

接入两个正式 HTTPS 项目并验证注册、SSO、重新认证、续期、退出和停权。真实手机验证通行密钥与扫码；真实邮件由人工检查收件箱、垃圾箱及延迟。不要把 localhost 或虚拟认证器通过当作上线验收。

## 发布与升级

1. 选择明确的版本或提交，先在测试环境验证，构建新镜像。
2. 安排维护窗口，停止接收新写入；停止所有应用和 worker，保留数据库运行。
3. 对数据库、app-data 卷及环境密钥做一致备份，记录版本和恢复点。
4. 执行迁移；成功后启动新应用和 worker，核对日志、登录与队列。

默认单应用部署对应命令。先更新到已验收的代码版本，例如在部署分支执行 `git pull --ff-only`；有未提交修改时先处理，不强制覆盖。

```sh
docker compose build app
docker compose stop app
# 在此完成数据库与 app-data 一致备份，再继续
docker compose run --rm -e DUOYU_MIGRATE_ONLY=true app
docker compose up -d app
```

迁移有事务和校验和，不编辑已应用迁移；没有自动 down migration。升级后回退旧镜像前确认 schema 兼容，否则按经过演练的备份恢复流程回退。恢复出来的邮件队列可能对应已经发送的消息，审查后才能重新运行 worker。

无论修改 Vue、登录 CSS、Go 或模板，都要重新 `docker compose build app` 并 `docker compose up -d app`。Dockerfile 自动构建前后端；`docker compose restart app` 只重启旧容器，不构建新代码，也不更新环境变量。只改 `.env` 时使用 `docker compose up -d --force-recreate app`。

升级后检查：

```sh
docker compose ps
docker compose logs --tail=100 app
curl --fail https://id.duoyu.link/.well-known/openid-configuration
```

## 常用运行命令

在原部署目录执行，保持同一 Compose 项目名与数据卷：

```sh
# 持续查看日志，Ctrl+C 只退出日志查看
docker compose logs -f --tail=100 app
# 停止应用，数据库继续运行
docker compose stop app
# 恢复应用
docker compose up -d app
# 仅重启现有进程
docker compose restart app
```

前端与 Go 由同一个 app 镜像提供，不使用 `pnpm dev` 或 `pnpm serve` 作为生产服务器。

## 运行角色与长期运行

默认 app 为 combined。需要拆分时自行增加 api 与 worker 服务，统一数据库、服务密钥、签名数据与 SMTP 配置；只启 api 会导致队列积压。当前 Compose 未包含代理服务、容器重启策略、外部探活、离机备份或告警发送通道，需要部署侧配置并验证故障恢复。

监控写入数据库和审计，进程或数据库整体不可用时需外部监控发现。备份、轮换和恢复操作见 [运维与恢复](operations.md)。

## 不使用 Docker 的应用部署

也可在目标平台构建并运行 Go 二进制；数据库独立管理，HTTPS 代理要求不变。

```sh
pnpm --dir admin install --frozen-lockfile --ignore-scripts
pnpm --dir admin build
go build -trimpath -o duoyu ./server
```

将二进制与 `admin/dist/` 一起发布，并为 data 目录设置持久存储和运行用户写权限。进程工作目录应包含 admin/dist，不能只复制二进制；Go 模板嵌入二进制，但管理端与登录 CSS 等资源仍由磁盘提供。

由系统服务管理器注入环境变量，设置启动目录、重启策略及日志输出。二进制不自动读取 `.env`；Windows 可使用 `scripts/start.ps1 -EnvFile '你的生产配置文件'` 进行受控验证，但日常生产应配置稳定的进程托管。不要复用开发模式或开发数据库密码。

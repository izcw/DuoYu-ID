# 多鱼账号 · Duoyu ID

独立账号认证中心：Go + PostgreSQL + Vue 3 / TypeScript / Element Plus。后台基于 Art Design Pro 执行官方 `clean:dev` 后开发，保留其 MIT LICENSE 与基础布局组件。

后续界面修改须参考 [UI 规范](docs/design-system.md)，项目根目录 [AGENTS.md](AGENTS.md) 已记录该要求。Logo 和 favicon 使用用户提供的原始资源。

## 当前功能

- 邮箱验证码注册/登录、邮箱密码登录、找回密码；密码登录只需邮箱和密码（含管理员）。
- 新邮箱验证码验证成功后自动注册，无勾选框；后台关闭注册时仍拒绝创建新账号。
- 通行密钥注册、发现式登录、移除；注册和移除需邮箱验证。
- iPhone 扫码登录：一次性二维码、手机通行密钥验证、识别码核对、明确授权和电脑自动登录。
- 个人资料、修改邮箱/密码、登录会话管理；头像本地存盘，解码后裁剪为 256px PNG。
- 管理后台：真实账号、项目、准入名单、项目密钥轮换、API 凭证、审计日志、注册开关、邮件预览。
- OIDC Discovery、Authorization Code + S256 PKCE、签名 ID Token、UserInfo、JWKS、令牌撤销/内省。所有项目通过标准协议接入。
- 邮件使用 HTML + 纯文本双版本，QQ SMTP 465 TLS 配置已预留。

不包含业务数据库托管、文件托管、短信、QQ/微信登录。Passkey 是通行密钥，支持什么设备和跨设备扫码取决于浏览器及系统。

扫码功能的使用方法、LumeHub 样式与源码分析见 [扫码登录说明](docs/lumehub-reference.md)。手机实际使用需同一 HTTPS 域名；localhost 只适合本机自动化联调，不是 Apple 账号 OAuth 登录。

## 本地运行（Windows）

需要 Go 1.27.3+、Node 24、pnpm 10、Docker Desktop。

```powershell
cd D:\Users\Administrator\桌面\DuoYu\DuoyuID
# 首次生成随机管理员密码及服务密钥；已有 .env.local 时不要再次生成
./scripts/init-dev.ps1
docker compose -f compose.dev.yaml up -d
./scripts/start.ps1 -BuildWeb
```

本次实现已经生成 `.env.local`，并启动本地数据库与 Mailpit。当前两个开发容器名称为 `duoyu-postgres-dev`、`duoyu-mailpit-dev`；重启可执行 `docker start duoyu-postgres-dev duoyu-mailpit-dev`。不要与 compose.dev 同时占用相同端口。以后切换到 compose.dev 会使用独立的新数据库卷。

本次临时安装的 Go 可用以下命令启动（长期使用建议正式安装 Go）：

```powershell
./scripts/start.ps1 -Go "$env:TEMP\duoyu-go-runtime\go\bin\go.exe"
```

- 登录页：<http://localhost:8080/login>
- 管理员邮箱：`2405824084@qq.com`。密码查看本机 `.env.local` 的 `DUOYU_ADMIN_PASSWORD`，请勿把该文件发给他人。
- 选择“密码登录”并输入邮箱和密码即可。使用验证码登录时，开发验证码只到本机 Mailpit：<http://localhost:18025>，不会发到真实邮箱。
- 后台：<http://localhost:8080/#/manage/overview>
- 普通用户：<http://localhost:8080/#/account/profile>

前端由 Go 从 `admin/dist` 提供。修改前端后运行 `pnpm --dir admin build`；修改 Go 后重启服务。不要直接把 Vite 5173 当作认证入口：Cookie、Origin 与回调都以 `DUOYU_ORIGIN` 为准。

## 部署到正式域名

### 本机启用真实 QQ 邮箱

当前 `.env.local` 默认使用 Mailpit，因此 QQ 邮箱收不到开发验证码。测试收件箱为 <http://localhost:18025>。

运行 `./scripts/configure-mail.ps1`，按提示隐藏输入新的 QQ SMTP 授权码，然后重启服务。脚本只修改本机环境文件，不发送邮件。重新打开登录页请求验证码，即可使用真实 SMTP 投递。无需将新授权码发到聊天中。

### 正式部署

1. 将 `.env.production.example` 复制为 `.env`；设置正式 HTTPS 域名、随机数据库密码（建议仅 hex，避免 DSN 转义）、64 位随机 hex 的 `DUOYU_SECRET`、强管理员初始密码。
2. 设置新的 QQ SMTP 授权码到 `SMTP_PASSWORD`。此前聊天中出现过的授权码应在 QQ 邮箱中撤销后重新生成；代码没有硬编码它。
3. `docker compose up -d --build`。数据库不暴露到主机；服务只监听主机回环端口 8080。
4. 用 Caddy 或 Nginx 配置 HTTPS 反向代理，示例见 `Caddyfile.example`。启动参数中的 issuer 域名必须与公网访问域名完全一致。
5. 初始化管理员后，从环境配置中清空 `DUOYU_ADMIN_PASSWORD` 并重建 app 容器。该参数只在账号不存在时创建管理员，不会覆盖已有密码或自动把已有普通用户提升为管理员。
6. 备份 PostgreSQL、`app-data` 卷（签名私钥及头像）以及服务环境密钥。更换 `DUOYU_SECRET` 会使已有密钥摘要/验证码等失效，不能随意重生成。

生产配置不允许 HTTP。当前实现按单实例部署设计；不要让多个实例同时初始化同一签名文件。Docker 构建文件已提供；本机尝试构建时 Docker Hub 认证端点网络连接超时，尚未完成镜像构建验证。Windows 原生服务已验证，首次服务器部署仍需实际构建与 HTTPS 联调。

## 项目怎么接入

在“项目管理”创建应用，登记精确回调 URL。服务端应用妥善保管仅显示一次的 client secret；浏览器公共客户端没有 secret。两种类型都必须 S256 PKCE。

配置标准 OIDC 客户端：

```text
issuer:        https://你的账号域名
discovery:     https://你的账号域名/.well-known/openid-configuration
response_type: code
scope:         openid profile email
PKCE:          S256
```

接入方需要一次性实现跳转、回调、校验令牌和本地会话；此后修改中心页面、邮件模板和账号管理通常不需要项目改代码。项目用 `iss + sub` 识别账号，不用可变的邮箱作为主键。用户昵称/头像可从 UserInfo 获取。接口范围的破坏性变更仍需要版本管理，标准协议不能保证永远零适配。

“准入名单”是项目级允许/拒绝，不是各项目业务 RBAC。业务角色与数据放在项目自己的数据库中。API 凭证支持 `app:read`、`mail:send`、`mail:read`，按需授予；不能当用户 token 或管理员 token。旧凭证不会自动获得发信权限。

项目后端可统一调用多鱼发送通知、业务验证码和告警：纯文字使用默认模板，自定义 HTML 使用项目提供的内容，无需后台模板编辑。管理端“应用接入 → 项目邮件”查看发送元数据。详见 [项目邮件接入](docs/project-mail.md) 和 [现状评估与升级方案](docs/upgrade-plan.md)。已升级为加密持久队列，支持幂等及有限安全重试，终态清除正文；SMTP 接受不等于送达。部署、迁移和备份恢复见 [运维文档](docs/operations.md)。

API 凭证默认选择“永久有效”，也可指定 1–365 天。永久凭证仍可撤销，项目停用后无法调用。已有凭证保留原有效期；在编辑页勾选“修改有效期”后可转为永久。仅修改名称不会延长有效期。数据库以 `expires_at = NULL` 表示永久，启动时幂等更新字段约束。

管理端菜单按“工作台 / 应用接入 / 安全与设置 / 我的账号”组织。项目配置、准入名单、凭证创建与编辑、个人账号安全操作均使用独立路由页面，可直接访问并刷新。项目与凭证表单有未保存离开确认，创建或轮换的密钥仅在结果页显示一次。

概览提供 7 / 14 / 30 天中心登录成功及失败趋势、新增账号图表，以及项目累计授权排名。每日统计按北京时间分组；顶部指标为最近 24 小时，项目排名为 `app_users` 开始记录以来的累计授权码兑换量，不表示项目在线人数。

“项目管理 → 登录用户”展示该项目成功兑换 OIDC 授权码的账号、首次/最近登录时间与次数（支持服务端分页查询，从功能上线后开始记录）。统一用户标识为 `iss + sub`，不同项目由 `client_id` 区分；`app_users` 记录项目与用户的多对多登录关联，不作为授权依据。同一账号在 A、B 项目可有两条关联，项目 A 的本地用户记录和业务角色仍由 A 自己维护。中心直接注册的账号，在首次登录某项目后才出现在该项目的登录用户中。若需完全隔离、不同项目下同一邮箱算不同账号，应另行设计租户体系，当前实现不是这种模式。

示例请求（凭证放环境变量）：

```sh
curl -H "Authorization: Bearer $DUOYU_API_KEY" https://你的账号域名/api/service/app
```

提供可运行的 [Go 接入示例](examples/client/README.md)，可用两个端口验证跨项目登录。详细协议边界与测试说明见 [docs/architecture.md](docs/architecture.md)。后台“接入文档”可下载当前服务 API 的 OpenAPI JSON。

最新实现与正式域名部署事项见 [登录与接入升级交付记录](docs/session-upgrade-2026-10-27.md)。

最新界面更新、功能清单、多类型接入验证及改进路线见 [完整评估报告（2026-10-27）](docs/ui-review-and-roadmap-2026-10-27.md)。

## 验证

最新完整接入测试、实测限制与分阶段方案见 [多项目接入测试报告（2026-10-26）](docs/integration-test-report-2026-10-26.md)。

```powershell
go test ./server
$env:DUOYU_TEST_DATABASE_URL='postgres://duoyu:duoyu_dev_only@localhost:55432/duoyu?sslmode=disable'
go test -v ./server
go vet ./server
pnpm --dir admin build
# 三个临时项目：机密网站、公共客户端和纯前端 SPA；不发送邮件
go build -o test-results/oidc-client.exe ./examples/client
cd admin
node tests/multi-project.mjs
# 桌面/手机的全部在用管理视图、42px 登录控件和运行状态筛选
node tests/ui-refresh.mjs
# 真实 SMTP 环境只测试页面与密码登录，不发邮件
$env:SKIP_MAIL='true'
Remove-Item Env:TEST_PASSKEY -ErrorAction SilentlyContinue
node tests/smoke.mjs
node tests/session.mjs
# 使用本机开发数据库的临时账号验证扫码全流程，不发邮件；结束清理测试数据
node tests/qr.mjs
# 独立编辑页面、永久凭证、离开确认和概览图表；不发邮件
node tests/admin-pages.mjs
# 邮件默认模板桌面/手机视觉检查，不调用 SMTP
node tests/mail-template.mjs
```

集成测试使用随机 schema，不修改正式表。浏览器测试使用本机 Chrome（可通过 CHROME_PATH 指定路径）。上述命令不发送真实邮件；多项目与扫码测试使用开发数据库中的临时验证码和账号，管理页面检查会创建管理员会话。不要在生产数据库运行。截图与结果输出到被忽略的 `test-results/`。

## 目录

```text
server/        Go 认证、后台 API、SQL、邮件和登录模板
admin/         Art Design Pro 管理后台与账号中心
scripts/       开发初始化、启动脚本
docs/          架构与协议说明
data/          原生运行时的签名私钥、头像（不提交）
```

建议落地与验收：[2026-10-27 实现和测试报告](docs/roadmap-implementation-and-tests-2026-10-27.md)。新增权限与凭证 UI 回归：在 admin 目录执行 `node tests/roadmap.mjs`。

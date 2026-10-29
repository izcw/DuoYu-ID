# 多鱼账号 · DuoYu ID

多鱼账号是面向多个独立项目的统一身份认证中心。用户使用同一个账号登录接入项目；中心负责身份验证、项目准入、会话与账号安全，各项目继续维护自己的业务数据和业务权限。

后端使用 Go 与 PostgreSQL，管理端使用 Vue 3、TypeScript 和 Element Plus，基于 Art Design Pro 的干净版本开发。登录页保持简洁的黑白灰风格，管理表单采用独立页面。

## 功能概览

| 能力 | 当前支持 |
| --- | --- |
| 登录与恢复 | 邮箱验证码、邮箱与密码、通行密钥、手机扫码、身份验证器（Microsoft Authenticator 等）、找回密码 |
| 账号管理 | 验证邮箱后自动注册、注册开关、头像与资料、邮箱与密码修改、会话管理 |
| 统一认证 | OIDC Discovery、授权码 + S256 PKCE、ID Token、UserInfo、JWKS、刷新令牌、撤销与内省 |
| 项目接入 | 服务端网站、SPA、原生客户端；回调配置、准入名单、登录用户记录 |
| 管理权限 | 系统管理员、指定项目维护员、只读审计员 |
| API 凭证 | 按项目授权、永久或限时有效、撤销、最长 168 小时的轮换过渡期 |
| 统一邮件 | 通知、业务验证码与告警；默认文字模板或项目 HTML；持久队列与幂等请求 |
| 运行维护 | 审计与请求编号、退出通知重试、监控告警、迁移、备份与恢复校验 |

头像存放在中心本地数据目录。业务数据库托管、文件托管、短信和 QQ/微信第三方登录不在当前范围内。“iPhone 扫码”是本项目的二维码与通行密钥确认流程，不是 Sign in with Apple。

## 快速开始

**已有本地环境**：在仓库根目录运行，不要重新生成已有环境密钥。

```powershell
docker start duoyu-postgres-dev
./scripts/start.ps1
```

首次安装、Go 路径设置和真实 QQ SMTP 配置见 [开发与启动](docs/development.md)。启动脚本读取 `.env.local`；没有前端构建产物时自动安装依赖并构建，后续更新可使用 `-BuildWeb`。

| 入口 | 地址 |
| --- | --- |
| 登录 | http://localhost:5252/login |
| 管理概览 | http://localhost:5252/#/manage/overview |
| 个人账号 | http://localhost:5252/#/account/profile |
| OIDC Discovery | http://localhost:5252/.well-known/openid-configuration |

管理员使用初始化时配置的邮箱与密码，通过“密码登录”进入。普通账号不会获得后台管理权限。

## 修改、打包与部署命令

本地 PowerShell，在仓库根目录执行：

```powershell
# 前端打包；Go 已运行时构建成功后刷新页面
pnpm --dir admin build
# Go 启动，自动读取 .env.local；确保数据库已启动
./scripts/start.ps1
# 前后端一起更新：先 Ctrl+C 停止旧服务，再构建并启动
./scripts/start.ps1 -BuildWeb
# 可选：界面热更新，完整登录仍使用 Go 的 5252 入口
pnpm --dir admin dev
```

Go 路径设置、二进制运行和模板更新区别见 [修改后的命令速查](docs/development.md#修改后的命令速查)。

Linux 生产服务器首次部署：先按 [部署指南](docs/deployment.md) 配置 `.env`、持久卷和 HTTPS，再执行：

```sh
docker compose config --quiet
docker compose build app
docker compose up -d postgres
docker compose run --rm -e DUOYU_MIGRATE_ONLY=true app
docker compose up -d app
docker compose logs --tail=100 app
```

Docker 自动打包前端和 Go，不需要另开 Vite。已有部署的备份与更新顺序见 [发布与升级](docs/deployment.md#发布与升级)。

## 项目如何接入

1. 创建接入项目，选择平台与客户端类型，登记精确回调地址。
2. 项目通过标准 OIDC 客户端发起授权码流程，使用 S256 PKCE；服务端客户端另需保存 client secret。
3. 项目校验签名、issuer、audience、有效期、state 与 nonce，再建立自己的会话。
4. 用 `iss + sub` 关联本地账号；邮箱、昵称、头像作为可更新资料，不作为稳定身份主键。

```text
issuer:        https://id.duoyu.link
discovery:     https://id.duoyu.link/.well-known/openid-configuration
response_type: code
scope:         openid profile email
PKCE:          S256
```

中心页面和内部管理功能调整通常不需要项目重新适配；协议、声明和接入配置的破坏性变化仍需版本管理。项目内退出和中心退出含义不同，详见 [架构与认证边界](docs/architecture.md)。

- [Go 接入示例](examples/client/README.md)：运行两个项目验证 SSO 与退出通知。
- [项目邮件接入](docs/project-mail.md)：从项目后端使用独立 API 凭证发信。
- 管理端“接入文档”提供服务 API 说明与 OpenAPI 下载。

## 技术栈

| 层次 | 技术与职责 |
| --- | --- |
| 服务端 | Go、net/http，认证页面、管理 API、OIDC 与后台任务 |
| 数据库 | PostgreSQL 17、pgx，账号、会话、授权、队列与审计 |
| 认证 | zitadel/oidc、go-webauthn、RS256、Argon2id |
| 前端 | Vue 3、TypeScript、Vite、Element Plus、Art Design Pro |
| 图表与图标 | ECharts、本地 Iconify 图标集 |
| 邮件 | SMTP 隐式 TLS、HTML/纯文本双版本、PostgreSQL 加密队列 |
| 验证与部署 | Go 测试、Playwright、Docker、Compose、HTTPS 反向代理 |

工具链与版本来源见 [环境要求](docs/development.md#环境要求)。当前不要求 Redis、对象存储或独立消息中间件。

## 文档导航

| 文档 | 内容 |
| --- | --- |
| [文档索引](docs/README.md) | 按开发、接入与运维场景选择文档 |
| [架构](docs/architecture.md) | 组件、数据归属、认证与退出、权限边界 |
| [开发与启动](docs/development.md) | 首次安装、已有环境启动、更新与排错 |
| [环境配置](docs/configuration.md) | 环境变量、SMTP、运行角色与加载规则 |
| [生产部署](docs/deployment.md) | Docker、域名与代理、发布升级与上线验收 |
| [运维与恢复](docs/operations.md) | 队列、监控、迁移、备份与恢复 |
| [测试指南](docs/testing.md) | 测试条件、CI 与验收限制 |
| [项目邮件](docs/project-mail.md) | 权限、幂等、状态与限额 |
| [设计规范](docs/design-system.md) | 页面、组件与邮件样式依据 |

## 目录结构

```text
server/               Go 服务、登录模板、SQL 迁移、队列与监控
admin/                管理端、个人中心、静态资源、浏览器测试
examples/client/      OIDC 接入示例
cmd/rotate-key/       签名密钥轮换工具
scripts/              初始化、启动、邮箱配置、备份与恢复
docs/                 当前指南与带日期的历史报告
.github/workflows/    自动化测试配置
data/                 原生运行时签名密钥与头像，不提交 Git
```

## 验证状态与贡献约定

最近一轮本地验收包括 31 个协议场景、16 个多项目浏览器流程、管理端回归与隔离恢复演练，见 [实现与测试报告](docs/roadmap-implementation-and-tests-2026-10-27.md)。历史结果不代替生产 HTTPS、真实手机、真实收件或最新 CI 的实际验收。

修改前阅读 [AGENTS.md](AGENTS.md)，界面修改遵循设计规范。不要提交环境密钥、运行数据、私钥、备份和测试产物。保留依赖与上游版权声明，管理模板原许可证见 [admin/LICENSE](admin/LICENSE)。

浏览器扩展接入：见 [公共客户端、来源配置与续期规范](docs/extension-integration.md)。

身份验证器绑定、恢复码及项目策略见 [身份验证器登录](docs/authenticator.md)。

项目角色配置、默认分配与 OIDC 声明见 [项目角色](docs/project-roles.md)。

项目管理 API 与一键复制 AI 接入资料：见 [使用说明](docs/project-management-api.md) 和 [完整 AI 接入说明](docs/ai-integration.md)。

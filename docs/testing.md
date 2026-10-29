# 测试与验收

将后端隔离测试、开发环境浏览器联调、生产验收分开执行。测试不得向真实 SMTP 发送邮件，也不得在生产数据库创建测试夹具。

## 后端测试

在仓库根目录运行：

```powershell
go test -count=1 ./...
go vet ./...
```

没有 `DUOYU_TEST_DATABASE_URL` 时，数据库集成测试会跳过，不能把命令通过当作完整后端验收。准备本地 PostgreSQL 后：

```powershell
$env:DUOYU_TEST_DATABASE_URL='postgres://duoyu:duoyu_dev_only@localhost:55432/duoyu?sslmode=disable'
go test -count=1 -v ./...
go vet ./...
```

集成测试创建随机 schema，结束后删除；目标数据库账号需要创建 schema 的权限。SMTP 使用模拟传输，不依赖真实收件。若 Go 不在 PATH，使用实际 go.exe 的完整路径执行同样命令。

覆盖密码、代理、OIDC、PKCE、令牌与刷新重放、QR 归属、管理角色隔离、凭证轮换、邮件幂等与队列、迁移校验、退出通知、监控去重和数据库故障。具体当前场景以 `server/*_test.go` 为准。

## 构建检查

```powershell
pnpm --dir admin build
```

依次生成离线图标、进行 Vue/TypeScript 类型检查、构建管理端与独立 Element Plus Message 资源。构建通过不等于浏览器交互、生产网络或实际投递通过。

## 本地浏览器联调

前置条件：

- 当前开发服务运行在 `http://localhost:5252`，已有前端产物。
- `.env.local` 的开发模式与 origin 满足测试断言，管理员登录可用。
- 本机 Chrome 已安装；可用 `CHROME_PATH` 指定其他兼容 Chromium 路径。
- `multi-project.mjs`、`roadmap.mjs`、`qr.mjs` 等夹具直接操作名为 `duoyu-postgres-dev` 的 Docker 容器，库和用户均为 `duoyu`。不同容器命名或环境必须先调整夹具，不能直接运行到未知数据库。

在根目录构建接入示例，然后切换 admin 目录：

```powershell
New-Item -ItemType Directory -Force test-results | Out-Null
go build -o test-results/oidc-client.exe ./examples/client
Set-Location admin
$env:SKIP_MAIL='true'
Remove-Item Env:TEST_PASSKEY -ErrorAction SilentlyContinue
node tests/multi-project.mjs
node tests/ui-refresh.mjs
node tests/roadmap.mjs
node tests/smoke.mjs
node tests/session.mjs
node tests/admin-pages.mjs
node tests/qr.mjs
node tests/mail-template.mjs
```

| 脚本 | 检查重点 |
| --- | --- |
| multi-project | 两个 Go 项目与 SPA，完整登录、SSO、回调校验、退出、通行密钥与扫码 |
| ui-refresh | 登录 42px、桌面/手机管理页、溢出、菜单遮挡与运行时错误 |
| roadmap | 管理权限表单、维护员隔离、审计只读、配置检查与 API 轮换 |
| smoke | 登录切换、密码登录、资料和管理页面；必须使用 SKIP_MAIL=true |
| session | 匿名与无效 Cookie 不产生多余 /api/me 探测 |
| admin-pages | 独立编辑页、永久凭证、未保存离开确认与概览图表 |
| qr | 临时账号及虚拟认证器的扫码归属、确认与登录流程 |
| mail-template | 模板视觉检查，不调用 SMTP |

旧 smoke 中保留本地 Mailpit 分支，本项目真实 SMTP 环境不要启用；不要设置 `TEST_PASSKEY=true` 触发它。通行密钥流程使用不发邮件的独立矩阵和 QR 测试。

浏览器脚本可能创建会话及临时数据，部分脚本使用管理员权限。应检查最终退出码、清理结果和截图，不只查看终端中途的 PASS。截图与日志位于被忽略的 `test-results/`。

## CI

[工作流](../.github/workflows/test.yml) 在 push / pull_request 上运行 Go 测试、vet、前端构建与代表性浏览器回归。它使用独立 PostgreSQL 服务与随机临时配置，无 SMTP 密码。

`admin/tests/ci.mjs` 只允许 `CI=true` 且没有 `.env.local` 的隔离工作区，拒绝覆盖本地配置。CI 浏览器部分目前执行 session、ui-refresh、smoke；依赖本机 Docker 容器名称的全部多项目脚本并未原样放入 CI。查看具体提交的 Actions 结果才可确认该提交的 CI 状态。

## 生产与设备验收

本地不同端口是不同 origin、同一 site，不能替代两个正式站点的跨站测试。正式验收还需检查：

1. TLS、issuer、回调、可信代理、Secure Cookie 和 WebAuthn RP ID。
2. 两个真实站点的首次注册、SSO、强制认证、续期、退出和停权。
3. iPhone/Safari 与电脑浏览器的注册、取消、拒绝、过期、重复扫码及账号恢复。
4. 人工确认真实邮件的收件箱、垃圾箱与延迟，区分 queued、SMTP accepted 和实际收到。
5. 停 worker、数据库不可用、邮件不确定结果、退出通知失败，以及备份恢复后队列处理。

已有历史证据见 [实现与测试报告](roadmap-implementation-and-tests-2026-10-27.md)。压力测试、完整读屏审计、独立安全审计和 OIDC conformance 不在上述通过声明中。

## 账号策略回归

`server/password_policy_test.go` 覆盖 6–64 个 Unicode 字符、空格密码、10/60/360/2160 分钟锁定递增、并发失败、锁定期间正确密码阻止、成功密码登录清零、邮箱重置解锁、HTTP 429 与 Retry-After、并发分配唯一数字账号。

开发环境构建并启动后，在 `admin` 目录执行 `node tests/account-policy.mjs`，验证账号 ID、昵称编辑保存、按账号 ID 搜索，以及 1440px/390px 操作按钮同行和页面宽度。脚本使用临时账号并清理，不发送邮件。`node tests/mail-template.mjs` 同时检查两种模板没有 4px 顶部边框。


## 隔离回归与接口契约

本轮验收使用独立 PostgreSQL 容器、工作目录与 5253 端口；不复用开发用户数据。测试环境没有 SMTP 密码，浏览器设置 SKIP_MAIL=true。测试脚本默认仍采用 localhost:5252 与开发容器名；隔离执行时须同时替换副本中的端口及容器名，不要改正式配置。多项目退出通知测试需要 combined 角色，api 角色不会处理后台退出队列。

业务响应契约见 api-contract.md。Go 测试额外覆盖 JSON 尾随内容、多值输入和 UTF-8 截断，以及成功/错误字段、请求编号、版本和禁止缓存。浏览器测试使用虚拟认证器，不能代替实体 iPhone 和系统密钥提供程序验收。


统一开发者接入与 MCP 额外验收见 [实现报告](mcp-implementation-tests-2026-10-29.md)。`node tests/developer-mcp.mjs` 使用本地开发配置，验证 UI 与真实 MCP HTTP 调用；Go 的 mcp_test.go 覆盖迁移、官方 SDK、工具映射和授权边界。

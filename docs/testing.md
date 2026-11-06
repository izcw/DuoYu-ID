# 测试与验收

将后端隔离测试、开发环境浏览器联调、生产验收分开执行。测试不得向真实 SMTP 发送邮件，也不得在生产数据库创建测试夹具。

## 当前跨端隔离入口

同级 `DuoYu-Extensions` 提供 `test:auth:isolated`、`test:id:pages` 与 `test:admin:isolated`。
先构建 ID 管理/登录前端及业务管理前端，再在扩展根目录设置 `DUOYU_TEST_DATABASE_URL`
为本机专用 `duoyu_sync_test` 数据库，设置 `GO_BINARY` 为本机 Go 可执行文件路径。
夹具创建随机 schema、临时账号、独立浏览器资料和短期 TLS 证书；启动真实 ID、Go 业务服务与
Node 同步引擎，ID 使用 api 角色。无需读取 `.env.local`，不启动邮件 worker，结束后删除 schema。
真实认证/同步、身份中心 20 页及业务管理 13 页的双视口检查分别运行，也可执行
`node scripts/auth-isolated.mjs --id-pages --admin-pages` 合并检查；命令必须从扩展根目录运行。
页面检查覆盖加载、权限、响应式和运行时错误；隔离入口另含真实昵称保存/刷新、项目角色增改、默认角色、成员分配及删除后转移的数据库验证。
还包含项目 Logo 上传与创建/修改、准入去重及清空、黑名单持久化及清空、开发者凭证 HTTP/MCP 作用域即时变更、轮换和撤销，
注册开关修改后恢复、邮件模板预览 503 清除旧内容并重试恢复。以上写操作均限于测试 schema。
业务管理检查另含组件 CRUD、模板快照发布、壁纸上传和草稿/公开资源边界。
扩展目录的 `test:id:role-race` 覆盖角色组件乱序响应、旧请求失败、成员路由切换和卸载。
`test:id:management-race` 覆盖凭证加载、Logo 上传、配置检查和邮件预览的乱序、旧失败及卸载隔离。
同一脚本也覆盖项目、角色及开发者凭证的旧保存成功/失败、卸载、过期删除确认及旧轮换响应；
实际请求继续按原目标完成，过期结果不会改变新页面的表单基线、加载状态、秘密显示或路由。
这些覆盖仍不替代所有编辑操作、真实设备和实际邮件投递验收。

`test:id:security` 在相邻扩展项目提供无私密配置的安全交互矩阵：CDP虚拟WebAuthn注册、
二维码解码与不同浏览器会话的确认/一次性登录、TOTP绑定与登录/重放拒绝、恢复码消费与解绑后会话撤销。
验证码直接植入随机测试schema，身份服务仅运行api角色；不发送邮件，结果明确标记虚拟认证器和测试验证码，
不能用来证明真实邮件送达、iPhone或实体密钥兼容性。
下方依赖固定端口、Docker 和本地配置的脚本属于旧联调入口，自动化审查应优先使用上述隔离夹具。

## 依赖与静态检查

管理前端执行 `pnpm lint`、`pnpm test:dependencies` 与 `pnpm build`。
仓库 CI 已在构建前接入 lint 和依赖兼容检查；本轮仅本地验证，尚未执行远程 GitHub Actions。
依赖兼容检查覆盖现有中英文翻译、XLSX/XLS 导入导出往返、Commitizen 配置查找、ESLint TypeScript 诊断和 Stylelint Vue 配置。
用 `pnpm audit --registry=https://registry.npmjs.org` 查询官方审计接口；部分镜像没有审计端点，端点失败不能视为零漏洞。

SheetJS 使用官方 CDN 的 0.20.3 固定包，保留原导入导出组件；vue-i18n 使用修复版 11.4.18。
锁文件固定传递版本，两个 minimatch 覆盖只限于已验证使用命名导出/构造器的 TypeScript-ESLint 与 ESLint。
Vue Stylelint 推荐规则限定兼容现有 Stylelint 16 的版本。
截至本次审查，完整依赖审计仍有 1 项 Commitizen→glob→minimatch 3 开发工具中风险告警；
尝试更新 glob 导致实际 Commitizen 配置查找失败，已撤回该覆盖，未屏蔽告警。
这一开发工具不会进入浏览器发布包，仍需跟进上游兼容修复。

Go 使用 `govulncheck -json ./...` 检查实际调用链，须区分模块包含与函数可达；
本次 ID 的 `golang.org/x/crypto/openpgp` 只有模块级提示，没有导入或调用链，业务后端无发现。
以上结果是检查时的漏洞数据库快照，不能保证未来没有新增漏洞。

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

[工作流](../.github/workflows/test.yml) 在 push / pull_request 上运行 Go 测试、vet、前端构建与代表性浏览器回归。它使用本机一次性 PostgreSQL 服务中的 `duoyu_ci` 库。临时管理员配置仅传给测试子进程，服务监听随机端口，通行密钥使用 localhost 域名；测试不写入项目的 `.env.local`，服务使用 api 角色。

`admin/tests/ci.mjs` 只允许 `CI=true` 且没有 `.env.local` 的隔离工作区，拒绝覆盖本地配置。CI 浏览器部分目前执行 session、ui-refresh、smoke；依赖本机 Docker 容器名称的全部多项目脚本并未原样放入 CI。查看具体提交的 Actions 结果才可确认该提交的 CI 状态。

## 生产与设备验收

服务生命周期专项 `TestServeHTTP*` 使用真实本机 HTTP 延迟请求验证：停止监听后等待已接收请求完成；10秒宽限期耗尽则关闭连接并返回错误；监听失败直接返回。它不等同于已在部署服务器上测试系统信号和进程管理器。`TestIntegration/Queued_project_mail_authorization` 使用隔离 PostgreSQL 和替代邮件传输，覆盖十类授权/撤权状态，不调用真实 SMTP。

本地不同端口是不同 origin、同一 site，不能替代两个正式站点的跨站测试。正式验收还需检查：

1. TLS、issuer、回调、可信代理、Secure Cookie 和 WebAuthn RP ID。
2. 两个真实站点的首次注册、SSO、强制认证、续期、退出和停权。
3. iPhone/Safari 与电脑浏览器的注册、取消、拒绝、过期、重复扫码及账号恢复。
4. 人工确认真实邮件的收件箱、垃圾箱与延迟，区分 queued、SMTP accepted 和实际收到。
5. 停 worker、数据库不可用、邮件不确定结果、退出通知失败，以及备份恢复后队列处理。

已有历史证据见 [实现与测试报告](roadmap-implementation-and-tests-2026-10-27.md)。压力测试、完整读屏审计、独立安全审计和 OIDC conformance 不在上述通过声明中。

同级扩展仓库的 `pnpm test:linux` 可在无宿主端口、独立临时 PostgreSQL 的 Linux 容器中
运行身份/业务/同步服务全部后端测试和真实进程入口检查。ID 使用 api 角色，不启动 SMTP；
非法运行角色、端口被占用和 HTTP 排空错误应以非零状态退出，正常 SIGTERM 在清理后返回 0。
脚本检查前两类启动失败及正常 SIGTERM；排空超时由 Go HTTP 生命周期测试验证。
该测试不替代正式域名、实体认证器、真实邮件和生产恢复验收。

## 账号策略回归

`server/password_policy_test.go` 覆盖 6–64 个 Unicode 字符、空格密码、10/60/360/2160 分钟锁定递增、并发失败、锁定期间正确密码阻止、成功密码登录清零、邮箱重置解锁、HTTP 429 与 Retry-After、并发分配唯一数字账号。

开发环境构建并启动后，在 `admin` 目录执行 `node tests/account-policy.mjs`，验证账号 ID、昵称编辑保存、按账号 ID 搜索，以及 1440px/390px 操作按钮同行和页面宽度。脚本使用临时账号并清理，不发送邮件。`node tests/mail-template.mjs` 同时检查两种模板没有 4px 顶部边框。


## 隔离回归与接口契约

本轮验收使用独立 PostgreSQL 容器、工作目录与 5253 端口；不复用开发用户数据。测试环境没有 SMTP 密码，浏览器设置 SKIP_MAIL=true。测试脚本默认仍采用 localhost:5252 与开发容器名；隔离执行时须同时替换副本中的端口及容器名，不要改正式配置。多项目退出通知测试需要 combined 角色，api 角色不会处理后台退出队列。

业务响应契约见 api-contract.md。Go 测试额外覆盖 JSON 尾随内容、多值输入和 UTF-8 截断，以及成功/错误字段、请求编号、版本和禁止缓存。浏览器测试使用虚拟认证器，不能代替实体 iPhone 和系统密钥提供程序验收。


统一开发者接入与 MCP 额外验收见 [实现报告](mcp-implementation-tests-2026-10-29.md)。`node tests/developer-mcp.mjs` 使用本地开发配置，验证 UI 与真实 MCP HTTP 调用；Go 的 mcp_test.go 覆盖迁移、官方 SDK、工具映射和授权边界。

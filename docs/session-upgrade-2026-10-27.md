# 登录与接入升级交付记录

日期：2026-10-27。正式账号域名：`https://id.duoyu.link`。本地运行地址仍为 `http://localhost:8080`，没有将开发数据库或密钥上传到公网。

## 已实施

| 原问题 | 当前行为 |
| --- | --- |
| 多标签页覆盖登录流程 | 每个授权请求使用独立的短期 HttpOnly Cookie，最多保留约 8 个；完成后清除。中心和 Go 示例都支持同项目并行标签页 |
| 强制重新认证要求先退出 | prompt=login / max_age=0 直接显示登录表单，保留邮箱和原授权请求；完成新认证后继续回调 |
| 中心退出、停权与项目会话不联动 | 新 Access Token 绑定中心会话；会话删除时数据库级联撤销。示例进行在线 UserInfo 复核，同时提供带签名的后台退出通知接收端 |
| 无后台退出通知 | 持久退出事件队列，RS256 签名，iss/aud/iat/jti/sub/sid/events；后台接收端验签、校验用途、去重并按 sid 删除会话 |
| 无续期机制 | 项目可显式启用 offline_access；刷新令牌每次轮换，数据库只保存摘要，事务串行消费；重放撤销整个家族及关联 Access Token |
| 无静默认证 | prompt=none 在有效会话、项目准入及已有授权范围满足时自动完成；否则返回 login_required / interaction_required / access_denied，不显示登录表单 |
| 公共客户端与原生平台混淆 | 独立 platform 字段（web/spa/native）；原生应用支持 HTTPS、严格 IP loopback 及反向域名私有 scheme；公共客户端不存 client_secret |
| 管理查询最多 200 条 | 用户、项目、凭证、审计、项目用户、邮件页面使用服务端分页和查询；旧数组响应仍保留兼容 |
| 缺少接入运行检查 | 后台接入文档显示 issuer、当前签名 kid、退出通知积压/失败和邮件队列状态；HTTP 响应带 X-Request-ID |
| 固定签名密钥 | 增加轮换命令，保留旧公钥在 JWKS 中，旧令牌不会因正常换钥立即失效 |
| 过期记录持续堆积 | worker 每 10 分钟分批清理过期会话/授权请求/挑战/限流、旧无会话令牌，以及 30 天前成功退出通知、90 天前审计 |

个人中心原有会话列表和逐设备撤销继续保留；本次令牌绑定与退出通知使这些操作能影响接入项目。没有重复新建另一套会话管理。

## 项目接入配置

有后端的网站继续采用机密客户端 + Authorization Code + S256 PKCE。SPA 选择公共客户端和 SPA 平台。桌面/移动应用选择公共客户端和原生平台，使用系统浏览器。

原生 loopback 示例：`http://127.0.0.1:8199/callback` 或 `http://[::1]:8199/callback`。授权时允许动态端口，但 scheme、IP、路径仍受注册信息约束。生产原生 HTTP 不接受 localhost 域名、任意内网地址或通配符。私有 scheme 示例为 `com.example.app:/callback`，不能简单填 `app://任意地址`。这些协议路径已经测试，真实系统回调/Universal Links 仍需设备验收。

刷新令牌默认关闭，不改变旧项目行为。启用后显式请求 `scope=openid profile email offline_access`，建议同时使用 `prompt=consent`。即使首次登录刚完成，页面也会让用户确认该授权。刷新请求通过发现文档提供的 token_endpoint 提交：

```text
grant_type=refresh_token
client_id=你的项目ID
refresh_token=上次取得的刷新令牌
```

机密客户端另外使用 HTTP Basic 提交客户端密钥；公共客户端不提交密钥。每次成功后原子替换保存的新 refresh_token，禁止多个线程同时刷新同一个令牌。刷新 scope 只能缩小；过期、注销、停权或重放后重新交互登录。

刷新令牌绝对有效期不超过创建它的中心会话（当前 12 小时），不会无限滑动续期。它不是永久登录凭证。应用关闭续期开关后拒绝新的刷新请求。旧版本没有绑定会话的 Access Token 最多仍可有效 5 分钟；需要完整新行为的项目应重新登录。

## 后台退出通知

在项目编辑页填写 `backchannel_logout_uri`，例如 `https://app.example.com/auth/backchannel-logout`。接收方参考 `examples/client/main.go`：

1. 接收 application/x-www-form-urlencoded 的 logout_token；限定请求体大小。
2. 使用中心 JWKS 验证 RS256 签名、issuer 和本项目 audience。
3. 检查近期 iat、非空 jti/sub/sid、正确 events，拒绝 nonce；不能把普通 ID Token 当退出通知。
4. 按 sid/sub 清理对应服务端会话，重复 jti 幂等返回成功。

中心退出、逐设备撤销、密码变更引发的会话删除、停用用户/项目和撤销受限项目准入，均会生成相应通知。更改准入名单时保留的成员不会因为整表替换而误收到退出通知。停用项目、删除准入会持久撤销已有刷新令牌，重新启用不会恢复旧授权。

通知在数据库事务中入队；worker 通常每秒检查，可分离运行。失败指数退避，最多 10 次，进程崩溃后的租约到期可恢复；成功为 delivered，超过次数为 failed。不是无限重试，也不能承诺项目离线时立即退出。可通过管理员接口 `GET /api/admin/logout-events?page=1` 查询；接入文档页显示积压/失败计数。失败事件应排查接收端后处理，项目敏感操作仍建议在线复核。

生产回调强制 HTTPS，禁止 URL 用户信息、查询参数和跳转；拨号时校验并固定解析 IP，拒绝私网/回环地址，避免 DNS 重绑定。开发模式仅允许本机 HTTP，便于自动化测试。

**仅更新中心不可能自动修改未适配项目的本地 Cookie。** 接入方须采用在线检查或接收退出通知；示例已同时实现。JWT 离线验证本身不能证明账号此刻仍有效。

## 运维与迁移

新增编号迁移 003–006：会话绑定、刷新令牌/平台字段、退出通知队列与触发器、策略撤销。保留原 issuer/client_id/sub，不改用户业务数据。已在隔离 schema 验证新安装、重复迁移及校验；升级前备份在 `backups/pre-session-upgrade`，另恢复到独立 `duoyu_restore_session_upgrade` 数据库检查升级，没有覆盖运行库。

worker 角色现在同时运行邮件、退出通知和过期数据清理。设置 `DUOYU_ROLE=api` 时务必另启 worker；否则队列不会被消费。失败退出通知不自动删除；邮件任务/幂等记录不纳入本次通用清理，避免改变幂等语义。

密钥轮换命令：

```powershell
go run ./cmd/rotate-key -data data
```

先备份，轮换后重启所有 API/worker；上一把公钥保留在 `data/verification-keys`，当前运行实例的旧私钥在重启前仍可签名。保留公钥直到所有旧令牌和退出通知重试结束；不要在多实例间使用不同的数据/密钥副本。本次只在临时目录测试轮换，未主动更换运行环境密钥。

后台 `GET /api/admin/diagnostics` 需要管理员会话，不向公网匿名暴露内部计数。可将它和 /healthz 接入监控。建议对退出通知 failed>0、长期积压、邮件 unknown 增长报警；外部告警通道与定时备份调度仍需部署侧配置，当前没有擅自建立系统计划任务。

## 验证记录

- Go 全包测试、go vet、前端类型检查及两份生产构建通过。
- 协议覆盖新增：双标签并行、重新认证后继续、静默登录边界、刷新轮换/缩权/跨客户端/重放/并发/中心退出、签名退出重试、生产回调私网拒绝、超过 200 条的分页、原生动态端口完整兑换。
- 三个真实本地接入进程完成：机密网站、公共客户端、纯 SPA；Go 客户端和浏览器分别验证令牌签名。
- 浏览器覆盖同项目双标签、prompt=login、max_age=0、prompt=none、密码、邮箱自动注册、虚拟通行密钥、独立手机上下文扫码确认、中心退出和账号停权。
- 为确认通知机制独立有效，测试项目 B 关闭在线检查后，仍由后台通知清除了会话；伪造退出通知被拒绝。
- 管理页面、永久凭证、图表、个人安全页面、运行诊断及无正常匿名 401 的回归通过。
- 真实 SMTP 未调用。测试账号/项目使用一次性数据并清理。Go 测试输出见 `test-results/session-upgrade-go.log`，浏览器结果见 `test-results/multi-project-browser.json`。

## 正式部署尚需处理

`id.duoyu.link` 已有 DNS 解析；本机 HTTPS 校验返回 **RemoteCertificateNameMismatch**，当前服务证书与域名不匹配。已将 `.env.production.example` 和 `Caddyfile.example` 改为此域名，但没有修改远程服务器。需要在实际入口为该域名配置正确证书，并确认 Discovery 中 issuer 等于 `https://id.duoyu.link`。

还缺两个接入项目的 HTTPS 地址及服务器部署访问方式，无法完成真实跨站环境验收。真实 iPhone/Safari、系统密钥同步、真实邮箱收件/垃圾箱、生产压力容量仍是待验收项。正式 issuer/RP ID 与 localhost 不同，本地通行密钥不能直接迁移为正式域名密钥，应在正式域名重新注册。

细粒度管理员角色、项目内无跳转验证码登录、第三方授权治理、永久凭证重叠轮换、告警供应商和进一步前端资源拆分属于原方案的后续扩展，本次未将这些描述成已经完成。

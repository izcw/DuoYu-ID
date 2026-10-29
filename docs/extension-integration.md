# 浏览器扩展与公共客户端接入

本页是当前接入规范；2026-10-27 的评估报告保留历史事实。参考客户端为同级 DuoYu-Extensions 项目（src/features/auth、src/entrypoints/background.ts），无需复制密码表单，不持有 client_secret。

## 创建项目

选择公共客户端 → 浏览器扩展。填写 Chrome / Edge 的 32 位扩展 ID，点击“填入扩展回调与请求来源”。生成 `https://<ID>.chromiumapp.org/oidc` 与 `chrome-extension://<ID>`，保存前以 `chrome.identity.getRedirectURL('oidc')` 的实际结果核对。需要续期时主动开启“允许会话续期”，请求 `openid profile email offline_access`。

Firefox 使用其 identity.getRedirectURL() 返回的 HTTPS 地址及实际 moz-extension:// 来源，不套用 Chromium ID 规则。当前提供配置和协议来源测试，未完成 Firefox 真实授权窗口验收。不得用临时 manifest key 改变现有扩展身份来解决回调错误；安装目录、商店发布身份变化后检查回调和 origin。开发与生产使用不同客户端。

三个配置独立：

| 配置 | 作用 |
| --- | --- |
| redirects | 授权结果精确返回地址，必须完整匹配 |
| allowed_origins | 浏览器跨来源 API 请求的精确来源，无路径、查询、末尾斜杠或通配符 |
| logout_redirects | 中心退出后的返回地址，不等同于退出通知接收端 |

迁移 015 会一次性从旧回调提取 HTTP(S) 来源，保留已有网站接入；之后新增或修改回调不会自动变更 allowed_origins。新项目需主动登记来源。不带 Origin 的服务端请求不受浏览器来源限制，但仍须有效客户端、PKCE 和令牌验证。CORS 不是客户端身份认证，不能替代授权。

## 协议能力与错误

Discovery 只公布 code、authorization_code / refresh_token、openid / profile / email / offline_access、S256。刷新仍受项目开关限制。不公布 implicit、JWT bearer、设备码、phone/address 范围。端点地址从 Discovery 读取。

UserInfo 无效、过期、撤销令牌返回 HTTP 401、`{"error":"invalid_token"}` 和 `WWW-Authenticate: Bearer error="invalid_token"`；有效令牌缺少 openid 范围返回 403 insufficient_scope。来源拒绝返回 403 origin_not_allowed。数据库故障返回 503 temporarily_unavailable，不表示账号已失效。前端不要根据所有 4xx/5xx 一律退出。

## 续期与休眠

Access Token 为 5 分钟；中心会话绝对寿命 12 小时，刷新不会延长该期限。offline_access 不等于永久登录。长期设备登录、最大寿命和闲置超时尚未引入，不通过增大 Access Token 生命周期代替。

扩展使用唯一后台 Service Worker 协调刷新：

1. 页面通过 runtime.sendMessage 请求状态；后台串行队列读取 storage.session。
2. 刷新前持久保存 refreshPending=true，再发出请求。Worker 重启发现该标记时清除本地会话并要求重新登录。
3. 校验响应、身份与轮换令牌；一次 storage.session.set 原子保存整份新会话，清除 pending 标记，然后再查询 UserInfo。
4. 网络断开、响应丢失或 Worker 终止时不要重放旧刷新令牌。结果不确定应重新授权；中心 Cookie 有效时可以 SSO。
5. 普通 UserInfo 暂时 5xx/断网可保留尚未过期的本地展示，提示验证失败；业务操作仍须后端在线鉴权。

storage.session 限定 TRUSTED_CONTEXTS；禁止 localStorage、同步存储或备份保存令牌。休眠可恢复，扩展重载/更新或关闭浏览器会丢失会话。并发多页不能各自持有刷新协调器。

## 退出保证

| 客户端 | 中心退出后的同步方式 | 边界 |
| --- | --- | --- |
| 有后端网站 | 验签并幂等处理 back-channel logout，必要时在线复核 | 需处理重试、网络故障与本地 Cookie 清理 |
| SPA | 页面恢复/获得焦点和定时 UserInfo 检查 | 无后台接收端时不保证瞬时变化 |
| 扩展 | 可见、聚焦及可见时每 60 秒检查；后台集中刷新 | 关闭浏览器不持续检查 |

“退出本扩展”只清理本地会话、撤销本项目令牌；“账号与安全 → 撤销会话”影响对应中心会话及其派生令牌。头像显示登录不是业务授权凭据。黑名单、停权和中心撤销在在线检查时生效，离线客户端不能承诺即时退出。

## 复现

DuoYu-ID：设置隔离测试数据库后运行 go test -count=1 ./...，pnpm --dir admin build，再运行 admin/tests/multi-project.mjs 与 admin/tests/admin-pages.mjs。

DuoYu-Extensions：pnpm typecheck、pnpm zip、pnpm test:auth、pnpm test:auth:integration。集成脚本使用本地 Docker 开发数据库、临时测试账号和 Edge 真实扩展授权窗口，不读取中心秘密、不发送邮件；禁止用于生产。

当前运行配置与架构分别以 configuration.md、architecture.md 为准；QQ SMTP 是用户当前选择，Mailpit 仅是可选隔离开发工具，不是默认真实收件途径。

参考：[Chrome Identity](https://developer.chrome.com/docs/extensions/reference/api/identity)、[Firefox getRedirectURL](https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/identity/getRedirectURL)、[RFC 6750](https://www.rfc-editor.org/rfc/rfc6750.html)。


## 局域网地址与 issuer 一致

扩展配置的账号中心地址必须与 Discovery 的 issuer 完全一致，不能一个用 localhost，一个用局域网 IP。同一服务仅配置一个规范 issuer，不按来访 Host 动态切换，也不关闭客户端的 issuer 校验。

本机当前局域网联调配置：

```env
DUOYU_ADDR=0.0.0.0:5252
DUOYU_ORIGIN=http://192.168.1.16:5252
```

修改 .env.local 后重启 Go。核对：

```powershell
(Invoke-RestMethod 'http://192.168.1.16:5252/.well-known/openid-configuration').issuer
```

结果应为 http://192.168.1.16:5252；authorization_endpoint、token_endpoint、jwks_uri 等也应使用该 origin。然后在扩展中退出并重新登录，旧 issuer 签发的令牌不再用于新配置。HTTP IP 仅用于开发，通行密钥与正式部署使用同一 HTTPS 域名。局域网 IP 变化时，服务端和接入方需同步修改。

开发模式允许 localhost 和实际绑定本机网卡的私有 IP 作为 issuer。IP issuer 不初始化 WebAuthn（其 RP ID 必须为域名），相关接口明确返回 503；邮箱、密码、身份验证器和 OIDC 继续可用。需要通行密钥时改用统一 HTTPS 域名，而不是设置 IP 为 RP ID。

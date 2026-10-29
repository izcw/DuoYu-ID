# 身份验证器登录

多鱼账号支持 Microsoft Authenticator 和兼容 TOTP 的应用。此功能是独立的动态验证码登录方式，不是微软账号登录、微软推送批准，也不自动强制其他登录方式进行二次验证。

## 绑定与使用

1. 先通过已有方式登录多鱼账号，在“账号与安全 → 身份验证器 → 管理”进入独立页面。
2. 输入当前邮箱收到的验证码，生成有效期 10 分钟的绑定二维码。
3. 在 Microsoft Authenticator 中添加“其他账号”，扫码；无法扫码时手动填写页面上的密钥。
4. 输入应用中的 6 位动态码确认绑定。确认前该密钥不能用于登录。
5. 保存页面一次性显示的 8 个恢复码，也可下载文本文件。每个恢复码仅可使用一次。
6. 登录页选择“身份验证器登录”，输入邮箱或 10 位账号 ID 和动态码。刚用于绑定或登录的动态码不能重复使用，请等待应用刷新。

手机丢失时可点击“使用恢复码”登录。解绑需要新的邮箱验证码与动态码或未使用的恢复码；解绑后中心会话与访问令牌撤销，需重新登录。更换验证器先解绑再绑定。若动态码与恢复码都丢失，可用已有其他方式登录，但不能仅凭旧会话解除绑定；当前没有自动邮箱单因素解除验证器的接口。

## 项目管理

项目编辑页的“允许的登录方式”增加 `totp`。关闭后隐藏入口，后端也检查登录、SSO、授权码兑换、刷新和 UserInfo。全局账号页仍可管理验证器，不提供项目登录开关。

新项目默认全部开启。017 迁移只给此前 password/code/passkey/qr 全开的项目追加 totp；原有受限策略和空列表保持不变。可在项目管理中自行启用。

TOTP 登录会话 AMR 为 `otp totp`，与邮箱的 `otp` 区分。恢复码走同一项目策略，使用后立即失效；它不代表额外的验证因素。

## 安全与运维

- TOTP：SHA-1、6 位、30 秒周期，允许前后一个周期的时钟偏差；服务器和手机应自动校时。
- 每个账号随机生成 160 位密钥，AES-GCM 加密保存，并用用户 ID 绑定密文；加密密钥由 DUOYU_SECRET 域分离派生。
- 绑定与确认绑定关联当前中心会话，待确认密钥 10 分钟过期，维护任务定期清理。
- 数据库行锁串行消费动态码和恢复码，防止并发重放。连续验证失败 5 次锁定验证器 10 分钟；另有 IP 与账号请求限流。
- 恢复码每个 80 位随机值，只保存带服务端密钥的摘要。绑定完成后不再提供明文查询。
- 解绑撤销中心会话、访问令牌，已有的会话撤销机制处理续期和退出通知。项目自身会话需继续遵守在线验证或退出通知策略。
- 备份数据库时同时保管原 DUOYU_SECRET；随意更换该值会导致既有绑定密钥无法解密。
- 动态码、密钥、二维码内容和恢复码不得写入日志、审计详情或公共截图。

算法依据 [RFC 6238](https://www.rfc-editor.org/rfc/rfc6238)；第三方账号绑定方式见 [Microsoft Authenticator 官方说明](https://support.microsoft.com/en-us/authenticator/how-to-add-your-accounts-to-microsoft-authenticator)。

## 接口

个人管理接口要求已登录与同源请求头；写入接口沿用 X-Duoyu-Request: 1。

| 接口 | 用途 |
| --- | --- |
| GET /api/me/authenticator | 查询绑定状态与剩余恢复码数量 |
| POST /api/me/authenticator/begin | code 为当前邮箱验证码，purpose=email；返回绑定图片和手动密钥 |
| POST /api/me/authenticator/finish | code 为动态码；成功后返回一次性的 recovery_codes |
| DELETE /api/me/authenticator | email_code、code、recovery（是否使用恢复码） |
| POST /api/auth/authenticator | account（邮箱或账号 ID）、code、recovery、auth_request_id |

接入项目仍优先使用标准授权码流程，auth_request_id 必须属于同一浏览器。不要把浏览器跨站直传动态码当作受支持的嵌入式登录接口。

## 更新、验证

先备份，按开发或生产部署文档构建前端并重启 Go；启动自动应用 017 迁移。前后端应同时更新。

```powershell
./scripts/start.ps1 -BuildWeb
```

测试命令（隔离数据库测试，页面测试仅用于已有本地开发环境）：

```powershell
$env:DUOYU_TEST_DATABASE_URL='postgres://duoyu:duoyu_dev_only@localhost:55432/duoyu?sslmode=disable'
go test -count=1 ./...
$env:SKIP_MAIL='true'
Set-Location admin
node tests/authenticator.mjs
node tests/admin-pages.mjs
```

authenticator.mjs 创建并清理临时用户与项目，通过数据库注入测试邮箱挑战，拦截发信接口，不发送真实邮件、不切换 SMTP。覆盖二维码参数、绑定、动态码/恢复码登录、公共及机密客户端授权、策略关闭和解绑；Go 测试覆盖 RFC 向量、加密篡改、重放、并发、限流锁定和会话撤销。

真实 Microsoft Authenticator 扫码和公网 HTTPS 仍需用实际手机验收；自动化计算动态码通过不能替代真机验证。

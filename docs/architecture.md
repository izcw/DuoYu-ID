# 认证与安全边界

## 数据归属

多鱼账号保存账号、密码摘要、头像、中心会话、通行密钥、应用注册信息、准入名单、API 凭证摘要与审计。项目保存业务数据和自己的登录会话。数据库中的用户 ID 稳定且不随邮箱变化。

## 一次登录流程

1. 项目生成并保存随机 state、nonce、PKCE verifier，跳转授权端点。
2. 多鱼账号检查回调地址、PKCE 与范围，展示项目名和将共享的资料。已有中心会话时点击继续；没有则登录。
3. 返回一次性授权码。项目后端/客户端带原 verifier 兑换 token；confidential 应用还须 HTTP Basic client secret。
4. 项目使用 OIDC 库校验签名、issuer、audience、expiry、nonce，并建立自身会话；不得只解析 JWT 或只信任前端传来的用户 ID。
5. 用户访问另一个项目时复用中心会话，仍会显示项目与继续按钮。

访问令牌为 opaque，5 分钟；ID token 5 分钟。中心会话 12 小时。授权请求 10 分钟。支持 openid/profile/email，项目可启用 offline_access 和轮换 refresh_token（绝对寿命受中心会话限制），不提供隐式流程、密码 grant 或设备码流程。prompt=none 在已有授权范围和有效会话满足时完成，否则返回标准错误；尚未通过完整 OIDC conformance 认证。

## 撤销与退出

禁用账号/项目、移除准入权限后，新的授权和 UserInfo/内省被拒绝。修改密码/邮箱、管理员撤销会话会撤销中心会话与存储的访问令牌。

已经签出的 ID token 在到期前仍可能被离线校验接受；各项目自己建立的会话不会被中心自动删除。已实现可配置的签名 back-channel logout 和持久重试队列，项目需接收通知并清理本地会话。需要及时停权的项目应每次重要操作调用 UserInfo/内省，或使用较短本地会话期限。不能将“中心退出”描述成所有项目立即退出。

## 已实现的防护

- Argon2id 密码摘要；随机会话、项目 secret、API key 只存摘要；验证码为 keyed HMAC。
- 数据库行锁保证验证码单次消费；5 次错误锁定；按邮箱和来源 IP 的持久化速率限制。
- 同源 Cookie，生产 Secure / HttpOnly / SameSite=Lax；管理 API 检查 Origin 与自定义请求头。
- 密码登录与邮箱验证码登录独立；通行密钥要求 user verification。
- 精确回调白名单、S256 强制 PKCE、一次性授权码；私钥保存在数据目录。
- 头像只接受 JPEG/PNG/WebP，限制文件和像素，重新编码，拒绝 SVG。
- 管理员访问后端二次鉴权；API key 绑定单项目、明确授权范围、可选永久有效及撤销。项目邮件入口仅使用 Bearer 权限鉴权，不使用 Cookie；其他管理写接口仍检查同源。

反向代理 IP 使用 DUOYU_TRUSTED_PROXIES 白名单解析 X-Forwarded-For；未配置时只使用连接地址。配置及测试见 operations.md，不信任任意外部转发头。

## 运维与后续工作

- `schema.sql` 为不可变迁移基线，schema_migrations 记录版本和校验和；新增字段必须添加编号迁移。脚本及备份恢复见 operations.md。
- 目前不自动清除历史日志和旧头像；需要根据磁盘容量、审计保留期制定清理策略。过期会话、授权记录和令牌不会再次被接受，但占用存储。
- 签名 key 固定持久化，尚未实现多 key 的平滑轮换。
- QQ SMTP 真实投递、正式域名 TLS、iPhone/Android 实际通行密钥扫码需在对应环境验证；虚拟浏览器测试不能替代设备测试。
- 管理列表一次最多返回 200 条，适合初期使用；规模扩大后补服务端分页与查询。
- 本项目尚未经过独立安全审计或 OIDC conformance suite。上线前应完成自己的域名、备份恢复、邮件投递和负载验收。

## 测试覆盖

Go 集成测试：两个项目 SSO、ID token 签名/claims、UserInfo、授权码重放、非法回调、无 PKCE 拒绝、OTP 并发单次使用和错误锁定、CSRF、管理员权限、API key 撤销、账号禁用。

浏览器 smoke：桌面/移动登录、本地 SMTP 邮件、管理员登录、个人账号页与全部后台页、页面运行时错误检查。可选 TEST_PASSKEY=true 已通过 Chromium 虚拟认证器的注册、发现式登录和删除测试。真实硬件通行密钥与真实 QQ 邮箱尚不在这项 smoke 中。

本次会话绑定、续期、退出事件、原生客户端和部署限制详见 [升级交付记录](session-upgrade-2026-10-27.md)。

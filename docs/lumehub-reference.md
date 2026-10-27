# LumeHub 样式与扫码登录分析

参考源码：
- https://github.com/izcw/LumeHub/blob/main/Frontend/src/components/modals/LoginModal.vue
- https://github.com/izcw/LumeHub/blob/main/Backend/internal/api/auth_passkey_qr.go
- https://github.com/izcw/LumeHub/blob/main/Backend/internal/store/passkeys.go

## 视觉

LoginModal 使用 380 × 420 的 Dialog；正文横向留白约 30px，输入框间距 14px，按钮铺满表单宽度，辅助文字为灰色。二维码模式替换表单主体，用右下卷角按钮切换，二维码下显示状态和倒计时。部分“注册/忘记密码”文字在源码中是 aria-hidden 的静态装饰。

多鱼账号沿用白底、黑色按钮、细边框、小圆角和灰色状态文字，但保留真实可操作的注册、密码重置和通行密钥入口。未复制对方 Logo、备案号或源代码。

## 扫码原理

LumeHub 自行创建 90 秒二维码会话：二维码是带 ticket 的网页 URL；手机打开确认页，通过 WebAuthn 通行密钥签名；电脑定期轮询并领取登录结果。这和浏览器原生 FIDO 跨设备二维码是两条不同流程，也不是 Sign in with Apple。

多鱼账号新增同类手机网页授权流程，但使用现有 go-webauthn 库验签，并将状态持久化到 PostgreSQL。二维码 120 秒过期，电脑使用 HttpOnly Cookie 证明会话所有权；手机 ticket 放在 URL fragment，进入手机页后清除地址栏；签名验证后必须明确确认账号及识别码。只有新完成的通行密钥登录能批准，密码会话或旧会话不能批准。电脑使用事务单次领取，并获取自己的 HttpOnly 会话 Cookie，不向脚本返回 bearer 登录 token。

## iPhone 使用方式

1. 将多鱼账号部署到电脑和手机均能访问的同一个 HTTPS 域名。
2. 用 iPhone Safari 打开该域名并登录，在“账号与安全”添加通行密钥，保存到系统“密码”中。也可在电脑添加时选“iPhone / 其他手机”，通过浏览器支持的跨设备保存流程注册。
3. 电脑登录页选择“iPhone 扫码登录”，用系统相机打开二维码。
4. 手机上点击通行密钥验证，通过 Face ID、Touch ID 或设备密码后，核对电脑识别码并明确确认。

此流程也可用于支持 WebAuthn 的 Android 手机。手机没有已绑定的密钥时不会自动注册账号。原有“使用通行密钥登录”继续走浏览器原生登录流程。

localhost 二维码仅能用于自动化联调，手机相机会把 localhost 解释为手机自身，不能用于两端实际扫码。注册于 localhost 的密钥不能移到正式域名复用，需要在正式域名重新绑定。

跨设备注册的 hybrid hints 只是浏览器提示，无法强制浏览器使用 Apple 设备。实际 iPhone 扫码和 iCloud 密钥同步仍需设备联调。

官方参考：
- https://developer.apple.com/passkeys/
- https://support.apple.com/en-euro/guide/iphone/iphf538ea8d0/ios
- https://github.com/w3c/webauthn/wiki/Explainer%3A-Hints-for-WebAuthn-Clients

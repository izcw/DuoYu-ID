# Go 接入示例

使用 coreos/go-oidc 和 oauth2 标准客户端库验证签名、issuer、audience、nonce、PKCE 与 UserInfo。仅供本机联调；内存会话、localhost Cookie 不应照搬到生产。

1. 在管理后台创建两个“公共客户端”，回调地址分别为 `http://localhost:8091/callback` 和 `http://localhost:8092/callback`。
2. 在项目根目录的两个终端分别运行：

```powershell
$env:CLIENT_ID='第一个项目的 client_id'
$env:CLIENT_PORT='8091'
go run ./examples/client
```

```powershell
$env:CLIENT_ID='第二个项目的 client_id'
$env:CLIENT_PORT='8092'
go run ./examples/client
```

访问第一个示例完成登录，再访问第二个示例，中心会话有效时只需点击继续。两个示例使用各自的 Cookie 名，不共享项目本地会话。

服务端客户端也可测试：在后台创建对应类型，设置 `CLIENT_SECRET`，示例将使用 HTTP Basic 验证客户端。secret 不得放进浏览器脚本。

## 会话与退出

示例默认在已登录页面访问时通过 UserInfo 在线复核，中心退出/停权后清理本地会话。项目后台退出通知地址填写 `http://localhost:示例端口/backchannel-logout`（仅开发模式）；示例验签并校验 sid/sub/events/jti/iat，重复事件幂等。`CLIENT_ONLINE_CHECK=false` 仅用于测试通知独立生效，正式项目敏感操作仍建议在线复核。

示例仍使用进程内会话、localhost HTTP Cookie，不能原样部署生产；生产使用持久共享会话、HTTPS 和 Secure Cookie。`/login?prompt=login`、`/login?max_age=0`、`/login?prompt=none` 可验证重新认证和静默认证。更多部署要求见 [升级交付记录](../../docs/session-upgrade-2026-10-27.md)。

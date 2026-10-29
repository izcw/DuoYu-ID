# 项目邮件接入

多鱼账号统一连接 SMTP。项目后端通过 API 发送通知、业务验证码和告警，无需各自配置邮箱服务器。管理端只配置 API 凭证权限并查看发送记录，不制作、保存或编辑项目模板。

## 开通

1. 注册接入项目。在“开发者接入 → 凭证与 MCP”创建专用凭证，授予 `mail:send` 和 `mail:read`，可永久有效。已有凭证保持原权限，需主动编辑授权。
2. 将凭证保存在项目后端的环境变量中。它不是 OIDC client secret，也不是用户 access token，不能放浏览器或手机应用代码中。
3. 接口统一为生产 HTTPS 域名；开发环境 localhost 可使用 HTTP。服务端 API 使用 Bearer 鉴权，不需要伪造 Origin 或 Cookie，其他管理 API 的同源检查保持不变。

## 默认模板

```http
POST /api/service/mail
Authorization: Bearer YOUR_API_KEY
Content-Type: application/json
Idempotency-Key: order-123-completed-v1

{
  "to": "user@example.com",
  "subject": "任务处理完成",
  "text": "你的任务已完成。\n请登录项目查看结果。",
  "category": "notification"
}
```

没有 `html` 或 `html` 为空时，使用多鱼的白底、黑色标题、细边框邮件模板。正文按纯文字转义，保留换行，不解析 Markdown 或模板表达式。项目名称来自凭证所属项目，不能通过请求伪造。发件人固定使用多鱼配置的 SMTP 账号。

## 自定义 HTML

请求字段同上，用 `html` 传入项目自己生成的完整 HTML。可以同时提供 `text` 作为纯文字替代内容；未提供时使用简短的 HTML 查看提示。HTML 不二次套壳，不执行模板变量，不在管理端渲染，不主动抓取图片或附件。项目自行处理变量转义、布局和邮件客户端兼容性。

```json
{
  "to": "user@example.com",
  "subject": "设备告警",
  "category": "alert",
  "text": "设备 A 离线，请检查。",
  "html": "<!doctype html><html><body><h1>设备告警</h1><p>设备 A 离线，请检查。</p></body></html>"
}
```

`category` 可为 `notification`（默认）、`verification`、`alert`。类别只是用途标记，不改变邮件优先级或登录权限。业务验证码由接入项目生成、保存摘要、验证、控制次数和有效期；发送 `verification` 邮件不会注册多鱼账号，也不会创建可用于多鱼登录的验证码。

## 结果与幂等

```json
{"code":200,"msg":"成功","data":{"id":"mail_xxx","status":"queued","error_code":""}}
```

不能仅凭 HTTP 200 判断投递成功，必须检查 `data.status`。

| 状态 | 含义与处理 |
| --- | --- |
| queued | 已持久化，等待后台发送 |
| retry | 确认未提交正文的临时失败，等待重试 |
| failed | 永久失败或重试耗尽，正文已清除 |
| expired | 超过有效期，未继续发送 |
| cancelled | 验证码失效或项目/凭证权限已撤销 |
| sending | 同一幂等请求仍在执行；查询状态，不重复生成请求 |
| accepted | SMTP 已接受；不保证进入收件箱或已读 |
| unknown | SMTP 失败、断连或记录更新结果不确定；不自动重发，人工确认 |
| limited | 超过发送配额，没有调用 SMTP；等待配额恢复，再使用新幂等键 |

用 `GET /api/service/mail/{id}` 查询，需要 `mail:read`，只能查询同一项目的记录。项目停用、凭证撤销或到期会拒绝后续调用。

每条业务消息使用独立的 `Idempotency-Key`（8–128 字符），同一项目内跨凭证去重。请求超时后用原键、原正文重试只返回已存在状态，不再次发送。原键配合不同内容返回 409。不要每次网络重试生成新键。幂等记录目前不自动清理，永久删除记录会失去该键的去重能力。

## 限额与数据

- 每次仅一个收件人；主题最多 200 UTF-8 字节且不可换行；text 最多 16 KiB，html 最多 64 KiB，请求体最多 128 KiB。
- 默认每项目 10 次/分钟、100 次/24 小时，全局项目邮件 500 次/24 小时，同一收件人 3 次/分钟。计数使用滚动起点固定窗口；已计数但后续环节被拒绝的尝试也占用预算。幂等重放不再次消耗发信预算，但仍计入 API 请求频率。
- 可用 `MAIL_APP_PER_MINUTE`、`MAIL_APP_PER_DAY`、`MAIL_GLOBAL_PER_DAY` 配置前三项，重启生效。这是产品保护值，不是 QQ 邮箱承诺的发送额度。
- 登录验证码沿用独立接口与限流，认证和项目各自有 worker；共享 SMTP 账号及进程资源，不保证完全资源隔离。
- 记录保存项目、凭证 ID、收件邮箱、类别、格式、时间、状态以及正文的 HMAC；日志和元数据不保存正文；队列以 AES-GCM 加密暂存正文、验证码与 HTML，终态或过期后清除，不在日志返回 SMTP 密码与原始错误。
- 管理端记录列表支持服务端分页；兼容的无分页请求最多返回最近 200 条。不提供批量群发、附件、自定义发件地址、模板后台编辑或不确定结果的自动重发。

## 队列可靠性边界

当前使用 PostgreSQL 持久队列。queued/retry 任务在重启后继续执行；发送中断超过 2 分钟转为 unknown，不自动重发。SMTP 正文提交前的明确临时失败最多尝试 3 次，正文提交后的不确定结果保守处理。无法严格保证 SMTP 端“恰好发送一次”。

业务 verification 在队列中最多保留 5 分钟，其他项目邮件 24 小时；账号验证码按实际 challenge 过期时间处理。项目业务验证码有效期仍由项目管理。需要真实送达/退信状态时，还需供应商回执。部署角色、加密密钥、迁移及恢复步骤见 [运维文档](operations.md)。

统一凭证若未绑定单一旧项目，HTTP 请求必须携带 `X-Duoyu-Project: 项目ID`。MCP 使用 send_mail/get_mail_status 并传 project_id；与 HTTP 共用授权和队列，详见 developer-access.md。

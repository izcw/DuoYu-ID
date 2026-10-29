# 业务 API v1 契约

适用 `/api/` 下由业务处理器返回的 JSON；OIDC/OAuth、JWKS、OpenAPI 文档、HTML、图片和文件保持各自标准格式，不能套用业务信封。框架层错误仍需按 HTTP 状态处理，客户端不可假设所有非 2xx 都有 JSON。

```json
{"code":200,"msg":"成功","data":{},"error":null,"request_id":"请求编号","api_version":"1"}
```

```json
{"code":429,"msg":"请稍后重试","data":null,"error":{"code":"rate_limited","retryable":true},"request_id":"请求编号","api_version":"1"}
```

- 采用唯一的 code/msg/data/error/request_id/api_version 结构，不提供旧版字段别名或兼容模式。HTTP 状态是成功/失败的主要依据；code 与 HTTP 状态一致。msg 仅用于展示，不得通过中文提示判断业务逻辑。
- error.code 是稳定的错误类别：invalid_request、unauthenticated、forbidden、not_found、conflict、precondition_failed、rate_limited、service_unavailable 等。它不代替具体业务状态，例如邮件的 data.status。
- request_id 与 X-Request-ID 对应；幂等重放可能返回最初响应的请求编号，当前请求头编号用于当前访问日志。不得把编号作为身份凭证。
- X-Duoyu-API-Version 与 api_version 当前为 1。业务响应禁止缓存。所有接入方按本契约实现，并忽略未来新增响应字段。
- 请求使用 UTF-8 JSON，不支持拼接多个 JSON 值或尾随垃圾；未知字段拒绝并返回 400，防止拼写错误被静默忽略。密码等敏感内容禁止写日志。日期为 RFC3339；标识符按字符串处理。
- 新增响应字段及可选能力属于向后兼容变更。删除/重命名字段、修改字段类型、改变状态语义或认证方式属于破坏性变更，必须新建主版本并保留旧版迁移窗口，不能仅更新版本号后覆盖现有行为。
- 现有路径为 v1 契约的固定入口，本轮不改 URL。将来破坏性升级采用显式新版本路径，并为旧版提供迁移文档和弃用时间。不能声称格式统一后即可永久避免兼容问题。
- 网络失败和未知提交结果禁止盲目重试创建操作；只有幂等请求可结合 Retry-After、退避及原幂等键重试。并发项目修改继续使用 If-Match。

验证：server/http_response_test.go、server/request_parsing_test.go，及项目管理 API / 多客户端浏览器回归。OpenAPI 的响应结构与当前实现同步维护。

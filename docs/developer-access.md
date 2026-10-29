# 开发者接入：HTTP API 与 MCP

开发者接入 → 凭证与 MCP 统一管理凭证。项目管理、邮件服务和 AI 工具共用 api_keys、项目授权范围、有效期、撤销、轮换及审计。OIDC 用户登录客户端的 client_id/client_secret 仍独立。

## 创建与授权

选择名称、七类权限、指定项目或全部项目、永久/限时有效。默认只授予项目读取/创建/修改，不授予邮件发送或客户端密钥轮换。凭证只显示一次；创建和轮换后的密钥保存在后端秘密配置，不提交 Git。编辑支持替换权限、项目范围和有效期；轮换可立即失效或通过 HTTP 管理接口指定 0–168 小时过渡期。撤销对所有调用入口生效。

| 权限 | 用途 |
| --- | --- |
| projects:read | 查询项目、角色、准入、黑名单、配置诊断、接入说明 |
| projects:create | 新建项目与上传 Logo |
| projects:update | 修改项目、角色、准入、黑名单及上传 Logo |
| projects:rotate | 轮换 OIDC 客户端密钥 |
| app:read | 项目服务资料 |
| mail:send | 项目邮件入队 |
| mail:read | 查询本项目邮件状态 |

HTTP 创建/修改项目继续使用 /api/management/projects；服务接口继续使用 /api/service/app、/api/service/mail 和 /api/service/mail/{id}。统一后台凭证 API 是 /api/admin/developer-keys（会话认证、CSRF、防管理员越权）。PUT /{key} 编辑，POST /{key}/rotate 轮换，POST /{key}/revoke 撤销。

多项目凭证调用服务接口时必须传 `X-Duoyu-Project: 项目ID`。项目范围与 scope 同时检查；不能通过替换目标 ID 访问未授权项目。新建项目自动授予创建凭证访问权。

```sh
curl https://id.duoyu.link/api/service/app \
 -H "Authorization: Bearer $DUOYU_DEVELOPER_KEY" \
 -H "X-Duoyu-Project: your_project_id"
```

## MCP

远程入口 https://id.duoyu.link/mcp，基于官方 Go SDK，支持无状态 Streamable HTTP（JSON 响应）和手动 Bearer 请求头。每次 HTTP 请求和工具执行均检查当前凭证；工具列表按权限过滤。没有额外的全权 AI 密钥。当前没有 OAuth 授权发现或动态客户端注册，只支持能显式配置 Authorization 请求头的客户端；不要把该方式宣传成已覆盖所有 MCP 客户端。

```json
{"mcpServers":{"duoyu":{"url":"https://id.duoyu.link/mcp","headers":{"Authorization":"Bearer YOUR_DEVELOPER_KEY"}}}}
```

配置字段因客户端而异；UI 可复制通用模板。模板使用占位符，列表/日志不显示密钥。

工具：list_projects、get_project、create_project、update_project、check_project、upload_project_logo、list_project_roles、save_project_role、update_project_role、delete_project_role、get_project_grants、set_project_grants、get_project_blacklist、set_project_blacklist、rotate_project_secret、get_integration_guide、get_project_service、send_mail、get_mail_status。

通用参数为 project_id；修改角色还需 role_id，读取邮件需 mail_id。body 遵循对应 HTTP API 的 JSON schema；create_project/send_mail 要求 idempotency_key，update_project 要求 if_match（从 get_project 返回 etag 取得）。Logo 上传传 filename 和标准 Base64 file_base64（原图不超过 2 MiB），返回 logo 路径后再创建项目。先 get_integration_guide 获取完整接入说明。

返回值遵循 MCP JSON-RPC；工具 structuredContent 内包含 status、etag、response，response 是业务 API v1 响应。业务错误标记 isError，协议错误保持 JSON-RPC 错误。邮件 queued/accepted 不代表实际收件，unknown 不可盲目重发。

AI 仍须具备目标仓库的读取、编辑、构建和测试权限；MCP 负责账号中心侧操作，不能代替部署网络、回调地址或代码访问权限。

## 存储升级

021_developer_keys 事务迁移把原项目管理凭证并入 api_keys，重新绑定项目授权及创建幂等结果外键，再删除 management_api_keys 表。原服务凭证保留记录、邮件关联和摘要；不导出明文密钥。历史迁移文件保持不变。升级前备份数据库。旧凭证表迁移后的数量与凭证关联由数据库迁移和集成测试验证。

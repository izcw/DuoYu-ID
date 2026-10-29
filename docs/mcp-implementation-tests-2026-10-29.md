# 统一开发者接入与 MCP 实现和测试

## 已实现

- 管理端“开发者接入 → 凭证与 MCP”：一处创建、编辑、轮换、撤销，选择七类权限、项目范围及有效期，复制 MCP 连接模板和 AI 接入说明。
- 原服务与项目管理凭证合并到 api_keys；新增事务迁移 021，保留旧摘要、项目授权、邮件关联、创建幂等结果，移除 management_api_keys。
- 服务 API 与管理 API 共用凭证；多项目服务调用明确指定 X-Duoyu-Project，scope 和项目范围共同检查。
- 官方 MCP Go SDK v1.8.0，无状态 Streamable HTTP；19 个工具覆盖项目、Logo、角色、准入、黑名单、配置检查、接入指南及邮件。
- MCP HTTP 入口与工具执行重新验证凭证。工具发现按权限过滤；读取标注 readOnlyHint，幂等操作按实现标注。无跨账号全权密钥，无网络回环代理或任意 URL 工具，参数只映射到固定业务处理器。
- 创建项目和邮件要求幂等键，修改项目要求 ETag/If-Match，角色删除要求替代角色。审计只记录操作和目标，不记录密钥或消息内容。
- 用户登录仍为 OIDC；OIDC 客户端密钥和开发者凭证分开。

## 验证结果

- Go 全量测试、数据库集成与 go vet：通过。
- 新增旧数据迁移测试：原服务/管理两条记录合并、owner/项目授权/创建结果外键验证通过。
- 官方 SDK HTTP 客户端完成 Connect、ListTools、CallTool。
- MCP JSON-RPC 验证 initialize、tools/list、项目查询与创建、Logo、更新与旧 ETag 拒绝、角色增改删、准入/黑名单、配置检查、项目资料、邮件入队和状态查询。
- 凭证无效、scope 过滤、跨项目读与邮件拒绝、即时权限修改、轮换和撤销均通过；原有管理 API 的到期及幂等测试继续通过。
- 前端类型检查、管理端与登录消息组件生产构建通过。
- 浏览器新增 developer-mcp：创建与编辑凭证、复制带占位符的连接模板、初始化与工具发现、权限撤回、1440/390px 布局通过。
- 原有 13 组浏览器脚本通过：management-api、session、ui-refresh、smoke、account-policy、authenticator、pagination、project-roles、multi-project、qr、admin-pages、roadmap、mail-template。
- 浏览器测试使用隔离 PostgreSQL 容器与 5253 端口；新测试等待账号页面加载完毕再进入动态管理路由；重复回归触发测试管理员登录频控时，仅清理隔离容器中该测试账号及回环地址的限流记录后复验，不关闭生产限流。没有发送真实邮件。

## 范围说明

MCP 当前使用显式 Bearer 头，不含 OAuth 自动发现授权或动态客户端注册，需支持自定义请求头的 AI 客户端。客户端配置格式可能不同。虚拟认证器测试不代替真实 iPhone；未做压力测试、独立安全审计或所有第三方 MCP 客户端验收。

2026-10-29 已部署到生产宝塔服务，同次发布 Go 二进制与 admin/dist，021、022 迁移均已应用。发布备份和线上验证见 [本次发布记录](release-2026-10-29.md)。OIDC 登录流程不受 MCP 路径影响。

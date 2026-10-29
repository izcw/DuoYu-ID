# 2026-10-29 优化与验收记录

## 修改

保持现有黑白灰布局，统一按钮间距、表单错误信息布局、选择卡片键盘焦点、移动端标题/分页/面板及减少动画偏好。业务响应集中到 http_response.go；采用唯一的 code/msg/data/error/request_id/api_version v1 契约，不提供旧字段别名或兼容模式。JSON 拒绝拼接和尾随内容，文本截断不破坏 UTF-8。固定 pnpm 10.18.0，Compose 增加 unless-stopped 重启策略。更新 README、部署、测试及设计文档。

## 验证

Go 全量测试（含数据库集成及新增契约/解析边界测试）、go vet、前端类型检查及两项生产构建全部通过。独立数据库、5253 端口下 13 组浏览器回归全部通过：session、ui-refresh、smoke、account-policy、authenticator、pagination、project-roles、management-api、multi-project、qr、admin-pages、roadmap、mail-template。

覆盖 1440/390px 与 11 个管理/账号页面、分页边界、头像和项目关联、项目角色、管理 API 幂等/并发修改、身份验证器恢复、虚拟通行密钥及扫码。多项目矩阵覆盖服务端网站、公共客户端、纯浏览器 SPA；验证 PKCE、签名、nonce/issuer/audience、SSO、强制认证、并发标签页、退出通知、停权。

隔离测试准备补齐图片和示例客户端，退出通知测试改用 combined。QR 测试补充安全输入框的真实聚焦动作。未发送真实邮件。虚拟认证器不能代替实体 iPhone 验收；未执行压力测试、独立安全审计、OIDC 官方 conformance。

## 发布

已备份生产数据库、data、环境配置、Nginx、旧二进制和前端资源。发布文件校验 SHA-256 后，更新宝塔 Go 项目 duoyu_id 并启动，账号和配置保留。线上健康、登录页、管理端入口 JS/CSS、OIDC Discovery/JWKS、统一成功和 401 错误响应均通过。

生产仍使用 combined 和 127.0.0.1:5252。发布备份在 /www/backup/duoyu-release-20261029-105036。回退时停止 Go 项目，恢复该目录 previous-binary、previous-dist，再启动并检查 healthz；本轮没有新增数据库迁移。

临时文件清理操作被自动审批拒绝，测试目录、容器和暂存包暂时保留，未提交 Git。不删除用户密钥或生产备份。

## 后续

主入口 JS 约 1.19 MB（未压缩），进一步拆包应通过性能测量决定。管理列表存在 200 条读取上限，大规模使用应实施服务端分页。仍需真实跨站项目、实体设备、收件、离机备份及外部告警验收。统一格式减少维护分歧，但不替代接口版本管理和破坏性变更审查。

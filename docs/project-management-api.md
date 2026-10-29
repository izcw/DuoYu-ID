# 项目管理 API 与 AI 一键接入

管理端“开发者接入 → 凭证与 MCP”提供统一开发者凭证。API 支持本地脚本、后端或 AI 工具完成 Logo 上传、项目创建/修改、角色和准入配置，再按标准 OIDC 接入业务项目。

## 一键复制入口

1. **项目管理 API → 创建管理凭证**：填写名称，默认授予读取/创建/修改权限，默认永久有效。默认只管理该凭证创建的项目，也可额外选择已有项目。仅明确需要时勾选“全部项目”或“轮换客户端密钥”。
2. 创建后点击 **复制 AI 完整接入说明和本次凭证**，将完整流程和当前凭证复制给你信任的本地项目 AI；凭证仅创建时显示，页面不会把它保存在 localStorage。普通“复制 AI 接入说明”不含任何密钥。
3. **项目配置 → 复制 AI 接入资料**：复制说明并附带当前项目 ID、类型、回调和登录方式，AI 优先更新该项目，不重复创建；不包含现有 client_secret 或管理凭证。
4. **接入文档 → 复制 AI 接入说明**：复制通用完整说明。issuer 来自服务端配置，不能用当前浏览器地址猜测。

复制支持 HTTPS Clipboard API，也提供局域网 HTTP 下的兼容复制。若浏览器阻止复制，可下载 OpenAPI。项目管理与项目服务共用开发者凭证，按权限与项目范围授权；OIDC client_secret 仍独立；不得嵌入前端、扩展包或提交 Git。

## 完整 AI 说明与接口契约

- [AI 接入说明示例](ai-integration.md)：可阅读的生产域名示例；实际使用优先点击页面复制，取得当前环境正确 issuer。
- [说明模板源文件](../server/templates/ai-integration.md)：服务端加载并替换 {{ORIGIN}} 后提供给复制入口。
- 管理端下载：GET /api/openapi.json（中心管理员会话）。
- 工具下载：GET /api/management/openapi.json，使用 Authorization: Bearer 管理凭证。
- 工具读取说明：GET /api/management/guide，同样需要 projects:read。

全部管理 API 位于 /api/management/，只接受具有相应项目权限的开发者 Bearer 凭证。浏览器 Cookie 不会获得管理 API 权限；携带 Origin 的浏览器请求被拒绝。管理凭证也不能调用 /api/admin/ 管理全站账号或签发更多凭证。

## 权限与范围

| 权限 | 能力 |
| --- | --- |
| projects:read | 查询获准项目、读取角色/准入/黑名单、说明和 OpenAPI |
| projects:create | 上传 Logo、幂等创建项目，自动获得新项目管理范围 |
| projects:update | 上传 Logo、修改获准项目、角色、准入和黑名单 |
| projects:rotate | 显式轮换获准项目 client_secret，旧密钥立即失效 |

每个管理凭证每分钟最多120次请求；到期、撤销或所属管理员不可用时不能继续使用。撤销凭证不会删除它创建的项目。需要调整已有凭证的权限/范围时，创建替代凭证、迁移工具配置，再撤销旧凭证。

## 创建与修改约定

POST /projects 要求 Idempotency-Key（8–128位），相同凭证、相同键和相同参数可在24小时内取得同一创建结果，解决网络响应丢失后重复创建的问题。请求和项目、管理范围在同一数据库事务中落地。client_secret 临时结果使用 AES-GCM 加密保存，过期清除密文并保留幂等记录；超过窗口返回410，参数冲突返回409。显式轮换后不再重放旧密钥。

GET /projects/{id} 返回 data 项目对象和 revision，响应 ETag 例如 "3"。PATCH/PUT 必须携带 If-Match，只合并提供的字段，不清空省略字段。未提供 If-Match 返回428，版本竞争返回412。null、未知字段及客户端类型变更返回400。包括管理页面在内的配置更新都会递增 revision。

列表支持 page/page_size/q，传 page 时 data={items,total,page,page_size}。GET、列表和更新不返回现有客户端密钥；新建项目返回客户端密钥，公共客户端为空。所有响应保持 no-store。

## 部署与验证

历史 020 迁移增加管理凭证，021 将其并入统一 api_keys 存储；当前版本还需执行 021。原 020 增加的能力包括、项目授权范围、加密幂等记录和项目配置版本。备份数据库、数据目录和 DUOYU_SECRET，再构建与重启：

```powershell
./scripts/start.ps1 -BuildWeb
```

生产按部署文档重建 app 并执行迁移。API 使用 HTTP 还是 HTTPS 由当前 DUOYU_ORIGIN 与开发/生产配置决定；生产使用 HTTPS，不能关闭 issuer 校验或把开发模式当生产。

测试：

```powershell
$env:DUOYU_TEST_DATABASE_URL='postgres://duoyu:duoyu_dev_only@localhost:55432/duoyu?sslmode=disable'
go test -count=1 ./...
$env:SKIP_MAIL='true'
Set-Location admin
node tests/management-api.mjs
```

集成测试覆盖凭证、范围、重试、更新竞争与密钥轮换；浏览器创建临时管理凭证，通过 API 上传 Logo、创建/修改项目，再完成真实 OIDC 授权码 + PKCE、client_secret_basic、JWT 验签与 UserInfo，最后撤销凭证并清理临时项目。复制操作使用测试剪贴板，不写入真实系统剪贴板、不打印凭证、不发送真实邮件。


MCP 工具和统一凭证设置见 [开发者接入](developer-access.md)。

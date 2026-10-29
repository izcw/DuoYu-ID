# 多鱼账号 DuoYu ID — 给 AI 的项目创建与认证接入说明

请在当前业务项目中完成“登记/更新 DuoYu ID 项目 + OIDC 登录接入 + 验收”。不要仅输出建议。操作前阅读项目现有代码，按其技术栈实现。

## 连接参数与凭证

- 账号中心规范 issuer：{{ORIGIN}}
- Discovery：{{ORIGIN}}/.well-known/openid-configuration
- 项目管理 API：{{ORIGIN}}/api/management
- OpenAPI：GET {{ORIGIN}}/api/management/openapi.json（同样使用管理凭证）
- 管理凭证从本地环境变量 DUOYU_MANAGEMENT_API_KEY 读取。没有时请用户在账号中心“项目管理 API”创建，不要索要账号密码或 SMTP 授权码。
- 项目管理和业务邮件使用同一类开发者 Bearer 凭证，按 scope 和项目范围授权；OIDC client_secret 用于用户登录客户端认证，与开发者凭证独立。管理凭证绝不能放入浏览器、扩展包、前端环境变量、Git 或日志。
- 所有 API 返回 {code,msg,data}，必须先检查 HTTP 状态。401=凭证无效，403=权限或项目范围不足，400=字段错误，409=幂等参数冲突，410=创建结果密钥重放窗口过期，412=项目版本变化，428=缺少 If-Match，429=限流，503=服务异常。
- 仅开发模式允许 HTTP 本机或本机网卡私有 IP。生产必须 HTTPS；开发 HTTP 网站回调仅允许 localhost / 127.0.0.1。Discovery issuer 必须与配置完全一致，不能关闭 issuer、签名或证书校验。

## 先确定接入方案

1. 从代码判断项目类型和运行地址，列出准确回调 URL、退出回调 URL、前端请求 origin，不能使用通配符。
2. 有可保密的后端：public=false, platform=web，使用授权码 + S256 PKCE，后端 HTTP Basic 认证。
3. 纯前端 SPA：public=true, platform=spa，无客户端密钥，必须 S256 PKCE。
4. 原生桌面/手机：public=true, platform=native，回调按实际应用配置。生产按平台支持的精确 HTTPS 或反向域名 scheme 登记。
5. Chrome/Edge 扩展：public=true, platform=extension，回调必须由 chrome.identity.getRedirectURL('oidc') 得到，通常 https://<扩展ID>.chromiumapp.org/oidc；allowed_origins=["chrome-extension://<扩展ID>"]。不能把 chrome-extension 页面当作回调。
6. 不确定部署 URL 或扩展 ID 时先完成代码与配置占位，再向用户询问缺失值；不要编造或登记无法使用的地址。

## 第一步：检查现有登记，避免重复创建

```sh
curl --fail-with-body "$DUOYU_ISSUER/api/management/projects?page=1&page_size=20&q=项目名称" \
  -H "Authorization: Bearer $DUOYU_MANAGEMENT_API_KEY"
```

设置 DUOYU_ISSUER={{ORIGIN}}。列表 data={items,total,page,page_size}（提供 page 时）；凭证只能看到获准项目。已有本地 DUOYU_CLIENT_ID 时，优先 GET /projects/{id}，确认对应项目，不因列表查不到就反复创建。

## 第二步：上传项目 Logo

创建项目必须上传本地 Logo，不能使用图床 URL、SVG 或默认占位图。PNG/JPEG/WebP，不超过 2MB。优先使用业务项目现有 Logo；没有时请用户提供资源。

```sh
curl --fail-with-body "$DUOYU_ISSUER/api/management/project-logos" \
  -H "Authorization: Bearer $DUOYU_MANAGEMENT_API_KEY" \
  -F "logo=@./public/logo.png"
```

记录 data.logo（/project-logos/...png），填入下面的 logo 字段。

## 第三步：创建项目

保存本次 Idempotency-Key 到本地临时配置，同一次创建重试必须使用相同 key 与相同参数，不能每次失败重新生成 key。

```sh
curl --fail-with-body "$DUOYU_ISSUER/api/management/projects" \
  -H "Authorization: Bearer $DUOYU_MANAGEMENT_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: <本次创建唯一且持久保存的UUID>" \
  --data-binary @duoyu-project.json
```

duoyu-project.json 示例（按真实地址修改）：

```json
{
  "name": "我的项目",
  "description": "项目用途",
  "logo": "/project-logos/<上传返回的文件名>.png",
  "public": false,
  "platform": "web",
  "enabled": true,
  "restricted": false,
  "refresh_enabled": false,
  "redirects": ["https://app.example.com/auth/duoyu/callback"],
  "logout_redirects": ["https://app.example.com/"],
  "allowed_origins": [],
  "login_methods": ["password", "code", "passkey", "qr", "totp"],
  "backchannel_logout_uri": ""
}
```

name 最多60字，description 最多1000字节；redirects 必填最多10个，logout_redirects 最多10个，allowed_origins 最多20个。origin 仅含协议/主机/端口，不含路径、查询或通配符。login_methods 省略默认全开，空数组暂停项目登录。

响应 data={id,client_id,secret,revision,issuer}。服务端项目的 secret 即 client_secret，立即写入业务项目本地私密配置；公共客户端 secret 为空。创建结果在相同管理凭证下可幂等重放24小时，服务端加密保存临时结果；之后返回410并保留创建记录，不会重复建项目。不要在最终报告或日志中打印 secret。

## 第四步：修改已有项目

GET /projects/{id} 返回完整配置与 revision，HTTP ETag 例如 "3"。PATCH /projects/{id} 只提交要修改的字段，并携带该 ETag 为 If-Match；也支持 PUT 的相同合并语义。

```sh
curl --fail-with-body "$DUOYU_ISSUER/api/management/projects/$DUOYU_CLIENT_ID" \
  -H "Authorization: Bearer $DUOYU_MANAGEMENT_API_KEY"

curl --fail-with-body -X PATCH "$DUOYU_ISSUER/api/management/projects/$DUOYU_CLIENT_ID" \
  -H "Authorization: Bearer $DUOYU_MANAGEMENT_API_KEY" \
  -H "If-Match: \"3\"" -H "Content-Type: application/json" \
  --data '{"redirects":["https://app.example.com/auth/duoyu/callback"]}'
```

字段规则与创建相同。不能改 id、created_at、revision、public；客户端类型不可切换。null 和未知字段返回400，空数组表示明确清空。修改成功再 GET 确认最终配置；412 时重新读取并合并用户意图，不能盲目覆盖全部字段。普通 GET、列表、PATCH 不返回现有 client_secret。

仅在明确需要替换客户端密钥时使用 POST /projects/{id}/rotate-secret，要求 projects:rotate 权限。该操作立即使旧密钥失效，非幂等，禁止为“读取密钥”自动调用。密钥轮换后，旧创建结果也不会再返回旧密钥（409）。

## 可选：项目角色、准入与黑名单

以下 {id} 均为当前项目 ID，读取要求 projects:read，写入要求 projects:update，并受凭证项目范围限制。

- GET /projects/{id}/roles：读取角色。默认管理员/admin、用户/user，默认注册角色为用户。
- POST /projects/{id}/roles：{name,role_key,description,is_default}，名称1–40字、字段小写字母开头且只含小写字母/数字/下划线、最多40位，描述1–500字。
- PUT /projects/{id}/roles/{role}：更新同样字段；DELETE 同地址提供 {replacement_id}，关联成员迁移到替代角色。每个项目保留一个默认角色，最多100个角色。
- GET /projects/{id}/grants；PUT 同地址 {emails:["user@example.com"]}，仅当 restricted=true 时限制准入，账号需已注册。
- GET /projects/{id}/blacklist；PUT 同地址 {entries:["user@example.com","1234567890"]}。黑名单优先于准入，可添加未注册邮箱。
- 不随意开启 restricted 或清空 login_methods，避免将用户锁在项目外。

## 第五步：实现业务项目的 OIDC 登录

1. 使用当前技术栈成熟的 OIDC 库，从 Discovery 读取端点，不猜测路径。
2. 发起授权前随机生成 state、nonce、PKCE verifier，保存到与当前浏览器绑定、短期且单次使用的状态；challenge=BASE64URL(SHA256(verifier))。
3. 跳转 authorization_endpoint：client_id、redirect_uri、response_type=code、scope=openid profile email、state、nonce、code_challenge、code_challenge_method=S256。
4. 回调先校验 state 并消费状态，拒绝错误或重复回调；交换 code 时使用相同 redirect_uri、code_verifier。服务端项目使用 client_secret_basic；公共客户端不携带 secret。
5. 校验 ID Token 的签名、alg、kid/JWKS、iss、aud、exp、nonce。不得把 Access Token 当 ID Token。用 iss+sub 绑定本地用户，不用邮箱或昵称当主键。
6. Access Token 访问 userinfo_endpoint，更新昵称、头像、邮箱等资料。profile 范围包含 roles:[字段] 和 project_role:{id,name,field,description}；各项目仅获得自身角色，由业务服务端执行权限判断。既有JWT为快照，敏感权限应在线更新或缩短缓存。
7. 后端站点建立自己的 HttpOnly、Secure、SameSite 会话 Cookie；前端/扩展避免长期存储令牌并处理退出和过期。业务数据保存在业务项目数据库。
8. 如需要续期，先显式 refresh_enabled=true，授权时请求 offline_access；中心确认授权后签发轮换刷新令牌。串行刷新，持久保存新令牌；遇到网络结果不确定时不要重复消费旧刷新令牌，转为重新认证。
9. 实现“退出本项目”，按需要实现“退出账号中心”。若启用 backchannel_logout_uri，用 JWKS 验证 logout_token 的签名、issuer、audience、iat、jti、events 与 sid/sub，防重放并幂等删除本地会话，不能仅凭未验签的请求注销用户。
10. 通行密钥、iPhone 扫码、身份验证器等界面由账号中心处理，接入项目无需各自重写这些表单。

## 第六步：完整验收与交付

- 验证 Discovery issuer 一致，项目配置与真实回调一致。
- 验证未登录跳转、登录回调、本地会话、UserInfo、昵称头像、项目角色、退出。
- 测试 state/nonce错误、重复回调、错误PKCE、账号停用或项目禁用、令牌过期。
- 启用刷新则验证一次性轮换与刷新失败重新登录；启用退出通知则验证验签与防重放。
- 扩展验证真实扩展ID、回调与 origin、后台重启后的会话恢复；SPA 验证 CORS 与无 client_secret。
- 自动化测试不得发送真实验证码或通知邮件。真实设备/收件验收由人工执行并记录。
- 最终报告提供项目ID、issuer、回调、非敏感配置、代码修改和测试结果，明确未完成的真实部署/设备项。不要打印管理凭证、客户端密钥、用户令牌或验证码。


## MCP 自动化

统一开发者凭证可连接 {{ORIGIN}}/mcp（Streamable HTTP，Authorization: Bearer），先调用 get_integration_guide 和 list_projects。创建项目先 upload_project_logo，再 create_project（idempotency_key）；修改先 get_project 获取 etag，再 update_project（if_match）。项目服务共用该凭证但必须另授 app:read/mail:send/mail:read，并指定 X-Duoyu-Project。MCP 方法参数为 project_id 与对应 body。不要发送凭证到前端、第三方网址或 Git。


## 成年声明与法律文件

自建登录表单须读取 GET {{ORIGIN}}/api/legal，将 terms_url/privacy_url 提供给用户，默认不勾选，确认用户年满18周岁并已阅读同意后，才向相应登录/验证码接口发送 X-Duoyu-Legal-Version: version 和 X-Duoyu-Adult: true。不得自动代用户勾选或默认声明成年；这不是实名年龄核验。标准OIDC跳转登录由中心页面完成该步骤，令牌端点协议不变。

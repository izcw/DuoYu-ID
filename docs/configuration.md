# 环境配置参考

## 配置如何加载

| 启动方式 | 配置来源 |
| --- | --- |
| scripts/start.ps1 | 默认读取根目录 .env.local，可用 -EnvFile 指定其他文件 |
| go run ./server 或编译后二进制 | 仅读取进程环境变量，不自动加载 dotenv 文件 |
| docker compose | 根目录 .env 参与 Compose 插值，并由 app 的 env_file 注入 |
| admin 构建 | admin/.env 等 Vite 配置；不得在前端配置放 SMTP、服务密钥或 client secret |

启动脚本按 `KEY=value` 原样读取，不是完整 dotenv 解析器；不要给值额外加引号或行尾注释。配置变更需重启进程；Compose 环境变更需重建容器，`docker compose restart` 不更新容器环境。

模板为 [.env.example](../.env.example) 与 [.env.production.example](../.env.production.example)。模板中的邮箱、密码占位符应替换成部署者自己的配置。不要覆盖已有环境文件重新生成密钥。

## 服务与身份

| 变量 | 默认或要求 | 说明 |
| --- | --- | --- |
| DUOYU_ADDR | 127.0.0.1:5252 | HTTP 监听地址；Compose 覆盖为 :5252 |
| DUOYU_ORIGIN | http://localhost:5252 | 对外 origin，也是 OIDC issuer；无路径、查询和片段 |
| DUOYU_DEV | 仅 true 开启 | 只允许 localhost 或 127.0.0.1；生产 false 且 origin 必须 HTTPS |
| DATABASE_URL | 必填 | PostgreSQL DSN；Compose 注入数据库服务地址 |
| DUOYU_SECRET | 必填，64 位随机十六进制字符 | 32 字节持久服务密钥，用于摘要、队列密钥派生与协议配置 |
| DUOYU_DATA_DIR | data | 原生相对当前工作目录；Compose 覆盖为 /data |
| DUOYU_ADMIN_EMAIL | 初始化时必填 | 首次系统管理员邮箱 |
| DUOYU_ADMIN_PASSWORD | 非空时尝试初始化 | 必须符合密码规则；只插入不存在的邮箱，不重置旧密码、不提升旧账号 |
| DUOYU_ROLE | combined | api / worker / combined，见下节 |
| DUOYU_MIGRATE_ONLY | true 时只迁移 | 不启动 HTTP 和任务，不初始化签名文件或管理员；仍需有效数据库、origin 和服务密钥 |
| DUOYU_TRUSTED_PROXIES | 空 | 逗号分隔可信代理 CIDR；空表示忽略转发头 |
| DUOYU_DATABASE_ALERT_BYTES | 10737418240 | 数据库容量告警，默认 10 GiB，必须为正整数 |
| POSTGRES_PASSWORD | Compose 必填 | PostgreSQL 容器密码，同时组成 app 的 DSN；建议随机 hex 避免 URL 转义问题 |

`DUOYU_SECRET` 不是 SMTP 授权码、OIDC client secret、API 凭证或 RSA 签名私钥。它与 data 中的签名文件都需要持久化和独立保管；任意更换会破坏已有摘要/加密队列等状态。WebAuthn RP ID 根据 origin 的主机名生成，换域名会影响已有通行密钥。

## 运行角色

| 角色 | HTTP | 邮件与退出通知 | 监控与清理 |
| --- | --- | --- | --- |
| combined | 是 | 是 | 是 |
| api | 是 | 否 | 否 |
| worker | 否 | 是 | 是 |

默认 Compose 仅启动一个 combined app。若拆分部署，需自行增加 worker 服务，统一数据库、DUOYU_SECRET、签名数据与 SMTP 配置；首次签名初始化由单实例完成。只部署 api 会造成邮件和退出通知积压。

## 真实 QQ SMTP

| 变量 | 配置 |
| --- | --- |
| SMTP_HOST | smtp.qq.com |
| SMTP_PORT | 465 |
| SMTP_USER | 发件 QQ 邮箱 |
| SMTP_FROM | 发件邮箱，省略时回退 SMTP_USER |
| SMTP_PASSWORD | QQ 邮箱生成的 SMTP 授权码，不是账号登录密码 |
| SMTP_PLAIN_LOCAL | false |

真实 SMTP 路径使用隐式 TLS，不是任意端口自动 STARTTLS 升级。不要把 587 直接代替 465 而不检查服务实现。

开发配置可用 `./scripts/configure-mail.ps1` 隐藏输入授权码。旧 `init-dev.ps1` 仍生成本地邮箱配置，新安装必须完成真实邮箱配置步骤。Mailpit 文件与测试分支只是保留的开发工具，不是当前真实邮箱运行方案。

## 项目邮件配额

| 变量 | 默认值 |
| --- | --- |
| MAIL_APP_PER_MINUTE | 每项目 10 次/分钟 |
| MAIL_APP_PER_DAY | 每项目 100 次/24 小时 |
| MAIL_GLOBAL_PER_DAY | 全局项目邮件 500 次/24 小时 |

同一收件人 3 次/分钟和服务 API 来源限流另行生效；认证邮件使用独立限制。以上是产品保护值，不代表 SMTP 服务商允许额度。请求和状态说明见 [项目邮件](project-mail.md)。

## 配置检查

生产可运行 `docker compose config --quiet` 检查格式与必填插值，避免把展开后的配置和密钥打印到日志。启动后从管理端检查 issuer、开发模式、SMTP 是否配置与监控最后更新时间。诊断可读取不等于真实送达、TLS 或多设备验收已完成。

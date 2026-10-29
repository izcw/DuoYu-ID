# 开发与启动

命令默认在仓库根目录、PowerShell 中执行。开发入口为 `http://localhost:5252`，由 Go 同时提供管理端构建产物与登录页面。

## 环境要求

| 工具 | 仓库使用版本 | 依据 |
| --- | --- | --- |
| Go | 1.27.3 | go.mod 与 Dockerfile；目标环境无法获取时先解决工具链可用性，不只改版本号绕过依赖要求 |
| Node.js | 24 | Dockerfile 与 CI；package.json 声明下限为 20.19.0 |
| pnpm | 10 系列 | Dockerfile 固定 10.18.0，CI 使用 10；使用锁文件安装 |
| PostgreSQL | 17 | Compose 文件；开发时以 Docker 容器运行 |
| PowerShell | 本地脚本执行环境 | 初始化、启动与邮箱配置脚本 |
| Chrome | 浏览器回归需要 | Playwright 默认使用本机 Chrome，可设 CHROME_PATH |

检查：`go version`、`node --version`、`pnpm --version`、`docker version`。版本以仓库文件为准，不承诺所有后续版本兼容。

## 启动已有环境

进入实际保存仓库的位置，当前本机目录是：

```powershell
Set-Location 'D:\Users\Administrator\桌面\DuoYu\DuoYu-ID'
docker start duoyu-postgres-dev
./scripts/start.ps1
```

如果 Go 未加入 PATH，可使用 `./scripts/start.ps1 -Go 'C:\你的Go安装目录\bin\go.exe'`。本机先前临时工具链位于 `$env:TEMP\duoyu-go-runtime\go\bin\go.exe`，目录存在时可用，不是新机器的固定路径。

保留已有 `.env.local` 和 `data/`；不要重复初始化、更换数据卷或随机生成新服务密钥来修复启动。真实 QQ SMTP 配置继续生效，不需要启动 Mailpit。

## 首次安装

### 获取代码

```powershell
git clone https://github.com/izcw/DuoYu-ID.git
Set-Location DuoYu-ID
pnpm --dir admin install --frozen-lockfile --ignore-scripts
go mod download
```

管理端已经是定制后的干净版本，日常开发不要再次执行上游 `clean:dev`。

### 创建数据库

以下命令只适用于没有同名容器的新环境；已有容器使用上一节的启动方式。

```powershell
docker run -d --name duoyu-postgres-dev -e POSTGRES_DB=duoyu -e POSTGRES_USER=duoyu -e POSTGRES_PASSWORD=duoyu_dev_only -p 127.0.0.1:55432:5432 -v duoyu-id-dev-postgres:/var/lib/postgresql/data postgres:17-alpine
docker exec duoyu-postgres-dev pg_isready -U duoyu -d duoyu
```

等就绪检查成功再启动服务。开发密码仅用于本机隔离数据库，不能用于生产。

也可用 `docker compose -p duoyu-id-dev -f compose.dev.yaml up -d postgres` 只启动数据库，但容器名称和数据卷不同，不自动读取原容器数据。某些浏览器夹具固定使用 `duoyu-postgres-dev`，详见测试指南。两种方式不能同时占用 55432。

### 初始化配置与真实邮箱

```powershell
./scripts/init-dev.ps1
# 在本机编辑 .env.local，确认初始化管理员邮箱和数据库地址
./scripts/configure-mail.ps1
```

初始化脚本仅在文件不存在时写入随机管理员密码与 `DUOYU_SECRET`。**该旧脚本仍生成 Mailpit 配置**，因此采用真实 QQ 邮箱方案时，首次启动前必须执行邮箱配置脚本。它隐藏输入授权码，切换至 `smtp.qq.com:465` 并设置 `SMTP_PLAIN_LOCAL=false`，不会发送测试邮件。模板中的管理员与发件邮箱应换成你的账号。

已有真实 SMTP 配置无需重复设置。不要把环境文件、管理员密码或授权码放进聊天、截图和 Git。

### 构建与启动

```powershell
./scripts/start.ps1 -BuildWeb
```

脚本加载 `.env.local`，构建前端，再运行 Go。普通启动应用待执行迁移，初始化缺失的签名文件和首次管理员。bootstrap 只创建不存在的邮箱，不覆盖旧密码或提升已有普通账号。

打开 [登录页](http://localhost:5252/login)，选择密码登录。管理员可使用管理后台，普通用户使用个人账号中心。

## 日常修改

| 修改内容 | 生效方式 |
| --- | --- |
| Vue、TypeScript、管理端 CSS、图标 | `pnpm --dir admin build`；刷新页面 |
| admin/public 下的 CSS、Logo 等静态资源 | 重新构建，复制到 dist |
| Go、登录与邮件 HTML 模板、SQL 迁移 | 重启 Go；模板由 go:embed 嵌入 |
| .env.local | 重启进程，不是热加载 |
| 前后端一起修改 | 停止旧进程后执行 `./scripts/start.ps1 -BuildWeb` |

前台运行的服务可用 Ctrl+C 正常停止，不必删除数据库或卷。

`pnpm --dir admin dev` 可用于界面开发，但不能直接把 Vite 地址当成完整认证入口。Cookie、Origin 检查、回调和 WebAuthn 均依赖 `DUOYU_ORIGIN`；完整登录联调使用 Go 的 5252 入口。

## 修改后的命令速查

以下 PowerShell 命令均在仓库根目录执行。启动脚本读取 `.env.local`；直接运行 Go 不会自动读取该文件。

### 修改前端后打包

```powershell
Set-Location 'D:\Users\Administrator\桌面\DuoYu\DuoYu-ID'
# 首次安装或依赖、锁文件变化后执行
pnpm --dir admin install --frozen-lockfile --ignore-scripts
# 每次修改前端后执行
pnpm --dir admin build
```

产物在 `admin/dist/`。此命令包含本地图标、TypeScript 检查、管理端和登录 Message 组件构建，不要只执行 `vite build`。Go 已运行时，构建成功后刷新页面即可；未运行时执行：

```powershell
docker start duoyu-postgres-dev
./scripts/start.ps1
```

访问 `http://localhost:5252/login`。`admin/public/` 中的登录 CSS、Logo 和图片也要重新构建，不能只改源文件后刷新。

### 前端开发与预览

```powershell
# 热更新开发服务器，访问终端打印的地址
pnpm --dir admin dev
# 构建后的静态预览
pnpm --dir admin serve --host 127.0.0.1
```

开发后端另开终端运行 `./scripts/start.ps1`。Vite 只有 `/api` 代理，不能替代 Go 的登录模板、Cookie 与 WebAuthn 同源环境。完整认证联调仍使用构建后的 5252 入口；dev 和 serve 都不是生产部署命令。

### 修改 Go 后启动

在旧服务终端按 `Ctrl+C`，再执行：

```powershell
./scripts/start.ps1
# Go 未加入 PATH 时，本机可用此临时工具链（目录须存在）
# ./scripts/start.ps1 -Go "$env:TEMP\duoyu-go-runtime\go\bin\go.exe"
```

脚本加载配置后实际执行 `go run ./server`。如果终端已注入全部必要环境变量，也可直接运行：

```powershell
go run ./server
```

Go 文件、`server/templates/` 和嵌入的迁移文件不支持热更新，修改后必须重启。不要同时启动两个进程占用 5252。

### 前后端同时修改

先在旧服务终端按 `Ctrl+C`，然后执行：

```powershell
./scripts/start.ps1 -BuildWeb
# 或显式指定 Go
# ./scripts/start.ps1 -BuildWeb -Go "$env:TEMP\duoyu-go-runtime\go\bin\go.exe"
```

只改登录 CSS 要构建；只改 `server/templates/login.html` 要重启 Go；两者都改使用上述命令。

### 编译并运行 Go 二进制

```powershell
New-Item -ItemType Directory -Force bin | Out-Null
go build -trimpath -o ./bin/duoyu.exe ./server
```

二进制不读取 `.env.local`。先按启动脚本相同规则注入进程环境，再在仓库根目录运行：

```powershell
foreach ($line in Get-Content -LiteralPath '.env.local') {
    if ($line -match '^([A-Z][A-Z0-9_]*)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
$env:DUOYU_MIGRATE_ONLY = 'false'
./bin/duoyu.exe
```

保持工作目录包含 `admin/dist/`，数据目录可写。修改 Go 后重新编译并重启二进制；生产部署见 [部署指南](deployment.md)。

## 常见问题

| 现象 | 先检查 |
| --- | --- |
| 5252 被占用 | `Get-NetTCPConnection -LocalPort 5252 -State Listen`，确认是否已有项目进程 |
| 数据库连接失败 | 容器状态、55432 映射、DATABASE_URL 与数据卷是否一致 |
| 邮箱收不到验证码 | SMTP 配置、worker、队列状态；SMTP 接受不等于进入收件箱 |
| 页面仍是旧样式 | 是否 build；是否打开同一服务；嵌入模板需重启 Go |
| migration checksum mismatch | 恢复已应用迁移的原内容，再添加新迁移，不修改库中校验和 |
| 手机打不开 localhost | 手机 localhost 指向手机自身；应通过正式 HTTPS 域名验收 |
| 改 bootstrap 密码无效 | bootstrap 不改现有密码，应使用账号安全页或找回密码 |

继续阅读 [配置参考](configuration.md)、[测试指南](testing.md) 或 [生产部署](deployment.md)。

登录/扫码页的 CSS 与登录 Message 资源 URL 带内容版本号，前端重新构建后版本自动变化；固定路径认证资源使用 no-cache 重新验证，避免新 HTML 配上旧样式。已打开的标签页仍需刷新。

## 局域网访问

默认监听 `0.0.0.0:5252`，启动日志显示 Local 和 Network 地址。例如：

```text
Local    http://localhost:5252/login
Network  http://192.168.1.16:5252/login
```

Network 地址从本机网卡读取，IP 变化后以新日志为准；虚拟网卡可能同时显示其他地址。同一局域网的设备使用 WLAN/以太网地址，系统防火墙需允许 TCP 5252 入站。

开发环境只允许访问本机网卡 IP、同一端口的同源请求，不开放任意跨域来源。`DUOYU_ORIGIN` 仍是认证中心的规范地址：本机默认 `http://localhost:5252`，局域网 IP 可打开页面和使用普通账号登录；跨设备 OIDC 回调需配置所有设备都可访问的统一 issuer。生产请使用 `https://id.duoyu.link`。

局域网 HTTP 不具备通行密钥所需的安全上下文；手机和 Mac 的通行密钥、直接验证及扫码完整流程应通过 HTTPS 域名验收。

如 Windows 防火墙阻止其他设备访问，在管理员 PowerShell 中执行（只允许同一子网）：

```powershell
New-NetFirewallRule -Name 'DuoYu-ID-LAN-5252' -DisplayName 'DuoYu ID LAN TCP 5252' -Direction Inbound -Action Allow -Protocol TCP -LocalPort 5252 -RemoteAddress LocalSubnet -Profile Any
```

此命令需管理员权限；普通终端可能返回“拒绝访问”。服务器监听成功或本机能通过局域网 IP 登录，并不能证明防火墙已允许其他设备入站。

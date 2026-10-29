# 项目角色

项目管理 → 角色配置，或项目编辑页下方 → 角色配置。每个项目默认有“管理员 / admin”和“用户 / user”，默认注册角色为用户。已有项目由 018 迁移初始化；历史项目成员分配用户角色，中心超级管理员也不会自动获得项目管理员角色。

## 配置与分配

- 每个角色填写名称、字段标识、描述，可设为默认注册角色；初始角色可以改名、修改字段和删除。
- 名称 1–40 字；字段 1–40 位，以小写字母开头，可包含小写字母、数字、下划线；同一项目字段唯一。描述须填写 1–500 字，每个项目最多 100 个角色。
- 角色 ID 稳定，字段可以修改。接入项目如果使用旧字段判断权限，需要同步调整判断规则。
- 默认注册指“账号首次成功加入当前项目”，并非中心账号第一次注册。调整默认角色不会覆盖已有成员的角色。
- 项目必须保留一个默认角色。要取消原默认角色，将其他角色设为默认即可。
- 删除角色必须选择该项目的另一个替代角色。关联用户在同一事务内迁移；删除默认角色时替代角色成为默认。最后一个角色不能删除。
- 登录用户列表显示当前角色，点击“修改角色”进入独立页面分配角色。目前每位用户在每个项目有一个角色。
- 项目管理员角色仅是发给接入项目的身份信息，不授予多鱼账号后台权限，也不绕过项目准入或黑名单。

## 接入项目读取

申请 `openid profile` 后，ID Token 与 UserInfo 包含：

```json
{
  "roles": ["user"],
  "project_role": {
    "id": "user",
    "name": "用户",
    "field": "user",
    "description": "项目普通用户"
  }
}
```

同一账号在不同 client_id 下返回各自角色，不包含其他项目的角色。只申请 openid 或 email 时不返回角色声明。项目仍要完成标准签名、issuer、audience、有效期校验，然后由自己的服务端实施业务权限。

修改用户角色或角色定义后，UserInfo 返回最新值，新签发/刷新的 ID Token 使用新值。已签发 JWT 和项目本地会话是快照，无法原地更新；涉及敏感权限时应在线读取 UserInfo 或设置权限缓存失效机制，不能把旧 JWT 当成永久权限。更改默认角色不影响已有成员。

## 管理 API

仅中心超级管理员可调用，写操作要求同源 Origin 与 X-Duoyu-Request: 1。

| 接口 | 作用 |
| --- | --- |
| GET /api/admin/apps/{id}/roles | 全部角色与成员数量 |
| POST /api/admin/apps/{id}/roles | 创建，字段 name、role_key、description、is_default |
| PUT /api/admin/apps/{id}/roles/{role} | 更新相同字段 |
| DELETE /api/admin/apps/{id}/roles/{role} | 删除，必须提供 replacement_id |
| PUT /api/admin/apps/{id}/users/{user}/role | 分配角色，提供 role_id |
| GET /api/admin/apps/{id}/users | 成员增加 role_id、role_name、role_key |

## 更新与测试

先备份，再构建前端并重启 Go，自动应用 018 迁移：

```powershell
./scripts/start.ps1 -BuildWeb
```

已有 Docker 部署按部署文档重建 app 并迁移。Go 集成测试覆盖默认分配、字段唯一、默认角色切换、删除迁移、项目隔离和声明 scope；浏览器测试：

```powershell
$env:SKIP_MAIL='true'
Set-Location admin
node tests/project-roles.mjs
```

浏览器测试创建临时项目、使用本地管理员登录，结束清理数据，不发送真实邮件。

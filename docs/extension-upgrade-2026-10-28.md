# 扩展接入建议落实与验收

日期：2026-10-28。修改 DuoYu-ID 与同级 DuoYu-Extensions；沿用现有 OIDC 实现。

## 已落实

| 建议 | 结果 |
| --- | --- |
| Discovery 与能力一致 | 仅 code、authorization_code/refresh_token、四个实际范围与 S256；移除设备码端点和未支持流程声明，拒绝其他 token grant |
| 浏览器扩展模板 | 公共客户端下新增扩展平台，输入 ID 生成 Chromium 精确回调与来源，不生成 secret；提供 Firefox 配置提示 |
| 回调和来源分离 | 新增 allowed_origins，精确 CORS 检查，旧网站配置迁移一次性回填；修改回调不隐式放宽来源 |
| 标准令牌错误 | UserInfo 401 invalid_token / 403 insufficient_scope；来源拒绝独立；请求级数据库故障统一 503，避免误清会话 |
| 刷新策略 | 保留轮换和重放撤销；扩展持久记录进行中的刷新，Worker 中断时不重放旧令牌；文档明确 12 小时绝对寿命 |
| 退出一致性 | 文档区分后端、SPA、扩展，明确后通道和在线复核边界 |
| 参考客户端 | 将现有 DuoYu-Extensions 作为可运行参考，增加测试命令和 Worker/重载/停权回归，不复制一套易漂移的认证代码 |
| 文档整理 | 新增 extension-integration.md，链接当前配置和架构；文档索引继续区分历史验收与当前指南，真实 QQ SMTP 不变 |

## 验收

- Go 全套测试、go vet、管理端 Vue 类型检查及构建通过。
- 新协议契约验证 Discovery、Chrome/Firefox 来源、拒绝回调来源代替 allowed_origins、非法 Origin、标准令牌错误和不支持 grant。
- 隔离测试数据库注入缺表查询故障，UserInfo、刷新和授权码兑换均返回 503；恢复后原有效令牌可查询。没有暂停真实数据库。
- 管理端浏览器验证扩展 ID 生成配置、保存 platform/allowed_origins、项目 Logo、黑名单、凭证及移动布局。
- 16 项真实多项目浏览器流程通过：后端网站、公共客户端、纯 SPA、SSO、PKCE、验签、通行密钥、二维码及退出同步。
- 扩展 pnpm typecheck、pnpm zip、协议负向测试通过。
- Edge 真实扩展测试通过：identity 授权窗口、密码、PKCE、验签、UserInfo、多页状态、串行刷新、Worker 实际终止后恢复、退出撤销、账号停权、中心会话撤销、取消授权、工作区入口、pending 刷新不重放、扩展 runtime.reload 后会话清空。
- 本机服务已重启、迁移 015 已应用，本地扩展项目已登记 extension 平台及实际 chrome-extension 来源。扩展构建目录 .output/chrome-mv3 已更新，现有浏览器需手动重新加载扩展。

## 明确边界

没有发送真实邮件或读取中心秘密；只用随机临时账号，结束清理。Firefox 仅配置/协议测试，没有真实浏览器验收；真实授权窗口在 Edge 测试，不能据此宣称 Chrome/Firefox 全部验收。未执行公网 HTTPS、商店安装身份变化、真实手机/Passkey、长期自然过期、压力或完整 OIDC conformance 验收。刷新 pending 的恢复以持久标记模拟中断窗口，Worker 休眠另有真实终止测试。

下一步在正式 HTTPS 和商店 ID 下登记生产客户端，完成浏览器矩阵验收；长期设备登录属于独立升级，仍需设计最大寿命、闲置超时和设备撤销，当前没有改成永久登录。

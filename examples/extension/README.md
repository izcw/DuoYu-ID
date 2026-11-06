# 浏览器扩展参考客户端

参考实现为同级 `DuoYu-Extensions` 项目，避免在认证仓库复制第二套实现。

- `src/features/auth/config.ts`：环境、客户端 ID、scope。
- `src/features/auth/protocol.ts`：Discovery、PKCE、回调与 JWT 验证。
- `src/entrypoints/background.ts`：唯一刷新队列、session 存储、pending 恢复与退出。
- `scripts/auth-protocol.mjs`：协议负向测试。
- `scripts/auth-isolated.mjs`：使用临时账号和独立浏览器配置，验证 MV3 授权窗口和账号中心联调。

在扩展目录运行 `pnpm typecheck`、`pnpm zip`、`pnpm test:auth`、`pnpm test:auth:integration`。集成测试需要本机专用 `duoyu_sync_test` PostgreSQL 库及 `GO_BINARY`；运行时创建随机 schema 和临时项目，结束后清理。详见[测试与验收](../../docs/testing.md)。

详细步骤见 [扩展接入指南](../../docs/extension-integration.md)，测试边界见 [验收报告](../../docs/extension-upgrade-2026-10-28.md)。

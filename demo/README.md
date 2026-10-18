# OAuth2 Demo 服务

零依赖 Node 脚本，用于端到端测试 SForum OAuth2 Provider 的
**授权码 + PKCE → 令牌 → userinfo → 刷新 / 撤销 / 内省** 全流程。

## 用法

1. 创建测试应用：后台 → OAuth2 → OAuth2 应用 → 新建，
   回调地址填 `http://127.0.0.1:9000/callback`（改了 PORT 就同步改）。
2. 复制 `.env.example` 为 `.env`，填入 `CLIENT_ID`（和 `CLIENT_SECRET`，公开客户端留空）。
   可选 `SCOPES`（空格分隔，如 `profile avatar`）：不填则授权请求不带 `scope`，
   由 provider 回落到应用在后台登记的范围——想验证多权限确认页就把应用勾上再留空。
3. 启动：

   ```bash
   node demo/server.js
   ```

4. 打开 <http://127.0.0.1:9000/>，点「使用 SForum 登录」，登录并确认授权后
   首页会展示 userinfo 与令牌信息，并可继续测试刷新 / 撤销 / 内省。

## 说明

- 机密客户端：token 请求带 `client_secret` + PKCE；公开客户端：只带 `code_verifier`。
- 所有写操作（token / revoke / introspect）先 `GET /api/oauth/csrf` 握手，
  再带 `X-Csrf-Token` + `Origin`（宿主 CSRF 中间件要求，见 README「Host 约束」）。
- `state`、授权码、`code_verifier` 均一次性使用；结果与令牌只存在内存里，重启即清。

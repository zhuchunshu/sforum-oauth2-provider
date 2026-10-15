# Agent Guide · sforum-oauth2-provider

本文件给**新会话**的接手说明。读完它 + `README.md` 的「Host 约束」章节，即可开工，不必重新摸索框架。

## 这是什么

SForum 的 OAuth2 授权服务器插件（SForum 当 provider，类似 GitHub OAuth Apps）：
第三方应用用授权码 + PKCE 取令牌，再调 userinfo 读取用户资料。账号与会话始终由 SForum 持有。

- 仓库：`~/Code/github/sforum-oauth2-provider`（独立仓库，GitHub `zhuchunshu/sforum-oauth2-provider`，MIT）
- 框架检出：`~/Code/github/SForum`（**同级目录**，插件通过本地 replace 解析宿主 SDK）
- 扩展 id：`sforum.oauth2-provider`；版本在 `plugins/sforum-oauth2-provider/sforum.extension.json`

## 目录布局（重要）

仓库根就是 SForum 的**外部扩展源根**，扩展包在 `plugins/` 下。宿主扫描规则是
`<EXTERNAL_EXTENSION_ROOTS>/plugins/<package-dir>`，且**同一扩展 id 只能出现在一个源**。

```text
sforum-oauth2-provider/
├── plugins/sforum-oauth2-provider/   ← 扩展包：manifest、schemas/、openapi/、frontend/、backend/
├── scripts/prepare-package.sh
├── README.md   CHANGELOG.md   NOTES.md（宿主的 8 条实测约束 + 历史决策）
└── .github/workflows/{ci,release}.yml
```

宿主 `DigestTree` **禁止符号链接**（根目录或树内任何 symlink 都返回 `ErrSymlink`），
所以不要用软链把仓库挂到别处，也不要保留第二份副本。

## 开发闭环

```bash
# 1. 改 plugins/sforum-oauth2-provider/ 下的后端或前端制品
cd plugins/sforum-oauth2-provider/backend && go vet ./... && go test ./...

# 2. 构建二进制 + 刷新摘要 + 宿主校验（SFORUM_DIR=… 可覆盖框架路径）
./scripts/prepare-package.sh

# 3. 让站点加载：重启 API 扫描到候选 → 后台「插件管理 → 禁用 → 升级 → 启用」
```

**改任何包内文件（含 `.mjs`/`.css`/`schemas/*.json`）都必须重跑第 2 步**：宿主以 exact-artifact
校验哈希，摘要不匹配的包会被拒绝启用。同版本号只改内容时，卡片上的 `重启` 不会切换制品，
必须走「禁用 → 升级（授权当前精确制品 → 确认并升级）→ 启用」。

## 当前环境事实（2026-10-15）

- `.env`：`EXTERNAL_EXTENSION_ROOTS=…/SForum/extensions/dev,~/Code/github/sforum-oauth2-provider`
- dev API：**后台运行**，日志 `/tmp/sforum-api-dev.log`；`air` 在**自身启动时**缓存 env，
  改 `.env` 后必须重启 `air` 才生效。要收回终端：`kill $(pgrep -x air) && ./scripts/api-dev.sh`
- 站点：`http://127.0.0.1:3000`，管理端 `/control-panel/extensions/sforum.oauth2-provider/pages/clients`
- 已有一个测试应用 `Smoke Test App`（`sfo_client_fceaa85a41f3b9eafeb9ff9d`，回调 `http://127.0.0.1:3000/`）
- SForum 工作区里可能有**其他人在改**的文件；**不要回退不属于本任务的改动**

## 验证标准（不要只跑单测就报完成）

1. `cd plugins/sforum-oauth2-provider/backend && go test ./...`
2. `cd ~/Code/github/SForum/apps/api && go run ./cmd/sforum extension validate|test <包路径>`
3. 真实浏览器冒烟：`/api/oauth/authorize` →（确认页）→ 回跳 `?code=` → `POST /api/oauth/token` →
   `GET /api/oauth/userinfo?access_token=…`；再验一次撤销后 401。
4. UI 改动要出截图：桌面 + 窄屏（≤900px 转堆叠），并确认无横向滚动条。

## 宿主约束速查（详见 README）

1. 插件路由默认 `filtered` authority：`Cookie`/`Authorization` 被剥离 → 访问令牌只能用
   `access_token` 查询参数交付。
2. 所有插件路由都过宿主 CSRF 中间件 → 服务端到服务端调用要先 `GET /api/oauth/csrf` 拿
   `csrf_` cookie，再带 `X-Csrf-Token` + `Origin`。
3. wire 层 schema 引用必须是版本化契约 `id@N`；包内路径会被拒 → 必须维护
   `openapi/routes.yaml`，每条 operation 要有带 schema 的 **2xx** 响应 + `default` 回退。
4. HTTP 响应必须自带 `Content-Type: application/json`；`action: add` 的路由无论 mode 都要
   `responseSchema`。
5. 插件 → 宿主的调用必须 **actorless**（清空 `RequestContext.Actor`），否则
   `protocol v2 actor context is not host-attested`。
6. 授权确认页用 `mode: stream`（`mode: http` 的响应被强制 JSON 校验，公共资产端点不服务 `.html`）。
7. PKCE S256 用 **base64url**（RFC 7636，无填充）。
8. userinfo 只能拿核心安全字段（`id`/`username`/`displayName`/`status`）。

## 发布

推 `v*` 标签触发 `.github/workflows/release.yml`：verify（测试 + 宿主校验 + 标签与清单版本一致性）
→ 四平台矩阵交叉编译（linux/darwin × amd64/arm64）→ 每平台各自写摘要后打包 zip + SBOM
→ Release 挂 `SHA256SUMS`。**发版前把 `sforum.extension.json` 的 version 提到与标签一致**。
CI/Release 都靠 `SFORUM_REF`（默认 `main`）检出框架，需要锁定版本时改成 tag/commit。
Windows 未发布：宿主按 `backend/entry` 直接执行、不做 `.exe` 归一化，未经实机验证。

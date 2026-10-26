# Agent Guide · sforum-oauth2-provider

本文件给**新会话**的接手说明。读完它 + `README.md` 的「Host 约束」章节，即可开工，不必重新摸索框架。

## 这是什么

SForum 的 OAuth2 授权服务器插件（SForum 当 provider，类似 GitHub OAuth Apps）：
第三方应用用授权码 + PKCE 取令牌，再调 userinfo 读取用户资料。账号与会话始终由 SForum 持有。

- 仓库：`~/Code/github/sforum-oauth2-provider`（独立仓库，GitHub `keeptrue/sforum-oauth2-provider`，MIT）
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

## 品牌资产（必须）

包内 `brand/` 是产品身份的一部分，不是可选项：

| 文件 | 尺寸 | 上限 |
| --- | --- | --- |
| `brand/icon.svg` | `viewBox="0 0 512 512"`，自包含 | 40 KiB |
| `brand/icon.png` | 512×512 PNG | 120 KiB |
| `brand/social-preview.png` | 1280×640 PNG（中文主卡） | 600 KiB |

- 生成：`node ~/Code/github/SForum/scripts/brand/generate.mjs --only <包目录名>`；
  先加 `--draft /tmp/x --sheet /tmp/x` 看样张。文案与字形改
  `~/Code/github/SForum/scripts/brand/spec.json`，不手改 PNG。
- 校验：`scripts/prepare-package.sh` 已含 `extension brand check`；单独跑
  `cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension brand check "$PKG"`。
- 规则（模板参数、错误码、GitHub 手动上传步骤）：
  `~/Code/github/SForum/docs/extensions/brand-assets.md`。
- 品牌文件不进 `packageFiles`，但属于包树：改图会改变包摘要，走正常升级流程；
  不要为了消掉摘要漂移去改版本号。

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

**改了 `components[].propsSchema` / `l2Component` 指向的制品，必须同时提升该组件的
`contractVersion`**（`sforum.oauth2-provider.component.apps@20` 这种 `@N` 后缀 +1）。
宿主的 `validateComponentUpgrade` 把「同契约版本 + 被引用内容变化」判成硬冲突
（`contribution … changed without a contract version`），整条升级 lifecycle 会在
`07.host.registering` 失败。`scripts/prepare-package.sh` 现在会在构建前比对
`.build/component-contract-state.json`，检测到这种漂移会直接报错退出，不要绕过它。

**改了 `capabilities`（如新增 `users.email.read`）还要重启 API**：宿主在插件运行时启动时
快照能力集，dev watcher 的自动提升与插件「重启」都不刷新它；不重启时宿主不签发同意式委派，
`email` 会静默缺席（授权仍成功）。`kill $(pgrep -x air) && ./scripts/api-dev.sh`，再确认
后台卡片是目标版本、`audit_events` 出现 `identity.email.consent.read`。

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
8. userinfo 的字段面由宿主三个面拼成：core-safe 投影（`id`/`username`/`displayName`/`status`/`avatar`/`groups`
   + 显式声明的 `topicCount`/`commentCount`）、稳定查询 `sforum.core.safe_user.by_id`
   （`created_at`/`updated_at`，清单声明 `core_views` 授权；失败一律降级成字段缺席，见
   `backend/account.go`），以及**同意式邮箱快照**（capability `users.email.read` +
   `core.query.identity.self_email@1`，授权码签发时读一次并落 `oauth_email_claims`，
   userinfo 只读快照；见 `backend/email_claim.go`）。插件 → 宿主的调用仍然必须 actorless，
   所以 userinfo 绝不可能现读 email。
   宿主对 initials 账号给出的 `avatar` 是空串、上传头像给的是相对路径，插件在授予 `avatar` 范围时
   生成确定性 SVG data URL 兜底（GitHub 风格 identicon：浅灰方底 + 5×5 镜像像素块，无文字），
   并用 `backend/baseurl.go` 把相对路径补成绝对链接
   （运行时 `site.url` → `APP_URL`）。`profileUrl` 走同一条站点地址解析链。**不要改回「非空才下发」
   或裸相对路径**。

9. 组件贡献的 `contractVersion` 必须与被引用制品同源变化：props/result schema、L2 前端制品、
   SSR 模板任一处内容变了就要 +1，否则升级被 `validateComponentUpgrade` 拒绝（见「开发闭环」）。

## 发布

推 `v*` 标签触发 `.github/workflows/release.yml`：verify（测试 + 宿主校验 + 标签与清单版本一致性）
→ 四平台矩阵交叉编译（linux/darwin × amd64/arm64）→ 每平台各自写摘要后打包 zip + SBOM
→ Release 挂 `SHA256SUMS`。**发版前把 `sforum.extension.json` 的 version 提到与标签一致**。
CI/Release 都靠 `SFORUM_REF`（默认 `main`）检出框架，需要锁定版本时改成 tag/commit。
Windows 未发布：宿主按 `backend/entry` 直接执行、不做 `.exe` 归一化，未经实机验证。

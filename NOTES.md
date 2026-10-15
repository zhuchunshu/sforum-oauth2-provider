# 开发笔记：sforum.oauth2-provider

本文件记录本次开发的关键决策与验证证据。**刻意写在插件包内**（`extensions/dev/*` 已被
`.gitignore` 覆盖），因此不会在 `knowledge/` 下产生 tracked 变更——本次任务要求不影响
当前 git 环境。若要归档到项目知识库，请手动把下面内容移入
`knowledge/modules/` 与 `knowledge/sessions/`。

## 决策

1. **形态选择**：Host 的 Identity 体系里已有“SForum 作为 OAuth2 客户端”的适配器
   （`extensions/builtin/plugins/sforum-auth-github`）。本次方向相反：SForum 作为授权服务器。
   Host 没有 provider 侧插槽，因此走**插件自有 HTTP 路由 + 自有数据库**，不新增 Core 代码。
2. **数据库**：manifest `database.grants: ["own_schema"]`，独立 schema/role
   `sforum_oauth2_provider`；运行时经 `SFORUM_DATABASE_URL` 租约用 pgx 直连（与
   `extensions/dev/plugins/sforum.marketplace` 同一成熟模式），无租约时回退进程内存储。
3. **授权确认页用 `mode: stream`**。原因：Host 对 `action: add` + `mode: http` 的路由强制
   `responseSchema` 校验，且 schema 编译只接受 `application/json`；公共资产端点
   （`publicPackageAssetContentTypes`）也不服务 `.html`。`mode: stream` 的正文是
   “opaque Protocol V2 chunks”，preflight 可声明 `Content-Type: text/html`，
   因此确认页由插件直接输出 HTML。
4. **访问令牌经 `access_token` 查询参数交付**。Host 插件路由默认 `filtered` authority，
   会剥离 `Cookie`/`Authorization`；`core.guard.raw_request` 在当前 Host 版本返回
   `ErrRouteGuardUnavailable`。故 `Authorization: Bearer` 到不了插件进程。
5. **非浏览器客户端需要 CSRF 握手**。Host 对注册表管理的路径统一启用 CSRF 中间件
   （`routeRegistryManagedOnly` + `/api/oauth/*` 命中插件路由 → managed），unsafe 方法需要
   double-submit token；唯一豁免是 `/api/v1/webhooks/inbound/*` 前缀与 `Bearer sft_` PAT。
   插件路径无法获得豁免，因此提供 `GET /api/oauth/csrf` 显式握手。
6. **命名空间**：所有 id/contractVersion/设置键/权限键都以扩展 id
   `sforum.oauth2-provider` 为前缀——`v3Validator.versionedID` 要求
   `strings.HasPrefix(id, manifest.ID+".")`，最初用 `sforum.oauth2.route.*` 会被判 `invalid manifest`。
7. **管理员页面免构建**：`frontend/admin/dist/clients.mjs` 是手写 ESM（`apiVersion=1` +
   `mount(target, bridge)`），不引入 Vite/Nuxt 构建，符合“只有预构建产物被加载”的约束。
   它直接调用插件自身 `/api/oauth/*` 路由，并按 double-submit 规则附带 `X-Csrf-Token`。

## 启用过程中实测到的四类 Host 约束（已修入代码）

1. **路由注册表准入**：`action: add` 的路由无论 `mode`（含 `stream`）都必须声明
   `responseSchema`，否则 `routes: invalid route registry admission: action "add" requires valid schemas`。
2. **wire schema 引用必须是版本化契约**：包内路径（`schemas/x.json`）能过 Manifest 校验与
   `extension test`，但分发时报 `protocol v2 schema reference "schemas/x.json" is invalid`。
   改用 `…@1` 引用并随包提供 `openapi/routes.yaml`（每条路由一个 operation，
   含 2xx 带 schema 的响应 + `default` 回退）后通过。
3. **响应必须自带 JSON Content-Type**：否则 `extensions.route_schema_invalid:
   exactly one Content-Type is required`。
4. **插件 → Host 调用必须 actorless**：Host 的 `validateProtocolV2HostContext` 拒绝携带
   Actor 的插件请求（`protocol v2 actor context is not host-attested`），设置读取与
   userinfo 都会失败。

另修掉一个自研引擎的真 bug：**PKCE S256 曾用 hex 编码比较 challenge**，而 RFC 7636 要求
base64url（无填充），导致按规范的客户端必然失败；单测当时用同样的 hex 假设而"通过"，
现已改为 base64url 并增加"hex 必须被拒绝"的回归断言。

## 验证证据

- `cd backend && go test ./...` → 全绿（含 `schema_alignment_test.go`：用与 Host 相同的
  `santhosh-tekuri/jsonschema` 校验每个 handler 的真实响应与 `schemas/*.json` 一致）。
- `go run ./cmd/sforum extension validate <pkg>` → OK（routes 16 / settings 6 / langs 2）。
- `go run ./cmd/sforum extension digest --write <pkg>` → 写入 23 个 packageFiles 摘要 + 后端摘要。
- `go run ./cmd/sforum extension test <pkg>` → PASS（errors 0 / warnings 0 / checks 6）。
- `git status --porcelain` → 本插件全部落在 `extensions/dev/*`（gitignored），无 tracked 变更。
- 端到端浏览器实测（本机 dev 环境，插件已 enabled，运行 `4d629d87…`）：
  授权页 →（PKCE `S256`）→ 确认页（显示「以 Inkedus 的身份授权」）→ 允许 → 回跳
  `?code=sfoc_…&state=…` → `POST /api/oauth/token` 200（`sfoa_…`/`sfor_…`，`expires_in=3600`）
  → `GET /api/oauth/userinfo?access_token=…` 200 `{sub:"1",username:"zhuchunshu",displayName:"Inkedus",status:"active",scope:"profile"}`
- 安全语义实测：授权码重放 400 `invalid_grant`；刷新令牌轮换后旧令牌 400；内省返回
  `{active:true,sub,scope,username,exp}`；`revoke` 后 userinfo 立即 401 `invalid_token`；
  撤销授权后确认页重新出现（`revokedAuthorizations:1, revokedTokens:6`）。
- 管理面板 `/control-panel/extensions/sforum.oauth2-provider/pages/clients` 实测渲染应用列表
  （名称 / client_id / 类型 / 回调地址 / 权限 / 归属 / 重置密钥 / 删除）。

## 管理端 UI 改版（第二轮）

- 由「内嵌表单 + 宽表格」改为**列表卡片 + 模态**：新建/编辑、一次性密钥、危险确认三种视图共用一个 modal。
- 布局问题逐个修掉：横向滚动条（改固定表格 + 百分比列宽，合计 100%）、操作列被挤成竖排
  （`white-space: nowrap` + 19~21% 列宽）、时间列三行换行（紧凑 `YYYY-MM-DD HH:mm`）、
  `client_id` 过长导致应用列被撑开（表格短显 + `title` 完整值 + 复制按钮）。
- 移动端（390x844）验证：≤900px 转堆叠卡片，首列标签独立成行；模态转底部抽屉。
- 暗色验证：临时给 `<html>` 加 `.dark`，卡片/边框/文字/按钮/徽章均随 Host 变量切换。
- 实测流程：空名称与非法回调地址的就地校验、创建 → 密钥视图（两个复制按钮）、
  删除二次确认 → 列表刷新（应用数从 2 回到 1）。
- 注意：插件组件里 `i-tabler-*` / `i-lucide-*` 类不生效（无 iconify 运行时），因此按钮全部用文字，
  不引入 emoji 或自绘图标。

## 待办（需要人工动作）

1. 重启 API 让 `SyncBuiltins`/外部根扫描发现新包：`./scripts/api-dev.sh`。
2. 管理员启用插件并完成 exact-artifact `super_admin` 信任确认，再启用管理页组件。
3. 给需要的角色授予 `sforum.oauth2-provider.manage_clients`。
4. 端到端冒烟：
   ```bash
   curl -c jar.txt -s https://站点/api/oauth/csrf
   CSRF=$(awk '$6=="csrf_" {print $7}' jar.txt)
   curl -b jar.txt -X POST https://站点/api/oauth/clients/create -H 'Content-Type: application/json' \
     -H "X-Csrf-Token: $CSRF" -H "Origin: https://站点" \
     -d '{"name":"Smoke","clientType":"public","redirectUris":["http://127.0.0.1:8000/callback"]}'
   # 浏览器打开 /api/oauth/authorize?...（带 PKCE）→ 确认页 → 回跳 code → POST /api/oauth/token → GET /api/oauth/userinfo
   ```
5. 可选：把本文件的决策与结论合并进 `knowledge/modules/`、`knowledge/sessions/`（会产生 tracked 变更）。

## 已知未覆盖项

- 未实现 `client_credentials` 与设备码流程（v1 范围外）。
- 未发 OIDC `id_token`（只有 userinfo）；如需 OIDC，需要签名密钥管理与 `/.well-known/openid-configuration`。
- 邮箱/头像未纳入 userinfo：属 Host 扩展用户字段，需要 actor 权限。
- 并发脚本压测与限流策略未做（Host 路由策略自带 rate limit 由 Host 决定）。

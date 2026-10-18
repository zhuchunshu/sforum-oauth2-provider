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
8. **授权确认页读取宿主公开配置与当前会话**（2026-10-18）：页面主色 / 浅色背景 /
   topbar 站点品牌 / 账号头像都来自两个同源 GET——`/api/v1/web-options`（public，
   含 `site.name`、`site.url`、`site.favicon_url`、`site.logo_url`、`appearance.theme`、
   `appearance.light_background`）与 `/api/v1/auth/session`（登录态 cookie，提供
   `avatar` 视图与 `appearance` 个人偏好）。这两条都是宿主 Core 契约：
   选项公开性在 `apps/api/app/Models/Options/`，会话形状在
   `apps/web/app/composables/identity/useAuthSession.ts`；宿主改动时同步更新
   `consent_page.go`。主题预设表与 `custom:#rrggbb` 混合规则是
   `apps/web/app/assets/css/main.css`（`data-sforum-theme` 块）与
   `apps/web/app/utils/settings/appearance.ts` 的镜像，头像色板/首字规则镜像
   `apps/web/app/utils/forum/forumListPresentation.ts` 与 `SFAvatar.vue`——宿主新增
   预设时需同步，`consent_page_test.go` 的 `node --check` 只兜语法不兜契约。
   明暗模式跟随站点 `nuxt-color-mode` localStorage 偏好（system/light/dark）。
9. **前端一律不写 `document.title`**（2026-10-18）：标签页标题是宿主 SEO 管道的领域——
   Page Registry 贡献声明 label → `apps/web/app/pages/[...sfRegistryPage].vue` 的
   `pageTitle` computed（label 优先于正文首个 h1 提取）→ `useSForumSeo` →
   `seoResolver` 按 `seo.page.title_template`（默认 `{pageTitle} | {seoSiteName}`）
   渲染 → unhead 输出。插件前端手写 `document.title` 直改 DOM 会绕过 unhead，把带
   站名后缀的合规标题覆盖成裸页面名，且在 unhead 之后的执行时序下永不被恢复——
   这正是「前台页面完全加载后 title 不对」的根因。确认页（stream 独立 HTML，不在
   宿主 Nuxt 内）是唯一例外：它解析 `seo.page.title_template` / `seo.site.name` /
   `seo.site.inherit_site_name` 自行对齐宿主语义。
10. **「切换账号」= 退出会话 + 整页回登录页 + 回到同一授权请求**（2026-10-18）：
   确认页的 `return` 参数此前是死代码（`fallbackAuthorizeQuery` 读它，`handleAuthorize`
   从不写），按钮只能回退成裸的 `/api/oauth/authorize`；而宿主 `/login` 对已登录用户由
   `guest` 中间件直接回跳，所以链接即使带全参数也只会绕回确认页。现在：
   `handleAuthorize` 把原始授权请求（path + 原始 query）作为 `return` 传给确认页，
   `isAuthorizeReturnURL` 只接受 `Path == /api/oauth/authorize` 且带查询串的相对路径
   （`return` 来自地址栏，不能当可信输入）；确认页的 `request` / `return` 改用
   `json.Marshal` 注入——`<script>` 不解析 HTML 实体，`html.EscapeString` 会把 `&` 变成
   字面量 `&amp;` 把授权地址拼坏，而 JSON 编码同时免疫 `</script>` 截断；点击时先
   `POST /api/v1/auth/logout`（double-submit `csrf_`，缺 token 先打 `/api/v1/health` 种）
   再 `location.assign('/login?redirect=<原始授权请求>')`。
   **宿主侧配套约束**：认证回跳目标是 `/api/` 或 `/plugin/` 时必须整页导航。客户端路由
   会把它们交给 `[...sfRegistryPage]` catch-all（渲染成 “Failed to load extension page”），
   端点永远收不到请求——这条不只影响切换账号，也让「未登录 → 登录 → 回到授权端点」整条
   链路失效。已在宿主 `apps/web/app/utils/identity/authReturn.ts` 增加
   `requiresDocumentNavigation`，由 `useAuthReturnNavigation.returnFromAuth` 分流；
   宿主改这里时插件侧无需跟进，但**升级宿主前要先确认这条存在**。

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

## 迭代期实测到的制品切换约束（2026-10-17 补记）

同版本号（1.0.0）下反复改前端制品时，踩到三条宿主行为，值得记下来省时间：

1. **外部源只在 API bootstrap 扫描一次**：`SyncExternalSources` 由启动流程调用，
   后台没有「重新扫描」入口。改完包、跑过 `prepare-package.sh` 后，宿主内存里仍是旧制品，
   卡片上不会出现「待升级」。正确顺序：**禁用 → 重启 API（重新扫描并暂存新制品）→ 启用/升级**。
2. **L2 组件内容变化必须提升 `components[].contractVersion`**：只改 `frontend/public/*` 的内容
   而不动 contract 版本，upgrade 会在组件注册表阶段被拒：
   `component registry contract conflicts with the active snapshot: contribution … changed without a contract version`。
   本次把 `sforum.oauth2-provider.component.apps@2` 提到 `@3` 后通过。
3. **`upgrade` 的前置条件是 extension 处于 enabled**：`disabled` 状态下 `Upgrade()` 直接返回
   `ErrLifecycleCoordinatorInvalid`（HTTP 422）。而 `disable → enable` 只会启用**当前活动版本**，
   不会把 staged 提到 active；staged 只能通过 `upgrade` 或安装路径切换。
   若 upgrade 报 `asset registry … exact fence conflict`（本地资产注册表 revision 竞态），
   用恢复接口重试同一次操作即可成功：
   `POST /api/v1/admin/extensions/<id>/lifecycle/<operationId>/recovery` body `{"decision":"retry","reason":"…"}`。


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
- userinfo 头像兜底实测（2026-10-18，1.2.0 新制品）：授予 `avatar` 后，上传过头像的
  id 1 → `avatar:"/media/avatars/…"`；initials 账号 id 3（种子用户2）→
  `avatar:"data:image/svg+xml;base64,…"`（解码后首字「种」，色板 `#5c7cfa`）；
  仅 `profile` → 响应无 `avatar`；撤销令牌 → 401 `invalid_token`。
  浏览器全链路（登录 → 确认页 2 项权限 → 允许 → demo 回调 → userinfo）通过，
  demo 结果页 `<img>` 以 128×128 渲染生成的 SVG；截图见
  `docs/images/oauth-consent-avatar-scope-2026-10-18.png` 与
  `docs/images/oauth-demo-userinfo-avatar-2026-10-18.png`。
- 安全语义实测：授权码重放 400 `invalid_grant`；刷新令牌轮换后旧令牌 400；内省返回
  `{active:true,sub,scope,username,exp}`；`revoke` 后 userinfo 立即 401 `invalid_token`；
  撤销授权后确认页重新出现（`revokedAuthorizations:1, revokedTokens:6`）。
- 管理面板 `/control-panel/extensions/sforum.oauth2-provider/pages/clients` 实测渲染应用列表
  （名称 / client_id / 类型 / 回调地址 / 权限 / 归属 / 重置密钥 / 删除）。

## 应用图标统一规范（2026-10-18）

1. **单一事实源在后端**：`backend/icon.go` 计算 icon 描述符
   `{kind: attachment|url|generated, url?, initials, background, foreground}`，随 `/clients`、
   `/authorizations`、确认页 summary 下发；三处前端（确认页 / 前台卡片 / 管理端列表）
   只按描述符渲染，杜绝三套哈希算法漂移（旧实现确认页取首字、前台取首字符、管理端没有头像）。
2. **生成算法**：种子 = `client_id`（稳定、公开、唯一；确认页/授权记录查不到 client 时
   退回 clientName）；`hue = (hue*31+rune)%360` 逐 rune 累加，渐变
   `linear-gradient(140deg, hsl(H 70% 55%), hsl((H+40) 68% 42%))`；首字母规则沿用旧
   initials()（ASCII 词首两字母大写，非 ASCII 前两字）。custom 也带生成兜底字段，
   `<img>` 失败移除即露出 tile，永不空白。
   > **已被取代（2026-10-18，见「默认图标改为 GitHub 风格 identicon」一节）**：渐变 + 首字母改为 GitHub 风格 identicon
   > （方型浅灰纯色底 + 5×5 镜像像素块，无文字）。本节其余条款（单一事实源、来源优先级、
   > logoUrl 校验、`<img>` 失败回退）继续有效。
3. **logoUrl 校验**（`validLogoURL`）：允许空；非空须 https（loopback http 放行）、
   有 host、无 fragment、≤2048。存 `oauth_clients.logo_url`（建表列 + `ADD COLUMN IF NOT
   EXISTS` 幂等迁移）；「未传保留、传空清除」语义与 description 一致。
4. **契约升版**（宿主 exact-artifact 准入要求形状变更必须提版）：`clients.response@4`、
   `client-write.request/response@2`（三条写路由 @2）、`consent-summary.response@2`、
   `authorizations.response@2`（authorizations-list 路由 @2）、`component.apps@4`；
   manifest routes/openapi/`constants.go` 三处引用同步。（2026-10-18 图标上传那一版又各自
   +1：`clients.response@5`、`client-write.request/response@3`、`consent-summary.response@3`、
   `authorizations.response@3`。）
5. 已知权衡：消费端直接向 logo 域名发图片请求（`referrerpolicy=no-referrer` 缓解）。
   2026-10-18 起二进制上传**改走 Core 的附件体系**（见下一节），URL 只作为次选入口保留。

## 默认图标改为 GitHub 风格 identicon（2026-10-18）

需求：无自定义图标的应用不应显示「渐变 + 首字母」，默认形态要参考 GitHub 的 identicon。
中间试过一版「SFAvatar 混合」（像素块 + 首字母叠加、SFAvatar 8 色板），操作者随后拍板
**完全对齐 GitHub**，混合形态已删除（`initials` 字段一并从描述符与四份 schema 里移除）。
最终定稿：

- **形态**：方型 tile（沿用现有圆角），纯色底 `#f0f0f0`（GitHub identicon 同款浅灰，
  不随主题变化，内描边 1px `rgba(0,0,0,.06)`），**删除所有渐变**，**不画任何文字**。
- **图案**：5×5 网格、左右镜像、nibble 偶校验填充（GitHub 的布局与填充规则），
  `inset 8%`（GitHub 实测边距约 8.3%）。15 个独立格 = 5 行 × 左中三列；
  第 4/5 列镜像第 2/1 列；全空时强制点亮中心格。
- **哈希原语**：`hash = hash*31 + UTF-16 codeUnit`（`stableHash`），不引入 MD5——
  Go 与三份 JS 镜像（前台 `apps.mjs`、管理端 `clients.mjs`、确认页内联脚本）可以逐位复现。
  **不追求与 GitHub 位级一致**，只对齐布局、填充规则与色块观感。
- **颜色**：单色像素块，色相 = `stableHash(seed) % 360`，`S 60% / L 63%`
  （抽样 GitHub 默认头像：S 50–65%、L 56–70%）；`identiconColor` 是唯一入口。
- **类型区分**：删掉 `.sfp-avatar--pub/--conf` 的 `hue-rotate`（会把色相旋飞），
  公开/机密交给既有文字标签。
- **userinfo 头像兜底同步换装**：`generatedAvatarDataURL` 从「圆形 + 首字」改为 128×128
  GitHub 风格 identicon（`viewBox 0 0 6 6`，5×5 网格居中 = 8.3% 边距），无文字。
  **确认页「当前账号」头像不在此列**——它继续逐像素复刻宿主 SFAvatar（那是站点头像，
  不是本插件的兜底头像）。
- **描述符**：`icon = {kind, url?, pattern, background, color}`——`pattern` 必填
  （25 个 `0/1`，行优先上→下、左→右）；`background` 恒为 `#f0f0f0`；`color` 是色块颜色；
  **`initials`/`foreground` 已删除**。`url` 仍只在 `attachment`/`url` 形态出现，
  两种形态照旧带全套兜底字段。
- **契约升版（两轮）**：混合形态那一版到 `clients.response@6` / `client-write.response@4` /
  `consent-summary.response@4` / `authorizations.response@4`、`component.apps@11`；
  改纯 GitHub 形态（删 `initials`、加 `color`）再各 +1：`@7` / `@5` / `@5` / `@5`，
  路由 `contractVersion` 同步 +1（`clients-list@5`、`clients-create@5`、`clients-update@5`、
  `clients-secret-rotate@5`、`authorizations-list@5`、`consent-summary@5`），
  `component.apps@12`；包版本 **1.5.0**（与并行的 userinfo 账号字段改动 1.4.0 同批上线）。
- **JS 镜像对齐**：两份 `.mjs` 与确认页内联脚本各带 `stableHash` / `identiconPattern` /
  `identiconColor`（表单预览与描述符缺失兜底才用）；实测三份镜像与 Go 的 golden 值逐字一致，
  `backend/icon_test.go` 固定了 golden，任何漂移都会变红。
- **已知代价**：深色主题下 `#f0f0f0` 方块偏亮（与 GitHub 一致，不做主题反转）；表单新建态
  的预览种子是名称、保存后种子变 `client_id`，图案会在保存后变化（与旧版色相行为一致）。

## 图标上传复用 Core 附件（2026-10-18）

1. **插件不碰字节**：浏览器把图片 POST 到 Core 的 `/api/v1/attachments`（multipart 字段
   `file`，带用户自己的会话 cookie + `X-Csrf-Token`），插件只拿 `publicId` 调
   `sforum.attachments.extension.claim@1`；自己的库只存
   `oauth_clients.attachment_public_id`。没有插件上传路由、没有 base64、没有私有目录。
2. **`expectedRevision` 的来源（与任务书不一致，已按 Core 实际契约实现）**：claim 要求
   请求信封的 `expectedRevision` 精确等于 `attachments.updated_at`，但 Core 的上传响应
   （`attachmentDTO` / OpenAPI `Attachment`）**只有 `createdAt`，没有 `updatedAt`**。
   两张表时间戳都是 `DEFAULT now()`，同一条 INSERT 内取值相同，因此用上传响应的
   `createdAt` 当修订号，首次 claim 成立；被别的写路径推进过则 Host 返回
   `host.attachment_revision_conflict`，插件映射成「图片已被修改，请重新上传」。
   若以后 Core 给上传响应补上 `updatedAt`，改一行取值即可。
3. **`Idempotency-Key` 是硬前提**：Host 只在 HTTP 请求带该头时才签发 Host Command 的
   actor 委派（`attachInvocationActorDelegation` 的 `idempotencyKey != ""` 分支），
   缺了它 `DelegatedCommandRequest` 返回 `ErrHostActorDelegationUnavailable`——与「旧宿主」
   不可区分。所以插件在调命令之前先检查 `Context.IdempotencyKey`，缺失直接返回
   `idempotency_key_required`（400），不误报成 `icon_upload_unsupported`。
4. **旧宿主 / 缺权限统一降级**：`Context.IdempotencyKey` 有值但没有
   `HostCommandDelegations`，说明宿主版本过老、当前精确制品没被授予 `host_commands`、
   或账号没有 `attachment.upload`——三者都返回 `icon_upload_unsupported`，前端展开
   「或使用图片地址」并提示升级，**绝不静默退回 base64 内联**。
5. **互斥语义**：`attachmentPublicId` 与 `logoUrl` 一次保存只生效一个；
   上传 → claim 后清 `LogoURL`；填地址 / 清空 → 先 `release` 再清 `AttachmentPublicID`。
   「两个字段都不提交」= 没动图标（保留原图），否则每次编辑都会重 claim 并撞修订号。
6. **删除应用**：best-effort release（失败只记日志），因为插件卸载时 Host 会兜底释放本扩展
   全部引用，而卡住用户删不掉应用更糟；附件本体等 `attachment.cleanup_orphan_after_days`
   （默认 30 天）回收，这就是误删宽限期。
7. **单测怎么顶替 Host**：`oauthServer.hostOverride` 注入
   `&pluginv2.Host{Commands: fake}`，委派由测试直接挂在 `RouteRequest.Context` 上
   （`HostCommandDelegations` + `IdempotencyKey`），不需要 go-plugin 运行时。

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

## scope 的三层解析（2026-10-18）

现象：后台给应用加了 `avatar`，确认页仍然只显示「1 项权限」。

根因不在确认页，而在授权端点：`verifyAuthorizeRequest` 只读请求里的 `scope` 参数
（`normalizeScopes(query.Scope, allowed)`），`client.Scopes` 从未参与授权流程——它只在
创建/编辑时被规范化入库、在列表里回显。于是「应用配置的权限」对实际授权没有任何影响；
第三方（demo）还把 `scope=profile` 写死在代码里，两层叠加就成了看到的 1 项。

决策：引入 `resolveAuthorizeScopes(requested, registered, allowed)`，三层约束从内到外：

1. `requested` 为空 → 用 `registered`（RFC 6749 §3.3 允许服务端预定义默认值）。
2. `allowed`（站点 `allowed_scopes`）是硬上限，沿用 `normalizeScopes`，`profile` 必选。
3. `registered` 是该应用的自我约束：请求超出 → `invalid_scope`。没有第 3 层，任何拿到
   `client_id` 的应用都能索取站点开放的全部权限。

`profile` 在容量层无条件放行：存量应用的 `scopes` 列可能没有它，但 `normalizeScopes`
会强制并入，不能因此把请求打回。

顺带修掉的两处：
- 确认页文案只认 `profile`，其余 scope 显示裸键名 → `scopeRegistry` 增 `LabelZh`，
  `scopeLabel` 改为查注册表（未登记时回退键名）。
- 已知 client 之后的 OAuth 错误被丢掉了 client，`handleAuthorize` 因此永远走 `routeFailure`
  而不是回跳。现在 `verifyAuthorizeRequest` 在错误分支返回已解析的 client，错误按
  RFC 6749 §4.1.2.1 回跳已登记回调。

## 迭代期踩到的两个运维坑（2026-10-18）

1. **`prepare-package.sh` 会与 dev plugin watcher 抢同一个 `backend/plugin`**：watcher 在文件
   变动时自己重建包，`digest --write` 写完立刻复验就可能读到 watcher 刚覆盖的二进制，报
   `package file digest mismatch: backend/plugin`。这不是摘要算错——等几秒重跑
   `extension validate` 就通过，别去改脚本。
2. **用 curl 调 `POST /api/v1/admin/extensions/:id/upgrade` 必须带 `Idempotency-Key` 头**：
   服务端把它写进 `BuildLifecycleCoordinatorRunInput` 并要求非空，缺了就返回
   `422 extension.lifecycle_invalid`（提示文案是「请刷新扩展信息后重试」，会把人带偏）。
   正确姿势：先 `POST .../trust/challenge?target=staged` 拿 `confirmationToken`，再带
   `X-Csrf-Token` + `Origin` + `Idempotency-Key` 提交升级。

## 新建/编辑应用弹窗字段重复（2026-10-18）

现象：前台 `/settings/oauth-apps` 的「新建应用 / 编辑」弹窗把前三个字段各渲染两遍
（应用名称、应用说明、应用图标 URL；`sfp-name`/`sfp-description`/`sfp-logo` 三个
id 重复，label 只绑定到第一个输入框）。

根因是**在役制品过期**，不是当前源码：线上实际加载的 `apps.mjs` 摘要为
`ce87c2aa…`，其弹窗表单块里同时存在「旧字段块（无必填/可选标签）+ 新字段块」；
仓库里的源码早已只有一份（`6a667dde…`），但宿主按 exact-artifact 运行，
`重启` 不会切换制品，必须升级。

修复与上线步骤（同版本号 1.1.3 也适用）：

1. 删除重复字段块（只保留带标签的一份），补
   `TestPageScriptModalFieldsAreUnique`（解析弹窗表单块，断言字段 id 唯一）。
2. **L2 组件内容变了必须升契约**：`component.apps@5` → `@6`。不升版宿主会在
   registry publication 阶段拒绝：`contribution … changed without a contract version`。
   这个错误在 UI 上只显示「扩展生命周期操作失败」，真实原因要去
   `extension_lifecycle_operations.error_message` 查。
3. `./scripts/prepare-package.sh` 刷新摘要 → 后台「升级」→「授权当前精确制品」→
   「确认并升级」（或按 NOTES 里的 trust challenge + upgrade API 两步）。
4. 验收：弹窗表单 `.sfp-field` = 6、`#sfp-name` 计数 = 1；桌面 + 390px 窄屏均无横向滚动。

踩坑：dev watcher 会随文件变更持续 re-stage，升级过程中若制品被重写，会报
`asset registry revision changed during replacement` / `supplied manifest does not match
snapshot`。等文件静默（staged_version_id 连续几秒不变）后再发起升级即可。

另一个启动期坑：同一 `(version, package_digest)` 上若存在**两条未撤销的 enable trust
grant**（手动 challenge 与 dev auto-promote 各发一条），API 启动会在
`restore extension route publications` 阶段失败：
`restore navigation authority for …: extension lifecycle registry publication exact fence
conflict`（`exactLiveGrantImpact` 要求唯一 live impact，见
`app/Support/Extensions/lifecycle_registry_publication_assets.go`）。
恢复办法：保留最新一条、把旧的那条 `revoked_at` 置为 now()，再重启 API：

```sql
UPDATE extension_trust_grants SET revoked_at = now()
WHERE extension_id = 'sforum.oauth2-provider'
  AND extension_version = '1.1.3'
  AND package_digest = '<active digest>'
  AND revoked_at IS NULL
  AND id <> (SELECT max(id) FROM extension_trust_grants WHERE ...); -- 只留最新
```

## 操作反馈改为 Toast（2026-10-18）

前台 `/settings/oauth-apps` 的所有文字提示（新建/更新/重置/删除/撤销的成功反馈、
应用与授权列表加载失败、表单校验与请求失败、复制反馈、制品漂移诊断）统一走宿主
Nuxt UI Toast；页面顶部 `.sfp-status` 退回「宿主 Toast 管线不可用」时的兜底通道，
弹窗内 `.sfp-form__error` 文字报错已删除。

- Toast 通道：`app.vue` 的 `<UApp :toaster>` 渲染 `useState('toasts')`，即
  `payload.state.$stoasts`（reactive 代理）。组件在 Vue 上下文外直接向该数组投递，
  即可复用宿主同一套渲染管线（右上角、success/info 4s、error 6s、`duration: 0` 常驻）。
- L2 组件内容一变就必须升契约：本次 `component.apps@6` → `@7`，否则 dev auto-promote
  报 `contribution … changed without a contract version`（UI 只显示生命周期失败）。
- 踩坑：连续失败的 auto-promote 会残留多个插件运行时实例与
  `extension_database_runtime_leases` 行；之后升级/重启报
  `extension runtime instance is not active` / `reserve runtime lease capacity`，
  前台整页 404（Page Registry 仍在，但运行时实例已死）。恢复方式：重启 dev API
  （`kill $(pgrep -x air)` 后重新 `./scripts/api-dev.sh`）；启动时按 active version
  重新拉起插件，随后 watcher 会把最新制品提为 active。

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
- 邮箱已接入（2026-10-18，宿主同意式读取落地后）：`email` 是可选范围，授权码签发时用宿主的
  一次性委派快照本人邮箱，userinfo 只读快照。细节见「email 同意式快照」一节。
- `bio` / `location` / `websiteUrl` 是公开数据（公开资料页对游客可见），但 `sforum_core_v1`
  没有对应视图、稳定查询里也没有这些列；要加只能宿主扩面，或抓
  `/api/v1/profiles/:username`（受 `forum.guest.read` 策略影响、payload 偏重，暂不采用）。
- 并发脚本压测与限流策略未做（Host 路由策略自带 rate limit 由 Host 决定）。

## userinfo 头像兜底（2026-10-18）

- **现象**：应用勾了 `avatar`、确认页也列了两项权限、令牌 scope 确实是 `profile avatar`，
  但 `userinfo` 里没有 `avatar`。
- **根因**：宿主 core-safe 投影的 `avatar` 是 URL 字符串；initials（站点默认
  `avatar.default_provider=initials`）由前端 `SFAvatar` 现场渲染，宿主侧没有 URL。插件此前
  按「URL 非空才下发」处理，于是默认安装下字段整个消失（本机 81 个用户档案、0 个上传头像）。
- **决策**：授予 `avatar` 即下发；宿主 URL 优先，缺失时生成确定性 SVG data URL
  （`backend/avatar.go`）。首字规则、8 色板与 UTF-16 哈希与 `SFAvatar.vue` +
  `forumListPresentation.ts` 对齐，同一用户永远同一张图；显示名含 `&<>"'` 时先做 XML 转义。
- **为什么不是新图片路由**：插件拿不到站点绝对地址（Host 不给 `site.url`，`Host` 头在插件
  路由里被过滤），相对地址对第三方服务端同样不可用；新路由还要额外契约、缓存与公开面。
- **为什么不是结构化 avatar 对象**：消费方都得自己实现首字渲染，DX 差，还要升契约。
- **绝对地址解析**（`backend/baseurl.go`）：宿主只给相对媒体路径，插件按
  运行时 `site.url`（`GET {APP_URL}/api/v1/web-options/site.url`）→ 进程 `APP_URL`
  的顺序补成绝对链接，缓存 5 分钟；两者都拿不到时保留相对路径，不猜测域名。
  只有宿主给相对地址时才发这次请求（Gravatar / 静态图 / 首字兜底都不查）。
- **已知边界**：宿主将来若在 core-safe 投影里直接给绝对地址或 avatar kind，可删掉
  `baseurl.go` 的解析；星平面字符（emoji）的首字按完整 rune 取，与 SFAvatar 的
  UTF-16 首单元略有差异。

## userinfo 账号时间线 / 主页链接（2026-10-18）

- **背景**：对比 GitHub `/user`，userinfo 缺 `created_at`/`updated_at`/`html_url`（以及 email）。
- **决策**：补 `id`（数字）、`createdAt`、`updatedAt`、`profileUrl`；email 与 profile 详情字段
  不在本仓解决（理由见上）。
- **数据来源**：宿主稳定查询 `sforum.core.safe_user.by_id`（读 `sforum_core_v1.safe_users`，
  公开数据），清单声明 `database.grants` 增加 `core_views`。请求形状、int64 过滤器与结果类型
  见 `backend/account.go`；单测 `backend/account_test.go` 同时钉住请求形状（查询 id、plan 版本、
  字段白名单、`sforum.core.query.int64@1` 过滤器、page limit=1、**actorless**）。
- **为什么不是抓公开资料接口**：`GET /api/v1/profiles/:username` 虽然也带 `joinedAt` 和
  bio/location/website，但它受 `forum.guest.read` 策略约束、要跑近期主题/活动等额外查询，
  每次 userinfo 多一跳重请求；稳定查询是一次主键索引查询，形状由宿主校验。
- **降级契约**：查询失败/无行/旧宿主/无 Query 通道 → `createdAt`/`updatedAt` 整体缺席，
  userinfo 其余字段与 200 不变；失败原因只记日志（相同原因去重），不进响应。用户不是
  `active` 时 `safe_users` 查不到行——这不是错误。
- **`profileUrl`**：`{site.url}/u/{username}`，`url.PathEscape`；站点地址解析不出来就省略
  （相对地址对第三方服务端无用，猜域名更糟）。与 `avatar` 共用 `publicBaseURL` 缓存。
- **契约**：`userinfo.response@2` → `@3`（新增四个可选字段，`required` 不变）；
  `id` 固定数字形态，schema 单测显式拒绝字符串 id。
- **代价与注意**：升级时影响评审要确认 `core_views`（宿主新的读授权面），否则查询返回
  `host.query_core_views_denied`、字段缺席；宿主构建需包含 E10 稳定查询（本机检出已含，
  尚未发版），旧宿主只会缺字段。

### email 的三条路（2026-10-18 已选 1）

1. **宿主新面（已落地）**：宿主新增 capability `users.email.read` + actor 委派门控的
   `core.query.identity.self_email@1`（`sforum.core.self_email@1`）；插件在授权码签发时快照
   `{email, emailVerified, capturedAt}` 存 own_schema，`email` 范围授予时下发，撤销/收窄即删。
2. **`raw_core` 授权**：能直接读 `public.users.email`，但该授权是 `public` 全表 SELECT
   （含 credentials、会话、API token），为一个字段扩大到这个 blast radius 不划算。已否。
3. **不做**：第三方改用站内账号关联。已否。

## email 同意式快照（2026-10-18）

- **宿主面**（SForum 核心已实现并验收）：capability `users.email.read`（risk high）+ Query
  Registry 投影 `core.query.identity.self_email@1`（plan `…plan@1`、结果 schema
  `sforum.core.self_email@1`、scope `route`）；宿主在**有活用户的 route/admin 调用**里把一次性
  query 委派放进 `RequestContext.host_query_delegations`，token 单次、≤2 分钟、绑定精确制品。
  错误语义：`host.query_actor_delegation_invalid` / `_replayed` / `host.query_capability_denied`
  / `host.query_rate_limited`（60 次/分钟/(actor,extension)）/ `host.query_runtime_stale`。
- **插件侧决策：快照，而不是每次现读**。委派只存在于当前调用，userinfo 是服务端到服务端
  （没有用户会话、没有委派），因此必须在签发授权码时读一次并落库。两条签发路径都要覆盖：
  确认页允许（`handleConsentDecision`）与免确认直放（`handleAuthorize`）。
- **存储**：`oauth_email_claims(user_id, client_id, email, email_verified, captured_at)`，主键
  `(user, client)`。不跨应用共享，也不做「读一次给所有 client 用」——那会越过单次授权。
- **删除时机**：撤销授权（`/api/oauth/authorizations/revoke`）、删除应用、以及新授权收窄到
  不含 `email`。前两者是显式用户动作，后者由同一次授权覆盖旧快照。
- **降级**：委派缺失（匿名/job 路径、能力未确认）、限流、宿主错误 → 不写快照，userinfo 缺
  `email`/`emailVerified`，**授权照常成功**。失败原因只记日志（原因变化才打），不进响应。
- **为什么不在确认页展示 email**：确认页每次渲染都读会多一次审计与限流消耗，而范围文案已足够
  表达；确认页只列「读取你的邮箱地址与验证状态」。
- **为什么不做「授权时校验 email 是否变化」**：快照的语义就是「授权当时的值」，变化由下一次
  授权覆盖；userinfo 不承诺实时性，README 已写明。

### 活体验收踩到的两个坑（都已修/已记录）

1. **委派不能跟着 context 回传**。声明 `users.email.read` 后，插件的路由调用上下文里多了
   `host_query_delegations`；`hostCallContext` 之前只清 `Actor`，于是 userinfo 读身份时把委派
   原样回传，宿主判 `protocol v2 actor delegations are Host-to-plugin only`，userinfo 直接 503。
   修法：`hostCallContext` 同时清 `HostCommandDelegations` / `HostQueryDelegations` /
   `HostSessionDelegation`，委派只经 SDK 的 `DelegatedQueryRequest` 放进
   `QueryRequest.actor_delegation`。回归用例：`TestUserinfoStripsInvocationDelegationsFromHostCalls`。
2. **capability 快照在 API 启动时固定**。dev watcher 自动提升制品（或插件卡片「重启」）不会
   刷新运行时的能力集；此时宿主根本不签发同意式委派，表现为 `email` 静默缺席（授权仍成功、
   没有任何报错）。必须重启 API（`kill $(pgrep -x air) && ./scripts/api-dev.sh`）。
   排查口径：宿主 `audit_events` 里没有 `identity.email.consent.read` + 日志里没有
   `Host Query trace … query_id=core.query.identity.self_email` 就说明委派没发出去。

## 接入说明页（2026-10-18，2026-10-19 定稿）

`/docs/oauth2-provider`：面向接入方的站内文档。最终形态是**宿主公开页 + SSR 模板正文 +
L2 组件填表**：

- `theme.json` 声明页面（`access: public`，`contract: sforum.page.oauth2_provider_docs@1`），
  模板 `templates/oauth-docs.html` 在 manifest `templates[]` 里注册（精确制品，
  `viewModelSchema` 复用页面契约 id）；
- 正文、标题、表格骨架全部写在模板里 —— navbar / footer / 站点布局与主题由 Core 渲染，
  排版用 Core `.sf-prose`，卡片沿用 `.sf-card`；
- 公开 L2 组件 `sforum.oauth2-provider.component.docs`（`frontend/public/docs.mjs`）经模板里的
  `sf-extension-widget` 岂载，只做两件事：替换示例域名为 `location.origin`、从
  `GET /api/oauth/docs` 取三张表写进模板预留的 tbody；
- `docs.css` 只补 Core 没有的：容器加宽到 960px（`--narrow` 的 760px 会把端点路径挤换行）、
  `.sf-prose` 缺失的表格排版、方法徽标、端点表 `min-width: 640px`（窄屏横向滑动）。

### 两轮返工记录

1. **第一版：插件自绘整页（`mode: stream`）—— 被否**。技术上能跑，但绕开了宿主导航/页脚/外观
   与 SEO 管道，视觉上与站点割裂。结论：transactional 页面（确认页）可以自绘；内容页必须用
   宿主外壳。另外插件包无法给 `view: template` 页面注入 CSS（`assets[].scope` 只认组件），
   所以「模板 + 自己的样式表」必须借道一个公开 L2 组件来挂样式。
2. **第二版：`view: component` 整组件正文 —— 闪屏，被否**。正文等 JS 挂载才出现：
   实测正文首帧 ~4.9s、CLS 0.09，用户看到的是「空白卡片 → 整页文字」。
   **根因**：`view: component` 的正文只能由客户端组件产生，宿主 SSR 阶段没有内容可输出。
   修法：正文移回 SSR 模板（首帧即有），组件降级为「填表 + 换域名」——表格行的数据本来就
   要等 fetch，让它晚到是合理的；正文文字晚到则不是。
3. **顺带**：宿主把 table 单元格继承成 `nowrap`，`.sf-prose` 又不覆盖表格 —— 表格样式必须
   显式 `white-space: normal` + `overflow-wrap: break-word`，并用 `table-layout: fixed` +
   固定列宽；4 列端点表在窄屏会给 `min-width` 走横向滑动，避免「说明」被压成一字一行。

### 三张表为什么不写死在前端

scope 清单在 `scopeRegistry`、端点在清单 `routes[]`、错误码在 `tokenError` / `oauthError` /
`userinfoError` 的分支里——各有一份唯一事实源。因此新增 `GET /api/oauth/docs`（`mode: http`，
公开）只输出这三张表，散文与版式留在模板里。`docs_test.go` 的守卫：

1. `docsEndpoints` 与清单 `routes[]` **同集**（双向比对）；
2. scope 行与 `scopeRegistry` 逐项一致（名称 + 中文描述）；
3. `docsErrorCodes` 里的每个码都能在**其它**源码文件里找到字面量（扫源码，排除 `docs.go`
   与测试文件自身），凭空编一个码会红；
4. `theme.json` 页面声明必须指向真实模板、`templates[]` 有精确声明；组件必须**只填表**，
   禁止再渲染任何页面骨架（`<main>` / `sf-card` / `sf-prose` / 岛标签都不许出现）；
   样式必须用 Core token、必须包含 960px 容器宽度，不得硬编码黑白。

### 运维注意

- 模板页正文进的是宿主 plugin shell（自动带 navbar/footer），模板里**不要**再写
  `<sf-navbar>` / `<sf-footer>`，否则渲染两份。
- `prepare-package.sh` 的两阶段 `digest --write` 期间，dev-auto 可能读到半截 manifest 并报
  `static preflight: invalid manifest: v3 aitools rule #N` 的瞬态 WARN——等构建结束后重启
  API（或等下一轮 promote）即可，`extension validate` 通过就是权威判定。
- 激活卡死时的干净恢复路径：`sforum extension disable <id>`（带 DATABASE_URL 的带外禁用，
  关闭卡住的 lifecycle 状态机）→ 重启 API → 管理端 enable（用 trust challenge 的
  confirmationToken）→ 需要时再 upgrade。

### 验证证据（2026-10-19 定稿）

- `cd backend && go test ./...` → 全绿（`docs_test.go` 四层守卫 + 既有 schema 对齐测试）。
- `./scripts/prepare-package.sh` → `extension validate` 报 `routes 17 / packageFiles 34 /
  components 2 / templates 2 / assets 2 / pages 2`，`extension test` 0 errors 0 warnings。
- 浏览器（`/docs/oauth2-provider`，宿主 plugin shell + 默认主题）：
  - 桌面 1633×810：`docs/images/docs-page-desktop-2026-10-19.png`（容器 960px）；
  - 端点表：`docs/images/docs-page-endpoints-2026-10-19.png`；
  - 桌面深色：`docs/images/docs-page-dark-2026-10-19.png`；
  - 窄屏 390×844：`docs/images/docs-page-narrow-2026-10-19.png`（端点表横向滑动）。
- 性能（PerformanceObserver layout-shift + 首帧探针）：
  - SSR 模板方案：正文首帧 **~245ms**，CLS **0.005**；
  - 改造前组件挂载方案：正文首帧 ~4.9s，CLS 0.09（记录为反例）。
- 实测：navbar/footer 各一份由宿主渲染，6 张表全部填满（scope 6 / 端点 6 + 内部 11 /
  错误码 7），示例域名替换为 `location.origin`，桌面与窄屏均无页面级横向滚动条；页面标题由
  宿主 SEO 渲染为「OAuth2 接入说明 | SForum」。

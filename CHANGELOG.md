# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与
[语义化版本](https://semver.org/lang/zh-CN/)。

## [1.6.0] - 2026-10-19

### Added

- **站内接入说明页 `/docs/oauth2-provider`**：此前插件没有任何站内文档，接入方只能去翻 GitHub
  上的 README。页面是**宿主公开页**（`theme.json` → `access: public`），外壳（navbar / footer /
  站点布局与主题）全部由 Core 渲染；**正文由宿主模板 `templates/oauth-docs.html` SSR 输出**，
  沿用 Core 法律页同构的卡片（`.sf-public-page` + `.sf-card`）与 `.sf-prose` 长文排版——
  首帧即有完整正文，无「空白卡片 → 整页文字」的布局跳动（实测正文首帧 ~245ms、CLS 0.005，
  改造前组件挂载方案为 ~4.9s / 0.09）。
- 页面内容覆盖「这是什么 / 创建应用 / 发起授权 / 换取令牌 / 读取用户资料 / 刷新撤销与内省 /
  权限范围 / 错误码 / 端点总表 / 限制与注意事项」，示例里的站点地址由 L2 组件在运行时替换成
  `location.origin`。
- 正文容器加宽到 **960px**（Core `--narrow` 默认 760px）：文档含 4 列表格与长代码行，
  760px 会把端点路径挤到换行。
- 新增数据端点 **`GET /api/oauth/docs`**（`mode: http`，`guard: core.guard.public`，
  `…route.docs.response@1`）：scope / 端点 / 错误码三张表分别由 `scopeRegistry`、
  `docsEndpoints`（与清单 `routes[]` 同集）、`docsErrorCodes` 生成。新增 `docs_test.go` 做四层
  守卫：数据与注册表/清单/错误分支一致、`theme.json` 页面声明指向真实模板与组件、组件只准填表
  不准渲染骨架、样式必须用 Core token 且不得硬编码黑白。
- 新增公开 L2 组件 `sforum.oauth2-provider.component.docs@9`
  （`frontend/public/docs.mjs` + `docs.css`，样式以 `assets[].scope` 绑定到该组件）：
  组件只负责取数填表与替换示例域名，`docs.css` 只补 `.sf-prose` 没有的表格排版、方法徽标与
  容器宽度（端点表 `min-width: 640px`，窄屏走横向滑动而不是把「说明」挤成一字一行）。
- 管理端「OAuth2 应用」页头新增「接入文档」入口（新标签页打开 `/docs/oauth2-provider`）。
- `handleRouteStream` 抽出 `streamResponse(open)`，确认页行为不变
  （`pluginv2.RouteStream` 字段未导出，抽出来才能被单测覆盖）。
- 包版本 `1.5.2` → `1.6.0`。

### Changed

- **`/settings/oauth-apps` 改为所有登录用户可访问**：页面此前声明
  `access: permission` + `permission: sforum.oauth2-provider.create_clients`，宿主对无该权限的
  登录用户直接 403，左栏入口也带同一权限而隐藏——而「撤销自己已授权的应用」本来只要求登录
  （`/api/oauth/authorizations` 与 `/revoke` 都是 self-scoped），结果普通用户没有任何撤销入口。
  现在 `theme.json` 改为 `access: login` 并去掉 permission，导航项去掉 permission
  （保留 `visibility: authenticated`），入口对所有登录用户可见。
- 前端按权限分层：`/api/oauth/clients` 返回 403 `oauth2.client_list_forbidden` 时不再弹
  「加载应用失败」，而是把 `canCreate` 置 false、显示「你没有创建应用的权限」空态，并在没有
  显式深链时默认切到「已授权」；`canCreate` 初值改为 false，避免无权限时残留可点的「新建应用」。
  后端路由契约未变。
- 文案对齐 GitHub 的 Applications / Authorized OAuth Apps：左栏与页面标题「OAuth2 应用」→
  「OAuth 应用」，同意页的撤销指引改为「设置 → OAuth 应用 → 已授权」。
- 新增静态回归测试 `TestPublicPageIsLoginAccessible`，锁住页面 `access=login` 与导航项无
  permission；组件契约 `component.apps@21` → `@22`（L2 制品内容变化，宿主升级校验要求同步提升）。
- CI 与 Release 的 `SFORUM_REF` 钉到宿主 `feat/extension-host-v2-surfaces` 分支：本版依赖
  `SelfEmailQueryRequest`、`attachments.extension.claim/release`、`admin.pages[].group` 与
  stream 路由登录重定向，这些宿主提交尚未合入 `main`。合入后改回 `main` 即可。

### Fixed

- **应用卡片底栏「更多操作」⋯ 按钮渲染成空方块**：宿主图标端点对 Iconify 别名只回
  `aliases[name].parent`，正文挂在规范名下（`lucide` 的 `more-horizontal` → `ellipsis`），
  而 `ensureIcons()` 只遍历 `payload.icons`，按请求名永远取不到 body，`iconElement()` 返回
  `null` 后按钮就只剩一块 27px 空白。现在沿别名链解析（最多 4 跳）并同时按请求名落缓存，
  ⋯ 按钮改引用规范名 `lucide:ellipsis`；新增 `applyMoreIcons()` 处理「卡片先于图标渲染」
  的晚到补丁，端点不可用时退化为 `⋯` 文本，按钮仍然可点。
- **复制成功的 ✓ 闪变从不出现**：`lucide:check` 不在 `ensureIcons([...])` 预载表里，
  `flashCopyDone()` 拿不到图标就提前 return。已补入预载表。
- 新增静态回归测试 `TestPageScriptPreloadsEveryIconReference`：`apps.mjs` 里每个
  `iconElement('…')` 字面量都必须出现在预载表中，防止同类漏载。
- 组件契约 `component.apps@20` → `@21`（L2 制品内容变化，宿主升级校验要求同步提升）。

## [1.5.0] - 2026-10-18

### Added

- **`email` 范围：授权时快照本人邮箱**。确认页会明确列出「读取你的邮箱地址与验证状态」；
  用户同意后，插件在签发授权码时用宿主的一次性 query 委派读取**一次**本人邮箱并落库，
  `userinfo` 只读这份快照（不会持续读取，也不会跨应用共享）。
  - 宿主面：capability `users.email.read` + `core.query.identity.self_email@1`
    （`sforum.core.self_email@1`，scope `route`，委派单次 ≤2 分钟）。
  - `backend/email_claim.go`：`captureEmailClaim` / `readOwnEmail`；两条签发路径（确认页允许、
    免确认直放）都覆盖；失败只让字段缺席，**不影响授权本身**。
  - `oauth_email_claims` 表（own_schema）：主键 `(user_id, client_id)`；撤销授权、删除应用、
    或新授权收窄到不含 `email` 时立即删除。
  - userinfo 新增可选字段 `email` / `emailVerified`（同进同出；无快照时整段缺席）。
  - **修复**：`hostCallContext` 现在会清掉宿主下发的 command/query/session 委派。此前只清
    `Actor`，声明 `users.email.read` 后 userinfo 读身份会把委派回传，宿主以
    `protocol v2 actor delegations are Host-to-plugin only` 拒绝（userinfo 503）。

### Changed

- 契约：`userinfo.response@3` → `@4`（新增两个可选字段，`required` 不变）；
  清单 `routes[].responseSchema` 与 `openapi/routes.yaml` 同步。
- 清单新增 capability `users.email.read`（高风险）：**启用/升级时会在影响评审里列出，
  需要运营确认**；未确认时宿主不签发委派，`email` 范围读取降级为字段缺席。
- `allowed_scopes` 默认值补上 `email`（`email avatar topic.count comment.count users.groups`）；
  已显式保存过该设置的站点不受影响，需要时在后台自行加入。
- 版本 `1.4.0` → `1.5.0`；升级仍需走「禁用 → 升级（授权当前精确制品 + 确认新能力）→ 启用」。

- **默认图标与 userinfo 兜底头像改为纯 GitHub 风格 identicon**：在 [1.3.0] 的混合形态基础上
  去掉 SFAvatar 元素——不再绘制首字母、不再用 8 色板；改为方型浅灰底（`#f0f0f0`）+
  5×5 镜像像素块（单色，色相由种子哈希决定，`S 60% / L 63%` 取自 GitHub 默认头像实测区间），
  布局、边距（`inset 8%`）与填充规则（nibble 偶校验）对齐 GitHub。
  userinfo 的兜底头像（`data:image/svg+xml`）同款、无文字。
- 图标描述符改为 `{kind, url?, pattern, background, color}`：删除 `initials` 与 `foreground`，
  新增 `color`（色块颜色）。四份 schema 再升一版：`clients.response@6` → `@7`、
  `client-write.response@4` → `@5`、`consent-summary.response@4` → `@5`、
  `authorizations.response@4` → `@5`；对应路由 `contractVersion` 同步 +1
  （`clients-list@5`、`clients-create@5`、`clients-update@5`、`clients-secret-rotate@5`、
  `authorizations-list@5`、`consent-summary@5`）；组件 `component.apps@11` → `@12`。
- `backend/avatar.go` 不再生成首字母 SVG：`avatarInitials` / `avatarTone` / 8 色板 /
  `xmlEscapeText` 全部删除，改为复用 `backend/identicon.go`；`backend/icon.go` 同样删掉
  `appInitials` 与渐变函数。
- 包版本 `1.4.0` → `1.5.0`；**升级仍需「禁用 → 升级（授权当前精确制品）→ 启用」**。

### Not included

- `bio` / `location` / `websiteUrl` 仍未纳入：公开数据，但宿主暂无对应稳定视图/查询。
- 未做 OIDC `id_token` / discovery；email 已具备，后续可在此基础上补。

## [1.4.0] - 2026-10-18

### Added

- **userinfo 补齐账号时间线与主页链接**（对齐 GitHub `/user`）：新增 `id`（数字，与 `sub`
  同值）、`createdAt`（注册时间）、`updatedAt`（资料更新时间）、`profileUrl`（站内公开主页
  `{site.url}/u/{username}`）。全部属于基础 `profile` 范围，不新增 scope、不改确认页结构。
- `backend/account.go`：通过宿主稳定查询 `sforum.core.safe_user.by_id` 读取
  `sforum_core_v1.safe_users` 的 `created_at`/`updated_at`（公开数据，actorless）。
  结果类型按宿主规则解析（BIGINT 是十进制字符串、timestamptz 是 UTC RFC3339Nano），
  并统一归一成 UTC RFC3339Nano。
- `backend/account_test.go`：钉住查询请求形状（查询 id / plan 版本 / 字段白名单 /
  `sforum.core.query.int64@1` 过滤器 / page limit=1 / **不带 actor**）、成功负载过 schema、
  以及五种降级路径（授权拒绝、无 active 行、无 Query 通道、传输错误、站点地址缺失）。

### Changed

- **清单 `database.grants` 增加 `core_views`**（`["own_schema","core_views","host_commands"]`），
  `database.contractVersion` → `sforum.oauth2-provider.database@2`。宿主只对声明了该授权的
  精确制品开放 `sforum.core.*` 稳定查询；**升级时影响评审需确认这项新授权**，否则查询返回
  `host.query_core_views_denied`（userinfo 只缺 `createdAt`/`updatedAt`，其余照常 200）。
- 契约升版：`userinfo.response@2` → `@3`（新增四个可选字段，`required` 不变；`id` 固定数字
  形态）。`openapi/routes.yaml` 与清单 `routes[].responseSchema` 同步。
- `profile` 范围的说明文案补上「注册时间」（`scopeRegistry` + 前台/管理端 `scopeDescProfile`）：
  确认页会如实披露多读的公开数据，scope 名不变因此老授权不受影响。
- 版本 `1.3.0` → `1.4.0`；升级仍需走「禁用 → 升级（授权当前精确制品）→ 启用」。

### Not included

- `email` / `email_verified`：宿主把 email 当敏感字段，只走 actor 门控面，而插件 → 宿主的
  调用必须 actorless；稳定视图/查询按策略也不得携带 email。补它需要宿主先开新面，方案记在
  `NOTES.md`「email 的三条路」。
- `bio` / `location` / `websiteUrl`：公开数据，但 `sforum_core_v1` 暂无对应视图/查询。

## [1.3.0] - 2026-10-18

### Changed

- **默认应用图标改为「SFAvatar 混合 identicon」**：无自定义图标（`generated`）时不再显示
  渐变圆角底 + 首字母，改为方型浅灰纯色底（`#f0f0f0`，无渐变）+ 5×5 镜像像素块（布局与
  偶校验填充参考 GitHub 的 identicon）+ 居中的 SFAvatar 首字母；像素块与首字母同色，颜色取自
  站点 SFAvatar 8 色板，块透明度 `0.4`。三处消费点（授权确认页、前台「我的应用 / 已授权」、
  管理端列表）与表单实时预览同步生效；公开 / 机密不再用 `hue-rotate` 区分（避免把色板旋出
  色板外）。
- **userinfo 兜底头像同步换装**：授予 `avatar` 但宿主没有头像 URL 时，生成的确定性 SVG 从
  「圆形 + 首字」改为 128×128 方形混合 identicon（首字母与 8 色板仍与站点 SFAvatar 同源）。
  确认页「当前账号」头像不变，继续复刻宿主 `SFAvatar`。
- 图标描述符新增必填 `pattern`（25 个 `0/1`，行优先、左右镜像）；`background` 从渐变串改为
  纯色串；`initials` 语义不变，`foreground` 现在固定是 SFAvatar 色板颜色。
- 契约升版（宿主 exact-artifact 准入要求）：`clients.response@5` → `@6`、
  `client-write.response@3` → `@4`、`consent-summary.response@3` → `@4`、
  `authorizations.response@3` → `@4`；对应路由 `contractVersion` 同步升版
  （`clients-list@4`、`clients-create@4`、`clients-update@4`、`clients-secret-rotate@4`、
  `authorizations-list@4`、`consent-summary@4`）；组件 `component.apps@9` → `@10`
  （apps.mjs 文案再改一版后又按宿主规则升到 `@11`）。
  **升级 / 启用需要重新走一次「禁用 → 升级（授权当前精确制品）→ 启用」。**

### Added

- `backend/identicon.go`：`stableHash` / `identiconPattern` / `identiconPath` 作为 Go 侧单一
  事实源；`backend/icon_test.go` 固化 25 格长度、左右镜像、确定性、空种子回退与 golden 值。
  前台 `apps.mjs`、管理端 `clients.mjs`、确认页内联脚本各带一份镜像，实测与 Go 对同一组种子
  输出逐字一致。

## [1.2.0] - 2026-10-18

### Added

- **应用图标支持直接上传图片**：前台 `/settings/oauth-apps` 与管理端「OAuth2 应用」的图标字段
  改为「上传为主、URL 为次」。浏览器把 PNG / JPEG / WebP（≤512 KiB）直接 POST 到 Core 的
  `/api/v1/attachments`，插件拿 `publicId` 调 Host Command
  `sforum.attachments.extension.claim@1` 把它登记成本扩展的公开资产，自己的库只存
  `publicId`（`oauth_clients.attachment_public_id`）——**不存字节、不存 URL、不自建上传路由**。
- 「或使用图片地址」入口保留（默认折叠），图标因此有三种来源，优先级
  `attachment`（上传）> `url`（填写地址）> `generated`（生成）。上传与 URL **互斥**：
  从「上传」切到「地址」或清空图标时，先调 `sforum.attachments.extension.release@1`
  撤掉旧附件引用，再清字段；漏掉 release 会留下永不回收的孤儿附件。
- 删除应用时 best-effort release（失败只记日志，不阻塞删除；附件本体等 Core 的孤儿回收，
  默认 30 天，本身就是误删宽限期）。

### Changed

- **`database.grants` 新增 `host_commands`**：这是 Host Command 的前提授权。
  因此本版本**升级/启用需要重新做一次 super_admin 信任确认**（exact-artifact 授权影响面变了），
  这是预期行为，不是 bug。
- `sforumVersion` 提到 `>=3.0.12-alpha.7`（首个包含上述三条附件命令的发布版本；Core 侧
  当前在主干，若首发版本号不同，以实际发布为准回填）。
- L2 组件内容变更（`frontend/public/apps.mjs` / `apps.css` 都改了），契约升版：
  `component.apps@7` → `@8`。不升版宿主会拒绝激活（component registry contract conflict）。
- 图标描述符的 `kind` 由 `custom` / `generated` 扩展为 `attachment` / `url` / `generated`；
  三处消费点（授权确认页、前台卡片、管理端列表）改为「有 `icon.url` 就叠 `<img>`」，
  generated 兜底不变。契约升版：`clients.response@4` → `@5`、
  `client-write.request@2` → `@3`、`client-write.response@2` → `@3`、
  `authorizations.response@2` → `@3`、`consent-summary.response@2` → `@3`，
  对应路由 `contractVersion` 同步升版。
- 保存 / 删除应用的请求现在带 `Idempotency-Key` 头（`crypto.randomUUID()`）：
  Host 只在它存在时才签发 Host Command 的 actor 委派，缺了它 claim / release 永远调不通。
- 图标类失败返回稳定错误码（`icon_upload_unsupported`、`icon_revision_conflict`、
  `icon_too_large`、`icon_forbidden`、`icon_reference_limit`、`icon_attachment_missing`、
  `icon_invalid`、`idempotency_key_required`），前端翻成可读文案；**旧宿主给出
  「升级 SForum 或改用图片地址」并自动展开 URL 输入，绝不静默退回 base64 内联**。
- 新建/编辑/删除路由的 `timeoutMs` 提到 15000，给 claim / release 的 Host Command 往返留余量。

- 前台 `/settings/oauth-apps` 的所有文字 alert 提示改为宿主 Nuxt UI Toast：新建/更新应用、
  重置密钥、删除应用、撤销授权的成功反馈，应用与已授权列表的加载失败，表单校验失败与
  请求失败，以及页面制品漂移诊断，统一走 `app.vue` 的 `<UApp :toaster>`（右上角浮层，
  success/info 4s、error 6s、制品漂移常驻到手动关闭）。页面顶部的 `.sfp-status` 文字条
  降级为「宿主 Toast 不可用」时的兜底通道，弹窗内的 `.sfp-form__error` 文字报错随之移除。
  L2 组件内容变更，契约升版：`component.apps@6` → `@7`。

- 权限范围扩展：新增 `avatar`、`topic.count`、`comment.count`、`users.groups` 四个可选 scope
  （连同必选的 `profile` 共五个）。数据面由宿主 core-safe 用户投影提供——宿主侧将
  avatar / 用户组徽章 / 公开内容计数纳入 `CoreSafeUserFields`（均为公开即视数据），
  插件 userinfo 按 token 授予的 scope 条件暴露对应字段（`route.userinfo.response` 契约升到 `@2`）。
  `allowed_scopes` 默认值更新为全部可选范围；manifest 设置描述同步。email 仍未纳入：
  它是非公开字段，需要宿主侧单独的授权设计，不做假实现。

- `profile` 升级为必选基础范围：`normalizeScopes` 无条件并入（authorize 与 clients 两条路径共享该不变量），
  配置解析保证 `allowed_scopes` 永远含 profile（管理员清单只决定额外可选范围）；存量应用在下次保存时自动补齐。
  前台与后台表单把 profile 渲染为「必选」锁定 chip，提交值恒含 profile。
- 新增 scope 目录：`/api/oauth/clients` 响应下发 `scopeCatalog`（名称 + 中英描述 + `alwaysOn` 标记，
  `route.clients.response` 契约升到 `@3`），前端 chip 描述改读目录、MESSAGES 文案仅作回退；
  新增 scope 时在 `backend/constants.go` 的 `scopeRegistry` 登记一行即可。
- 应用表单的「权限范围」从自由文本改为 chip 多选：选项来自站点 `allowed_scopes`
  （`/api/oauth/clients` 响应新增 `allowedScopes` 字段，`route.clients.response` 契约升到
  `@2`），每个 scope 带一句说明，并提供「全选 / 仅基础资料 / 清空」快捷项（清单 ≤1 时自动
  隐藏）；「一个不选 = 站点默认（创建）/ 保持现状（编辑）」的语义不变。清单外的历史
  scope 以锁定 chip 呈现并显式提示保存后移除，避免被静默丢弃或被服务端拒收。前台
  `settings/oauth-apps` 弹窗与后台管理端表单同步换新。
- 前台「OAuth2 应用」页改版（软卡片方向）：药丸分段标签页 + 统计带（应用数 / 授权数 /
  接入端点并入内容流，取消右栏）+ 软阴影圆角应用卡栅格（按名称取色的渐变类型头像、
  粉彩徽章、输入框式 client_id 字段、图标键值元信息、分隔线操作行）+ 虚线「新建应用」
  占位卡；颜色全部走宿主主题 token，深色与多主题自动跟随。挂载点契约不变
  （`sfp-rail-app-count` / `sfp-rail-auth-count` / `sfp-rail-help` 移入统计带）。
- 后台侧栏入口改为独立一级菜单：两个管理页声明 `admin.pages[].group`，宿主把同名页签合并成
  可展开的「OAuth2」分组（应用 / 设置两个入口），不再平铺进「扩展管理」，也不再依赖专业模式。
  需要宿主支持 `admin.pages[].group`（旧版宿主会因 `additionalProperties` 拒收清单）。
- 授权确认页（`/api/oauth/consent`）改版：窄屏为底部抽屉（抓手 + 粘底操作区 +
  `env(safe-area-inset-bottom)` 安全区），宽屏收成 430px 居中面板；权限默认折叠成
  「读取 N 项资料」一行摘要（附前两项名称），展开后逐条列出 scope key 与用途；应用图标取
  应用名首字，身份行显示 `displayName` / `@username` 并提供「切换账号」（回到同一授权请求）；
  「已失效」与「加载中」各有独立版式，失效时隐藏操作区并保留技术原因便于排查。配色跟随
  `prefers-color-scheme`，新增 `--accent-fg` 前景令牌，避免深色下图标、计数与链接糊在底色里。
  路由契约、`__REQUEST_TOKEN__` / `__RETURN_URL__` 注入、summary / decision 调用与 CSRF
  握手均未改动，仍是无外部依赖的单文件 HTML（`mode: stream`）。

- `frontend/public/*` 内容变更后把 L2 组件契约从 `sforum.oauth2-provider.component.apps@2`
  提升到 `@3`（宿主禁止「内容变了但契约版本不变」的贡献升级）。

- `/settings/oauth-apps` 加载闪屏治理：页头标题模板改用 `<p>` 占位（L2 CSS 到位前不再闪
  56px 巨字，挂载时升级为带 `data-slot="title"` 豁免的 h1 并回写 `document.title`）；
  「站点已关闭自助创建」提示改为挂载后按 `canCreate` 动态创建（静态块曾在每次加载时
  对已开启自助创建的站点闪一帧错误内容）；页头图标改为就位后就地补丁（不再重建按钮）；
  图标请求先于数据返回时保持模板骨架（不再闪「还没有应用」空态）。
  宿主侧配套（SForum `SFExtensionWidget`）：L2 loading 期间隐藏兜底提示（只在真正失败/
  隔离时出现），描述符瞬时失败按 1.5s/4s 退避自动重试至多 2 次（隔离计数与 3 次上限不变），
  空气重启/网络抖动不再卡骨架到手动刷新。
- `/settings/oauth-apps` 页面与 profile 页共用同一套账号设置三栏 chrome：左栏改为
  宿主新增的 `sf-account-settings-sidebar` 侧栏岛（模板路由 `sforum.account` 侧全部由宿主组件渲染，
  不再有仿制导航），窄屏与 profile 共用同一抽屉。
- 插件页 chrome 收敛到宿主统一管线：页脚改为宿主新增的 `sf-content-column-footer`
  内容列页脚岛（与 settings 页同组件同 scope，透明背景 + 内容列出血、随内容滚动，
  不再贴视口底）；导航与页脚不再依赖主题静态外壳模板；
  `document.title` 由正文首个 h1 推导，不再显示 page id。
- 中间栏改为分段标签页（我的应用 / 已授权，`role=tablist` 结构，支持 `#apps` / `#authorized`
  深链与左右键导航），计数徽标随实际条数渲染。
- 页头字号改为跟随宿主 `--sf-public-page-title-size`（22px），不再使用旧制品的 56px。

### Fixed

- 授权确认页的「切换账号」是死链，点下去落到一个错误链接：确认页地址只带 `?request=`，
  `return` 参数从未被赋值，按钮回退成裸的 `/api/oauth/authorize`；点击时仍处于登录态，
  宿主 `guest` 中间件立刻回跳，最终得到
  `{"error":"invalid_request","errorDescription":"client_id is required"}`。现在
  `/api/oauth/authorize` 跳确认页时把原始授权请求作为 `return` 带过去（只接受本站授权
  端点、且必须带查询串的相对路径，防开放重定向），确认页把 `request` / `return` 改为
  JSON 字面量注入（HTML 转义会把 `&` 变成 `&amp;`，`<script>` 不解析实体），「切换账号」
  改为真动作：`POST /api/v1/auth/logout` 退出会话后整页跳到
  `/login?redirect=<原始授权请求>`，登录完成回到同一请求（`state` / PKCE / scope 全保留）。
  配套宿主约束：认证回跳目标是 `/api/` 或 `/plugin/` 时必须整页导航，否则会被
  `[...sfRegistryPage]` catch-all 接住（“Failed to load extension page”）——已在 SForum
  `useAuthReturnNavigation` 修正（见 NOTES.md 第 10 条）。
- 前台 `/settings/oauth-apps` 的「新建 / 编辑应用」弹窗把前三个字段渲染了两遍
  （应用名称、应用说明、应用图标 URL 各出现两次，`sfp-name` / `sfp-description` /
  `sfp-logo` 三个 id 重复，label 只对第一个输入框生效）。原因是弹窗表单块在
  `apps.mjs` 里被复制成了「旧字段块 + 新字段块」两份；现在只保留带必填/可选标签的
  一份，并补 `TestPageScriptModalFieldsAreUnique` 防止再次复制。由于 L2 组件内容变更，
  宿主 exact-artifact 准入要求契约升版：`component.apps@5` → `@6`（不升版会被
  「contribution … changed without a contract version」拒绝升级）。
- 前台 `/settings/oauth-apps` 完全加载后标签页标题退化为裸的「OAuth2 应用」，
  丢失 Core 的 `{pageTitle} | {seoSiteName}` 后缀：前端在把占位 `<p>` 升级成 h1 时
  手写了 `document.title`，直改 DOM 绕过宿主 unhead，与宿主 SEO 管道（贡献声明
  label → seoResolver 模板）打时序竞争且赢面下永不恢复。标签页标题全部交给宿主渲染，
  前端不再写 `document.title`；h1 升级仅保留 CSS 巨字号豁免用途。
- 授权确认页标签页标题硬编码 `授权确认 · {site.name}`：分隔符与站点
  `seo.page.title_separator` 无关，站名没走 `seo.site.name` 继承链。改为解析
  `seo.page.title_template`（仅 `pageTitle` / `seoSiteName` 两个变量，与宿主
  `seo_options` 的模板白名单一致），`seo.site.name` 未配置或开启继承时退回 `site.name`。
- 授权确认页硬编码绿色主题：页面主色与浅色背景改为运行时从宿主公开配置
  （`/api/v1/web-options`）与当前会话（`/api/v1/auth/session`）读取，优先级为
  用户外观偏好 > 站点外观设置 > 推荐默认值。预设主题与 `custom:#rrggbb` 混合规则、
  12 种浅色背景都是宿主 `apps/web` 外观契约的镜像；明暗模式跟随站点
  `nuxt-color-mode`（system/light/dark）。注入前保持中性灰，不再闪任何站点之外的强调色。
- 确认页 topbar 显示后台设置的站点名与站点 icon（`site.favicon_url` → `site.logo_url`
  → Core 默认品牌），点击后新标签页打开站点首页；此前是硬编码的「SForum 身份服务」。
- 确认页账号头像与站点 `SFAvatar` 不一致（灰底 `IN` vs `I` + 主题色板）：首字规则与
  稳定哈希色板复刻宿主实现，会话上传/Gravatar 头像在本页同样生效。
- 点击「拒绝」会在页面状态条渲染红色原始载荷（如 `true`）：决策处理对非字符串
  载荷做类型保护，拒绝成功直接回跳第三方 `?error=access_denied`（与 GitHub 一致），
  拒绝失败显示中性提示；仅允许失败使用红色错误样式。
- `userinfo` 在授予 `users.groups` 时整条路由 502：`groups` 是 `[]string`，而响应文档经
  `structpb.NewStruct` 编码，protobuf 的 Value 只接受 `[]interface{}`。改为
  `groupNamesToAny` 转换（空列表仍编码成 `[]`）。宿主侧同款问题见 `identity_encode_failed`。
- 授权确认页只列出 1 项权限（后台给应用加了 `avatar` 也不生效）：`/api/oauth/authorize`
  只看请求里的 `scope`，应用登记的范围从未参与授权流程。新增 `resolveAuthorizeScopes`，
  三层约束落地——请求不带 `scope` 时回落到应用登记范围（RFC 6749 §3.3 的服务端默认值）、
  请求超出应用登记范围按 `invalid_scope` 拒绝、站点 `allowed_scopes` 仍是硬上限。
  后台「权限范围」从此是有效约束，不再是无人读取的配置。
- 确认页权限文案只认 `profile`，其余 scope 直接显示裸键名（`avatar`）。`scopeRegistry`
  新增 `LabelZh` 并接管 `scopeLabel`，五个范围都有中文说明；确认页补上
  `topic.count` / `comment.count` / `users.groups` 的图标。
- 已知 client 之后的 OAuth 错误（`invalid_scope` / `unsupported_response_type` /
  `code_challenge` 非法）没有回跳第三方，而是返回裸 JSON：`verifyAuthorizeRequest`
  在错误分支里丢掉了已解析的 client。现在返回 client，错误按 RFC 6749 §4.1.2.1 回跳到
  已登记的回调地址。
- `demo/server.js` 把 `scope` 写死成 `profile`，改为读 `SCOPES` 环境变量；留空则不带
  `scope`，由 provider 回落到应用登记范围。

- **授予 `avatar` 范围后 userinfo 仍拿不到头像**：`handleUserinfo` 只在宿主返回的头像 URL
  非空时才写 `avatar`，而站点默认 `avatar.default_provider=initials` 时宿主投影的 `avatar`
  就是空串（initials 由前端 SFAvatar 现场渲染，没有 URL），于是默认安装下所有账号的
  `avatar` 字段都被静默丢弃，第三方无法区分「没授权」和「授权了但没有图」。现在授予即下发：
  宿主有 URL（上传 / Gravatar / 静态默认图）原样使用，没有 URL 时按显示名生成确定性的
  `data:image/svg+xml;base64,…` 首字头像（首字规则与 8 色板复刻站点 SFAvatar，同一用户永远
  同一张图）。**返回的 `avatar` 一定是可访问的绝对地址**：上传头像的站点相对路径会按运行时
  `site.url`（公开选项接口）→ 进程 `APP_URL` 补全，解析结果缓存 5 分钟；两者都拿不到时才
  保留相对路径，不猜测域名。`userinfo.response` 形状未变，契约版本不变；同时修正
  README / AGENTS 里「avatar 属于扩展字段、v1 不请求」的过时说明。
- **`client_id` 不再是裸字符串**：管理端应用表格与前台应用卡片都在值前面加了 `客户端 ID` 标签和
  「可公开」标记，悬停可读到「这是公开标识，要保密的是 client_secret」。此前这一串标识没有任何
  文字说明，容易被当成需要分发的公钥或私钥。
- **一次性凭据弹窗说清楚谁公开谁保密**：弹窗改名为「请立即保存客户端密钥（client_secret）」，
  两行分别标注 `客户端 ID · 可公开` 与 `客户端密钥 · 保密`，各附一句用途说明，并补一句
  「两者不是公私钥对：客户端 ID 相当于用户名，客户端密钥相当于密码，没有需要分发的公钥」。
- **修复 `/settings/oauth-apps` 弹窗完全不可见**：隐藏状态只切换了 `sfp-hidden` 类，而模板仍保留
  原生 `hidden` 属性，`[hidden]` 的 `display:none` 压过 `.sfp-modal` 的 `display:flex`，导致
  「新建应用 / 重置密钥 / 删除」弹窗永远不会出现（POST 已经发出，界面却毫无反馈）。
  现在模板初始状态同时带 `sfp-hidden`，`setHidden` 同步原生 `hidden` 属性与 `aria-hidden`，
  两种隐藏机制始终一致。
- **一次性密钥不再把「复制」按钮挤到下一行**：值框 `flex: 1 1 0` + 按钮 `flex: none`，
  43 字符的 `client_secret` 在值框内换行，复制按钮留在同一行。

- 升级宿主校验失败（`component registry contract conflicts with the active snapshot`）：
  页面重做后组件贡献 `sforum.oauth2-provider.component.apps` 的 L2 内容变化，契约版本需同步
  递增至 `@2`；同版本号升级必须走「禁用 → 升级/启用」流程。
- 页面挂载点校验：缺少模板挂载点时现在显示可见错误，运行时统一用 `.sfp-hidden` 类切换面板，
  并补写 `data-slot="title"` 以适配宿主全局标题样式约束。

[1.0.0]: https://github.com/keeptrue/sforum-oauth2-provider/releases/tag/v1.0.0

## [1.1.1] - 2026-10-18

### Changed

- 自定义 logo 改为白实底 + 发丝描边呈现（授权确认页 / 前台卡片 / 管理端列表三处一致）：
  透明 PNG 不再透出生成底色与首字母，观感与 Slack/Linear 的应用图标一致；图片加载中或
  失败时才露出渐变兜底层。前台组件契约升到 `component.apps@5`。

## [1.1.0] - 2026-10-18

### Added

- 应用图标（logo）与统一 icon 规范：应用可设置 `logoUrl`（https；本地开发放行
  loopback http；无 fragment，≤2048 字符），未设置时由后端按统一算法生成现代风格
  图标——种子取 `client_id` 哈希出色相的双色渐变底 + 应用名首字母，同名同色、不同
  应用自然错开。渲染描述符 `icon`（kind/url/initials/background/foreground）由后端
  `backend/icon.go` 单点计算，随 `/clients`、`/authorizations`、确认页 summary 下发，
  授权确认页、前台卡片、管理端列表三处共用同一描述符渲染，不再各画各的；logo 图片
  加载失败自动回退生成图标，永不空白。契约升版：`clients.response@4`、
  `client-write.request/response@2`、`consent-summary.response@2`、
  `authorizations.response@2` 及对应路由 contractVersion；前台组件 `component.apps@4`。
  `oauth_clients` 新增 `logo_url` 列（`ADD COLUMN IF NOT EXISTS` 幂等迁移）。

## [1.0.0] - 2026-10-15

### Added

- OAuth2 授权服务器：授权码 + PKCE（S256 / plain）、刷新令牌轮换、令牌撤销（RFC 7009）、
  令牌内省（RFC 7662）、userinfo。
- 授权确认页（`mode: stream` 自包含 HTML）与 `require_consent` 策略；已授权应用列表与撤销。
- 应用管理：管理员管理全部应用（`sforum.oauth2-provider.manage_clients`），用户可自助创建、
  编辑、重置密钥、删除；一次性明文密钥只在创建/重置响应中出现。
- 管理端面板（Admin Micro-frontend API v1，免构建 ESM）：列表卡片 + 新建/编辑模态、
  一次性密钥视图、危险操作二次确认、窄屏堆叠、暗色跟随宿主主题。
- 存储：`own_schema` 租约（`oauth_clients` / `oauth_requests` / `oauth_tokens` / `oauth_consents`），
  密钥与令牌只存 SHA-256；启用幂等建表，停用/卸载保留数据。
- 设置项：令牌有效期、自助创建开关、是否必须确认授权、允许的权限范围，均可一键恢复默认值。
- 契约测试：与宿主相同的 JSON Schema 校验每个 handler 的真实响应，防止响应契约漂移。

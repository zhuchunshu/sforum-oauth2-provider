# SForum OAuth2 Provider

[![ci](https://github.com/zhuchunshu/sforum-oauth2-provider/actions/workflows/ci.yml/badge.svg)](https://github.com/zhuchunshu/sforum-oauth2-provider/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/zhuchunshu/sforum-oauth2-provider)](https://github.com/zhuchunshu/sforum-oauth2-provider/releases)

让 **SForum 作为 OAuth2 授权服务器**（类似 GitHub OAuth Apps）：第三方应用通过授权码 + PKCE
拿到访问令牌，再调用 userinfo 读取用户资料。账号、密码与会话始终由 SForum 持有，插件不接触用户凭据。

- 形态：SForum 扩展包（Manifest V3 + Protocol V2 后端），`own_schema` 数据库租约，无第三方 OAuth2 依赖。
- 功能：授权码 + PKCE、刷新令牌轮换、令牌撤销（RFC 7009）、令牌内省（RFC 7662）、
  应用管理（管理员管全部 + 用户自助）、授权确认页、已授权应用与撤销。
- 安全：密钥与令牌只存 SHA-256 哈希；回调地址精确匹配；授权码一次性；常量时间比较。

| 授权确认页 | 管理端应用列表 |
| --- | --- |
| ![授权确认页](docs/images/consent-page.png) | ![管理端应用列表](docs/images/admin-clients.png) |

## 安装

1. 从 [Releases](https://github.com/zhuchunshu/sforum-oauth2-provider/releases) 下载与服务器平台匹配的压缩包
   （`linux_amd64` / `linux_arm64` / `darwin_arm64` / `darwin_amd64`），校验 `sha256sum -c SHA256SUMS`。
2. 装到 SForum（两种方式任选）：
   - **上传**：后台「插件管理 → 安装插件」选择 zip；
   - **外部源**：解压到某个外部源根的 `plugins/sforum-oauth2-provider/`，然后重启 API；
     或者直接把本仓库路径加进 `EXTERNAL_EXTENSION_ROOTS`（本仓库根就是源根）。
3. 后台启用 **OAuth2 Provider**，按宿主提示完成一次 exact-artifact `super_admin` 信任确认
   （`授权当前精确制品` → `确认并启用`）。
4. 需要“管理全部应用”的角色，到「角色权限」授予 `sforum.oauth2-provider.manage_clients`。

包内容变化会生成新的候选制品，需要重新确认信任；**版本号不变只改内容时**用「禁用 → 升级 → 启用」
切换（`重启` 不会切换制品）。

## 端点

| 端点 | 方法 | Guard | 说明 |
| --- | --- | --- | --- |
| `/api/oauth/authorize` | GET | public | 授权入口。未登录跳 `/login?redirect=…`；需确认跳确认页；已授权直接回跳并带 `code` |
| `/api/oauth/consent` | GET | login | 授权确认页（`mode: stream`，自包含 HTML） |
| `/api/oauth/consent/summary` | GET | login | 确认页数据：应用名、请求的权限、当前用户 |
| `/api/oauth/consent/decision` | POST | login | 允许 / 拒绝，返回回跳地址 |
| `/api/oauth/token` | POST | public | 授权码兑换、刷新令牌轮换 |
| `/api/oauth/userinfo` | GET | public | 读取用户资料（`access_token` 参数） |
| `/api/oauth/revoke` | POST | public | RFC 7009 撤销令牌 |
| `/api/oauth/introspect` | POST | public | RFC 7662 令牌内省 |
| `/api/oauth/csrf` | GET | public | 非浏览器客户端的 CSRF 握手端点 |
| `/api/oauth/clients` | GET | login | 应用列表（管理员看全部，用户看自己的） |
| `/api/oauth/clients/create` | POST | login | 创建应用（返回一次性明文密钥） |
| `/api/oauth/clients/update` | POST | login | 更新名称、说明、回调地址、权限 |
| `/api/oauth/clients/rotate-secret` | POST | login | 重置密钥（返回一次性明文密钥） |
| `/api/oauth/clients/delete` | POST | login | 删除应用（同时清理令牌与授权） |
| `/api/oauth/authorizations` | GET | login | 我已授权的应用 |
| `/api/oauth/authorizations/revoke` | POST | login | 撤销我对某应用的授权并作废其令牌 |

权限键 `sforum.oauth2-provider.manage_clients` 由插件声明、由宿主授权；插件的安装/启用代码不会
自行授予任何角色。

## 第三方应用接入

### 1. 建应用

管理员或用户创建应用，得到 `client_id` 与一次性 `client_secret`（公开客户端无密钥，必须用 PKCE）。
回调地址必须是 `https`，本地开发允许 `http://127.0.0.1` / `http://localhost`。

### 2. 浏览器授权

```
https://站点域名/api/oauth/authorize
  ?client_id=sfo_client_xxx
  &redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback
  &response_type=code
  &scope=profile
  &state=随机串
  &code_challenge=BASE64URL_SHA256(code_verifier)   # 公开客户端必填
  &code_challenge_method=S256
```

### 3. 换取令牌

```bash
# 非浏览器客户端先做 CSRF 握手：GET 会种下 csrf_ cookie（HttpOnly=false）
curl -c jar.txt https://站点域名/api/oauth/csrf

CSRF=$(awk '$6=="csrf_" {print $7}' jar.txt)
curl -b jar.txt -X POST https://站点域名/api/oauth/token \
  -H "Content-Type: application/json" \
  -H "X-Csrf-Token: $CSRF" \
  -H "Origin: https://站点域名" \
  -d '{
    "grant_type": "authorization_code",
    "client_id": "sfo_client_xxx",
    "client_secret": "sfos_xxx",
    "code": "sfoc_xxx",
    "redirect_uri": "https://app.example.com/callback",
    "code_verifier": "原始 verifier"
  }'
```

响应：`{ "access_token": "sfoa_…", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "sfor_…", "scope": "profile" }`。
刷新用 `grant_type=refresh_token`；刷新令牌每次使用都会轮换，旧的立即失效。

### 4. 读取用户资料

```bash
curl "https://站点域名/api/oauth/userinfo?access_token=sfoa_xxx"
# {"sub":"7","username":"alice","displayName":"Alice","status":"active","scope":"profile","clientId":"sfo_client_xxx"}
```

## Host 约束（实测，决定了本插件的实现方式）

1. **访问令牌用查询参数交付**：插件路由默认以 `filtered` authority 运行，会剥离 `Cookie` 与
   `Authorization`；`core.guard.raw_request` 在当前宿主版本关闭，因此 `userinfo` 只能用
   `access_token` 查询参数（RFC 6750 §2.3 允许的形式）认证。
2. **所有插件路由都受宿主 CSRF 中间件保护**：服务端到服务端的 `POST /api/oauth/token` 必须先
   `GET /api/oauth/csrf` 拿 `csrf_` cookie，再带 `X-Csrf-Token` 与 `Origin`。宿主只豁免
   `/api/v1/webhooks/inbound/*` 与站点 PAT（`Bearer sft_…`）。
3. **wire 层 schema 引用必须是版本化契约 `id@N`**：包内路径（`schemas/x.json`）能过清单校验，
   但分发时报 `protocol v2 schema reference … is invalid`。所以路由的 `requestSchema`/`responseSchema`
   统一用 `sforum.oauth2-provider.route.*@1`，并随包提供 `openapi/routes.yaml` 为每条路由声明
   operation；每条 operation 必须有一个带 schema 的 **2xx** 响应，再加 `default` 让 302/4xx/5xx
   也能命中回退。
4. **HTTP 响应必须自带 `Content-Type: application/json`**（恰好一个），否则分发时
   `extensions.route_schema_invalid`。`action: add` 的路由无论 `mode`（含 `stream`）都必须声明
   `responseSchema`，否则路由注册表准入被拒。
5. **插件 → 宿主的调用必须 actorless**：宿主拒绝携带 Actor 的插件请求
   （`protocol v2 actor context is not host-attested`），因此读设置与用户资料时必须清空
   `RequestContext.Actor`（core 安全字段不需要 actor）。
6. **授权确认页由 `mode: stream` 承载**：宿主对 `mode: http` 的响应强制 JSON 校验，公共资产端点
   也不服务 `.html`；`stream` 模式的正文是不透明数据块，preflight 可声明 `Content-Type: text/html`。
7. **PKCE S256 用 base64url**（RFC 7636 §4.2，无填充）；单元测试同时断言十六进制 challenge 被拒绝。
8. **userinfo 只返回宿主核心安全字段**（`id`/`username`/`displayName`/`status`）：邮箱、头像属于
   扩展用户字段，需要 actor 权限，v1 不请求。

## 设置项（后台 → OAuth2 设置）

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `sforum.oauth2-provider.access_token_ttl` | 3600 | 访问令牌有效期（秒） |
| `sforum.oauth2-provider.refresh_token_ttl` | 2592000 | 刷新令牌有效期（秒） |
| `sforum.oauth2-provider.authorization_code_ttl` | 600 | 授权码与确认请求有效期（秒） |
| `sforum.oauth2-provider.allow_user_clients` | true | 是否允许普通用户自助创建应用 |
| `sforum.oauth2-provider.require_consent` | true | 首次授权是否必须确认；关闭后仅在新增权限范围时确认 |
| `sforum.oauth2-provider.allowed_scopes` | `profile` | 允许的权限范围（空格分隔） |

设置读取失败时回落到上表默认值，因此缺少设置不会让端点不可用。

## 管理端面板

`frontend/admin/dist/clients.mjs` + `clients.css` 是免构建的手写 ESM 组件（Admin Micro-frontend API v1：
`apiVersion = 1` + `mount(target, bridge)`），挂在 `/control-panel/extensions/sforum.oauth2-provider/pages/clients`。

- 列表卡片 + 三种模态视图：新建/编辑表单、一次性密钥、危险确认（删除、重置密钥）。
- 就地校验与后端规则一致：名称必填、至少一个回调地址、地址必须 `https` 或本地回环且不带 fragment。
- 主题沿用宿主管理端变量（`--bg-admin-card`、`--bg-admin-app`、`--border-admin`、`--text-admin-main`、
  `--sf-accent*`），暗色由祖先 `.dark` 类驱动。
- ≤900px 转堆叠卡片，模态变底部抽屉。插件微前端拿不到 iconify 运行时（`i-tabler-*` 类不生效），
  因此按钮统一用文字，不使用 emoji 或自绘 SVG。

| 新建应用（模态） | 窄屏堆叠 |
| --- | --- |
| ![新建应用](docs/images/admin-create-modal.png) | ![窄屏](docs/images/admin-clients-mobile.png) |

## 目录布局

仓库根就是 SForum 的**外部扩展源根**，扩展包本体放在 `plugins/` 下（框架约定的集合布局）：

```text
sforum-oauth2-provider/
├── plugins/sforum-oauth2-provider/   ← 扩展包（sforum.extension.json、schemas、openapi、frontend、backend）
├── scripts/prepare-package.sh         ← 构建本机二进制 + 刷新摘要 + 宿主校验
├── docs/images/                       ← README 截图
└── .github/workflows/                 ← ci.yml / release.yml
```

因此既可以从 Release 装 zip，也可以把这个仓库直接挂成外部源（无需拷贝、无需软链）：

```bash
# .env
EXTERNAL_EXTENSION_ROOTS=/已有源,…/sforum-oauth2-provider
```

注意：宿主会拒绝重复扩展 id（同一 id 出现在两个源会冲突），也会拒绝包内的符号链接
（`extension package: symbolic links are not allowed`），所以不要把仓库软链到别处、也不要保留第二份副本。

## 本地开发

后端是嵌套 Go module，通过本地检出解析宿主 SDK（`apps/api` 尚未发布为 Go module），
约定两个仓库同级：

```text
Code/github/
├── SForum                        ← 框架检出
└── sforum-oauth2-provider        ← 本仓库
```

```bash
# 后端：编译 + 单测
cd plugins/sforum-oauth2-provider/backend && go vet ./... && go test ./...

# 构建可执行文件、刷新摘要、跑宿主校验（需要同级 SForum 检出；SFORUM_DIR=… 可覆盖）
./scripts/prepare-package.sh
```

改了源码（含前端 `.mjs`/`.css` 与 `schemas/*.json`）后必须重跑 `scripts/prepare-package.sh`：
宿主以 exact-artifact 校验哈希，摘要不匹配的包会被拒绝启用。若 SForum 已把本仓库挂成外部源，
重启 API 后会把它暂存为候选制品，按「禁用 → 升级 → 启用」切过去即可。

## 发布

推 `v*` 标签即触发 [release 工作流](.github/workflows/release.yml)：

1. `verify`：后端 vet/test + 宿主 `extension validate` / `extension test`，并校验标签与清单版本一致。
2. `build`：矩阵交叉编译 linux/amd64、linux/arm64、darwin/arm64、darwin/amd64；每个平台各自
   写入清单摘要后由宿主 CLI 打成 `sforum-oauth2-provider_<version>_<os>_<arch>.zip`（附 SBOM）。
3. `publish`：汇总产物、生成 `SHA256SUMS`、创建 Release 并上传。

发布前请把 `sforum.extension.json` 的 `version` 提到与标签一致（例如 `1.0.1` ↔ `v1.0.1`）。
Windows 未列入矩阵：宿主按清单 `backend/entry` 直接执行、不做 `.exe` 归一化，未经实机验证前不发布。

## 数据与生命周期

`own_schema`（`sforum_oauth2_provider`）下的表：`oauth_clients`、`oauth_requests`、`oauth_tokens`、
`oauth_consents`、`schema_meta`。`enable`/`install` 幂等建表；`disable`/`uninstall` 保留数据
（`retention: retain/export`）。没有数据库租约时回退进程内存储（仅供本地演示，重启即丢）。

## 许可

MIT，见 [LICENSE](LICENSE)。开发期间的宿主约束发现与决策记录见 [NOTES.md](NOTES.md)。

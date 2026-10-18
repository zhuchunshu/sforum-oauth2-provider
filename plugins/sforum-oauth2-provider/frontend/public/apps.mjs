export const apiVersion = 1

// 前台自助应用面板（Public L2 组件，无构建步骤的纯 ESM）。
//
// 视觉契约：三栏 chrome 由宿主渲染（左栏是宿主岛 sf-account-settings-sidebar，
// 与 /settings/profile 同构），本组件只负责中间栏的分段标签页与两组卡片栅格 + 模态。
// 列表采用可选卡栅格（对齐 /settings/appearance 的主题选择卡语言）：
// 墨色/强调色类型头像 + 徽章 + 等宽 client_id 芯片 + 键值元信息 + 底部操作行。
// 只写宿主没有的少数几块；卡片/按钮/徽章/空态/骨架一律用宿主全局 CSS 与组件类。
// 图标：插件页面没有 iconify 运行时，改从宿主自身的 /api/_nuxt_icon 端点取原始 SVG
//      内联，与宿主渲染出的图标逐像素一致。
//
// 模板约束（宿主模板净化器，写错会在包预检期就被拒）：
// - 模板上的 `hidden`、`data-*`、`aria-controls`、`tabindex` 会被净化器剥掉，
//   因此初始隐藏状态用 `sfp-hidden` 类表达，其余 aria/tabindex 在挂载时补齐。
// - 模板里的岛只能声明绑定表里的属性（左栏岛只有 active），且必须非空。
//
// 其它约束：
// - 只调用插件自身的 /api/oauth 路由（同源 fetch）；unsafe 方法带 X-Csrf-Token，
//   cookie 缺失时先打 /api/oauth/csrf 握手。
// - 首屏沿用模板里的宿主 .sf-skeleton 兜底，挂载后换成真实行，几何对齐避免位移。

const API = '/api/oauth'
// 图标上传直连 Core 的附件接口：字节交给宿主的附件体系，插件只拿 publicId 去 claim。
const CORE_API = '/api/v1'
// 前端前置校验只做体验（权威判定在 Host）：PNG / JPEG / WebP，≤512 KiB。
const ICON_MAX_BYTES = 512 * 1024
const ICON_TYPES = ['image/png', 'image/jpeg', 'image/webp']

const ICON_ENDPOINT = '/api/_nuxt_icon'
const iconCache = new Map()

// 与 templates/oauth-apps.html 的兜底骨架保持逐字一致：挂载瞬间不产生位移。
const APPS_SKELETON = `
  <div class="sf-skeleton sfp-skel-card">
    <div class="sfp-skel-card__cover"></div>
    <div class="sf-skeleton__content sfp-skel-card__body">
      <span class="sf-skeleton__line"></span>
      <span class="sf-skeleton__line sfp-skeleton-line--short"></span>
      <span class="sf-skeleton__line sfp-skeleton-line--mid"></span>
    </div>
  </div>
  <div class="sf-skeleton sfp-skel-card">
    <div class="sfp-skel-card__cover"></div>
    <div class="sf-skeleton__content sfp-skel-card__body">
      <span class="sf-skeleton__line"></span>
      <span class="sf-skeleton__line sfp-skeleton-line--short"></span>
      <span class="sf-skeleton__line sfp-skeleton-line--mid"></span>
    </div>
  </div>
`

// 挂载点：模板与脚本是一份精确制品，两边的选择器必须同时成立。
// 少一个就说明站点跑的模板与这份脚本版本不一致——显式报错，不要静默空转。
const MOUNT_POINTS = {
  appsBody: '.sfp-apps-body',
  authBody: '.sfp-auth-body',
  status: '.sfp-status-mount',
  appsDesc: '.sfp-apps-desc',
  authDesc: '.sfp-auth-desc',
  countApps: '.sfp-count-apps',
  countAuths: '.sfp-count-auths',
  tabApps: '#sfp-tab-apps',
  tabAuths: '#sfp-tab-auths',
  tabAppsLabel: '.sfp-tab-apps-label',
  tabAuthsLabel: '.sfp-tab-auths-label',
  panelApps: '#sfp-panel-apps',
  panelAuths: '#sfp-panel-auths'
}

const MESSAGES = {
  'zh-CN': {
    appsTab: '我的应用',
    authTab: '已授权',
    tabsLabel: 'OAuth 应用视图',
    appsHint: (count, source) => `共 ${count} 个应用 · 存储：${source}`,
    refresh: '刷新',
    create: '新建应用',
    edit: '编辑',
    rotate: '重置密钥',
    remove: '删除',
    copy: '复制',
    copied: '已复制',
    copiedToast: '已复制到剪贴板',
    copyFailed: '复制失败：浏览器拒绝了剪贴板权限，请手动复制',
    tagRequired: '必填',
    tagOptional: '可选',
    formSub: '应用只能申请你在下方勾选的权限范围，用户授权时会再向你确认一次。',
    never: '从未使用',
    usedAt: (value) => `最近使用 ${value}`,
    emptyTitle: '还没有应用',
    emptyHint: '创建第一个应用，拿到 client_id 与密钥后即可接入 OAuth2 授权。',
    appsForbiddenTitle: '你没有创建应用的权限',
    appsForbiddenHint: '你仍然可以在这里查看并撤销已授权访问你账号的应用。需要创建 OAuth 应用请联系站点管理员。',
    appsForbiddenDesc: '你的账号没有创建 OAuth 应用的权限',
    createTitle: '新建应用',
    editTitle: '编辑应用',
    fieldName: '应用名称',
    fieldNameHint: '展示给授权用户看的名字，例如「我的博客」。',
    fieldDescription: '应用说明',
    fieldDescriptionHint: '可选，一句话说明用途。',
    noDescription: '未填写说明',
    fieldIcon: '应用图标',
    fieldIconHint: '可选，上传 PNG / JPEG / WebP（不超过 512 KiB）；留空则自动生成 GitHub 风格 identicon 图标（浅灰方底 + 镜像像素块）。',
    iconUpload: '上传图片',
    iconRemove: '移除图标',
    iconUseURL: '或使用图片地址',
    iconUploading: '正在上传…',
    iconUploaded: '已上传，保存后生效',
    iconUploadedSaved: '已上传',
    iconUploadFailed: '图片上传失败，请重试',
    iconTypeInvalid: '只支持 PNG / JPEG / WebP 图片',
    iconTooLarge: (max) => `图片不能超过 ${max}`,
    iconUnsupported: '当前宿主版本不支持图标上传，请升级 SForum 或改用图片地址',
    iconRetry: '页面状态已过期，请刷新后重试',
    iconRevisionConflict: '图片已被修改，请重新上传',
    iconForbidden: '这张图片不属于你，或已被其他功能使用',
    iconReferenceLimit: '本扩展的图片引用已达上限，请先移除其他应用的图标',
    iconAttachmentMissing: '图片已不存在，请重新上传',
    fieldLogo: '应用图标 URL',
    fieldLogoHint: '可选，https 图片地址；留空则自动生成 GitHub 风格 identicon 图标（浅灰方底 + 镜像像素块）。',
    logoInvalid: (value) => `图标地址不合法：${value}（需 https，或本地 http://127.0.0.1 / http://localhost，且不带 #片段）`,
    fieldType: '客户端类型',
    typeConfidential: '机密（有密钥）',
    typePublic: '公开（无密钥，必须用 PKCE）',
    typeConfidentialShort: '机密',
    typePublicShort: '公开',
    typeConfidentialHint: '机密客户端持有 client_secret，只应部署在可信服务端。',
    typePublicHint: '公开客户端没有密钥，浏览器/桌面端必须使用 PKCE（S256）。',
    secretSet: '密钥 · 已设置',
    secretSetHint: '该应用持有 client_secret；密钥只在创建或重置时显示一次，之后只能重置。',
    secretUnset: '无密钥',
    fieldRedirect: '回调地址',
    fieldRedirectHint: '每行一个，必须是 https；本地开发允许 http://127.0.0.1 或 http://localhost，且不能带 #片段。',
    fieldScopes: '权限范围',
    fieldScopesHint: '点选该应用可申请的权限范围：用户授权时默认按这里申请，应用请求超出范围会被拒；不选则使用站点默认。',
    scopePresetAll: '全选',
    scopePresetNone: '清空',
    scopePresetProfile: '仅基础资料',
    scopeCustomTag: '历史',
    scopeRequiredTag: '必选',
    scopeAlwaysOnNote: '身份读取始终授予，无需勾选。',
    scopeDescProfile: '读取你的基础资料：用户名、昵称、注册时间与账号状态。',
    scopeDropWarning: (list) => `这些历史范围不在站点清单内，保存后将一并移除：${list}`,
    submitCreate: '创建',
    submitSave: '保存',
    submitting: '提交中…',
    cancel: '取消',
    idLabel: '客户端 ID',
    idTagPublic: '可公开',
    idHint: '客户端 ID 是公开标识，可以对外展示；需要保密的是客户端密钥 client_secret。',
    secretTitle: '请立即保存客户端密钥',
    secretHint: '客户端密钥（client_secret）只显示这一次；关闭后无法再次查看，只能重置密钥。',
    secretLabelId: '客户端 ID',
    secretLabelKey: '客户端密钥',
    secretTagPrivate: '保密',
    secretIdNote: '公开标识：可以写进应用配置、也可以被用户看到，泄漏没有风险。',
    secretKeyNote: '保密凭据：只放在可信服务端，不要提交到代码仓库、也不要下发到浏览器。',
    secretPairsNote: '两者不是公私钥对：客户端 ID 相当于用户名（公开），客户端密钥相当于密码（保密），没有需要分发的公钥。',
    secretDone: '我已保存',
    rotateTitle: '重置密钥',
    rotateHint: '重置后旧密钥立即失效，使用旧密钥的集成会开始报错。',
    rotateConfirm: '确认重置',
    deleteTitle: '删除应用',
    deleteHint: '删除后该应用的所有授权与令牌都会作废，且无法恢复。',
    deleteConfirm: '确认删除',
    created: '应用已创建',
    saved: '应用已更新',
    rotated: '密钥已重置',
    deleted: '应用已删除',
    gateTitle: '站点已关闭自助创建',
    gateBody: '你仍然可以管理自己已有的应用；如需新建，请联系站点管理员。',
    nameRequired: '请填写应用名称。',
    redirectRequired: '请至少填写一个回调地址。',
    redirectInvalid: (value) => `回调地址不合法：${value}（需 https，或本地的 http://127.0.0.1 / http://localhost，且不带 #片段）`,
    authHint: (count) => `共 ${count} 个应用可以访问你的账号`,
    authEmptyTitle: '还没有授权任何应用',
    authEmptyHint: '当你在第三方应用里点击「用 SForum 登录」并允许后，这里会出现它。',
    grantedAt: (value) => `授权于 ${value}`,
    revoke: '撤销授权',
    revokeTitle: '撤销授权',
    revokeHint: '撤销后该应用将无法再读取你的资料，它的访问令牌与刷新令牌会立即作废。',
    revokeConfirm: '确认撤销',
    revoked: '授权已撤销',
    storeHint: (source) => `存储：${source} · 你只能看到并管理自己创建的应用。`,
    mountMismatch: '页面制品与前端脚本版本不一致：缺少挂载点 %s。请重新激活扩展（禁用 → 升级 → 启用）。',
    requestFailed: '请求失败',
    loadAppsFailed: '应用列表加载失败',
    loadAuthsFailed: '已授权应用加载失败',
    searchPlaceholder: '搜索名称、client_id 或回调地址',
    apiInfo: '接入信息',
    apiInfoTitle: 'OAuth2 端点',
    apiInfoNote: '访问令牌以 access_token 查询参数交付；服务端到服务端调用需先取 CSRF。',
    searchNoMatchTitle: '没有匹配的应用',
    searchNoMatchHint: '换个关键词试试。',
    moreActions: '更多操作',
    editApp: '编辑应用',
    copyClientId: '复制 client_id',
    factCallback: '回调',
    factScopes: '权限',
    factSecret: '密钥已设置',
    factPkce: 'PKCE S256',
    scopesCount: (count) => `${count} 项`,
    footNever: '从未使用',
    footUsed: (value) => `最近使用 ${value}`,
    footGranted: '授权于'
  },
  'en-US': {
    appsTab: 'My applications',
    authTab: 'Authorized',
    tabsLabel: 'OAuth application views',
    appsHint: (count, source) => `${count} application(s) · store: ${source}`,
    refresh: 'Refresh',
    create: 'New application',
    edit: 'Edit',
    rotate: 'Rotate secret',
    remove: 'Delete',
    copy: 'Copy',
    copied: 'Copied',
    copiedToast: 'Copied to clipboard',
    copyFailed: 'Copy failed: clipboard permission denied; copy manually instead.',
    tagRequired: 'Required',
    tagOptional: 'Optional',
    formSub: 'The application can only request the scopes you tick below; users confirm them once more on the consent screen.',
    never: 'Never used',
    usedAt: (value) => `Last used ${value}`,
    emptyTitle: 'No applications yet',
    emptyHint: 'Create your first application to get a client_id and secret, then wire up the OAuth2 flow.',
    appsForbiddenTitle: 'You cannot create applications',
    appsForbiddenHint: 'You can still review and revoke apps authorized to access your account. Contact the site administrator to create an OAuth app.',
    appsForbiddenDesc: 'Your account cannot create OAuth applications',
    createTitle: 'New application',
    editTitle: 'Edit application',
    fieldName: 'Application name',
    fieldNameHint: 'Shown to users on the consent screen, e.g. "My blog".',
    fieldDescription: 'Description',
    fieldDescriptionHint: 'Optional, one line about what it does.',
    noDescription: 'No description',
    fieldIcon: 'Application icon',
    fieldIconHint: 'Optional: upload a PNG / JPEG / WebP up to 512 KiB; leave empty to auto-generate a GitHub-style identicon (light square, mirror pixel blocks).',
    iconUpload: 'Upload image',
    iconRemove: 'Remove icon',
    iconUseURL: 'Or use an image URL',
    iconUploading: 'Uploading…',
    iconUploaded: 'Uploaded; save to apply',
    iconUploadedSaved: 'Uploaded',
    iconUploadFailed: 'The image upload failed; try again',
    iconTypeInvalid: 'Only PNG / JPEG / WebP images are supported',
    iconTooLarge: (max) => `The image must be at most ${max}`,
    iconUnsupported: 'This host version cannot accept uploaded icons; upgrade SForum or use an image URL',
    iconRetry: 'The page state expired; refresh and try again',
    iconRevisionConflict: 'The image changed; upload it again',
    iconForbidden: 'This image is not yours or is already used by another feature',
    iconReferenceLimit: 'This extension reached its image reference limit; remove another app icon first',
    iconAttachmentMissing: 'The image no longer exists; upload it again',
    fieldLogo: 'Application icon URL',
    fieldLogoHint: 'Optional https image URL; leave empty to auto-generate a GitHub-style identicon (light square, mirror pixel blocks).',
    logoInvalid: (value) => `Invalid icon URL: ${value} (use https, or http://127.0.0.1 / http://localhost locally, without a #fragment)`,
    fieldType: 'Client type',
    typeConfidential: 'Confidential (has secret)',
    typePublic: 'Public (no secret, PKCE required)',
    typeConfidentialShort: 'Confidential',
    typePublicShort: 'Public',
    typeConfidentialHint: 'A confidential client holds a client_secret and must run on a trusted server.',
    typePublicHint: 'A public client has no secret; browsers and desktop apps must use PKCE (S256).',
    secretSet: 'Secret · set',
    secretSetHint: 'This client holds a client_secret; the secret is shown only once when created or rotated.',
    secretUnset: 'No secret',
    fieldRedirect: 'Redirect URIs',
    fieldRedirectHint: 'One per line, must be https; local development allows http://127.0.0.1 or http://localhost, without a #fragment.',
    fieldScopes: 'Scopes',
    fieldScopesHint: 'Pick the scopes this application may request: consent defaults to this list and out-of-range requests are rejected. Leave empty to use the site default.',
    scopePresetAll: 'Select all',
    scopePresetNone: 'Clear',
    scopePresetProfile: 'Profile only',
    scopeCustomTag: 'Legacy',
    scopeRequiredTag: 'Required',
    scopeAlwaysOnNote: 'Identity is always granted; no need to select.',
    scopeDescProfile: 'Read your basic profile: username, display name, registration date and account status.',
    scopeDropWarning: (list) => `These legacy scopes are not in the site allowlist and will be removed on save: ${list}`,
    submitCreate: 'Create',
    submitSave: 'Save',
    submitting: 'Submitting…',
    cancel: 'Cancel',
    idLabel: 'Client ID',
    idTagPublic: 'Shareable',
    idHint: 'The client ID is a public identifier and is safe to show; the client_secret is the part that must stay private.',
    secretTitle: 'Save the client secret now',
    secretHint: 'The client secret (client_secret) is shown once; after closing you can only rotate it.',
    secretLabelId: 'Client ID',
    secretLabelKey: 'Client secret',
    secretTagPrivate: 'Private',
    secretIdNote: 'Public identifier: safe in app configuration and visible to users.',
    secretKeyNote: 'Private credential: keep it on a trusted server — never in a repository or shipped to a browser.',
    secretPairsNote: 'This is not a public/private key pair: the client ID is like a username (public) and the client secret like a password (private); there is no public key to distribute.',
    secretDone: 'I saved it',
    rotateTitle: 'Rotate secret',
    rotateHint: 'The old secret stops working immediately and existing integrations will start failing.',
    rotateConfirm: 'Rotate',
    deleteTitle: 'Delete application',
    deleteHint: 'All authorizations and tokens for this application are revoked. This cannot be undone.',
    deleteConfirm: 'Delete',
    created: 'Application created',
    saved: 'Application updated',
    rotated: 'Secret rotated',
    deleted: 'Application deleted',
    gateTitle: 'Self-serve creation is disabled',
    gateBody: 'You can still manage your existing applications; contact the site administrator to create new ones.',
    nameRequired: 'An application name is required.',
    redirectRequired: 'At least one redirect URI is required.',
    redirectInvalid: (value) => `Invalid redirect URI: ${value} (use https, or http://127.0.0.1 / http://localhost locally, without a #fragment)`,
    authHint: (count) => `${count} application(s) can access your account`,
    authEmptyTitle: 'Nothing authorized yet',
    authEmptyHint: 'After you approve a third-party app with "Sign in with SForum", it shows up here.',
    grantedAt: (value) => `Granted ${value}`,
    revoke: 'Revoke',
    revokeTitle: 'Revoke authorization',
    revokeHint: 'The application loses access to your profile and its access/refresh tokens are revoked immediately.',
    revokeConfirm: 'Revoke access',
    revoked: 'Authorization revoked',
    storeHint: (source) => `Store: ${source} · you only see and manage applications you created.`,
    mountMismatch: 'Page artifact and front-end script versions differ: missing mount point %s. Please reactivate the extension (disable → upgrade → enable).',
    requestFailed: 'Request failed',
    loadAppsFailed: 'Failed to load applications',
    loadAuthsFailed: 'Failed to load authorizations',
    searchPlaceholder: 'Search name, client_id or redirect URI',
    apiInfo: 'Endpoints',
    apiInfoTitle: 'OAuth2 endpoints',
    apiInfoNote: 'Access tokens are delivered as the access_token query parameter; server-to-server calls must fetch a CSRF token first.',
    searchNoMatchTitle: 'No matching applications',
    searchNoMatchHint: 'Try a different keyword.',
    moreActions: 'More actions',
    editApp: 'Edit application',
    copyClientId: 'Copy client_id',
    factCallback: 'Callback',
    factScopes: 'Scopes',
    factSecret: 'Secret set',
    factPkce: 'PKCE S256',
    scopesCount: (count) => `${count} item(s)`,
    footNever: 'Never used',
    footUsed: (value) => `Last used ${value}`,
    footGranted: 'Granted'
  }
}

// 保存/删除请求必须带 Idempotency-Key：Host 只在它存在时才签发 Host Command 的
// actor 委派（claim / release），缺了它命令永远调不通。
function newIdempotencyKey() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `sfp-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

function readCookie(name) {
  for (const item of document.cookie.split(';')) {
    const [key, ...rest] = item.trim().split('=')
    if (key === name) return decodeURIComponent(rest.join('='))
  }
  return ''
}

// 插件路由成功时返回裸文档；Host 的错误信封是 {code,message,data}。两种形态都要能吃。
function unwrap(body) {
  if (!body || typeof body !== 'object') return {}
  const enveloped = body.data && typeof body.data === 'object'
    && (body.code !== undefined || body.message !== undefined)
  return enveloped ? body.data : body
}

function text(value) {
  return value === undefined || value === null ? '' : String(value)
}

function validRedirectURI(value) {
  let parsed
  try {
    parsed = new URL(value)
  } catch (error) {
    return false
  }
  if (parsed.hash) return false
  if (parsed.protocol === 'https:') return true
  return parsed.protocol === 'http:' && (parsed.hostname === '127.0.0.1' || parsed.hostname === 'localhost')
}

// logo 地址校验：与 backend validLogoURL 同规则（空 = 用生成 icon）。
function validLogoURL(value) {
  const trimmed = value.trim()
  if (!trimmed) return true
  if (trimmed.length > 2048) return false
  let parsed
  try {
    parsed = new URL(trimmed)
  } catch (error) {
    return false
  }
  if (parsed.hash) return false
  if (parsed.protocol === 'https:') return true
  return parsed.protocol === 'http:' && (parsed.hostname === '127.0.0.1' || parsed.hostname === 'localhost')
}

function formatTimestamp(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (input) => String(input).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

async function copyToClipboard(value) {
  // 剪贴板 API 在部分环境（标签页失焦、权限提示悬空）会永久挂起而非 reject，
  // 用 1.2s 竞速兜底：超时同样走 execCommand 回退，保证任何情况下都有明确反馈。
  try {
    await Promise.race([
      navigator.clipboard.writeText(value),
      new Promise((_, reject) => { window.setTimeout(() => reject(new Error('clipboard-timeout')), 1200) })
    ])
    return true
  } catch (error) {
    // 非安全上下文、权限被拒或 API 挂起时的回退路径。
    try {
      const area = document.createElement('textarea')
      area.value = value
      area.setAttribute('readonly', '')
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.append(area)
      area.select()
      const ok = document.execCommand('copy')
      area.remove()
      return ok
    } catch (fallbackError) {
      return false
    }
  }
}

// 宿主图标：从 Nuxt Icon 的服务端点取原始 body，内联成 SVG。
// 插件页面不加载 iconify CSS，这是唯一能拿到与宿主一致图标的途径。
// 引用允许两种写法：宿主 manifest 风格 `i-tabler:key` 与组件内惯用的 `lucide:key`。
function parseIconRef(reference) {
  const match = /^(?:i-)?([a-z0-9]+)[-:](.+)$/.exec(text(reference).trim())
  return match ? { collection: match[1], name: match[2] } : null
}

// 缓存键统一带 i- 前缀存储；查询时对两种写法都兼容。
function iconCacheKey(reference) {
  const key = text(reference).trim()
  return iconCache.has(key) ? key : `i-${key}`
}

// Iconify 对改名后的旧名只回 aliases[name].parent，正文挂在 parent 名下
// （lucide 的 more-horizontal → ellipsis 就是这样）。不解析别名就会把引用
// 永远当成缺失，按钮渲染成空方块。
function resolveIconBody(payload, name) {
  let current = text(name)
  for (let hop = 0; hop < 4 && current; hop += 1) {
    const icon = payload && payload.icons ? payload.icons[current] : null
    if (icon && typeof icon.body === 'string') return icon.body
    const alias = payload && payload.aliases ? payload.aliases[current] : null
    current = alias && typeof alias.parent === 'string' ? alias.parent : ''
  }
  return ''
}

async function ensureIcons(references) {
  const wanted = new Map()
  for (const reference of references) {
    const parsed = parseIconRef(reference)
    if (!parsed) continue
    if (iconCache.has(iconCacheKey(reference))) continue
    const names = wanted.get(parsed.collection) || []
    names.push(parsed.name)
    wanted.set(parsed.collection, names)
  }
  for (const [collection, names] of wanted) {
    try {
      const response = await fetch(`${ICON_ENDPOINT}/${collection}.json?icons=${encodeURIComponent(names.join(','))}`, {
        credentials: 'same-origin',
        headers: { Accept: 'application/json' }
      })
      if (!response.ok) continue
      const payload = await response.json()
      for (const [name, icon] of Object.entries(payload.icons || {})) {
        if (icon && typeof icon.body === 'string') iconCache.set(`i-${collection}:${name}`, icon.body)
      }
      // 再按「请求名」落一份：别名（以及将来任何被改名的图标）也能直接命中。
      for (const name of names) {
        const body = resolveIconBody(payload, name)
        if (body) iconCache.set(`i-${collection}:${name}`, body)
      }
    } catch (error) {
      // 图标不可用时退化为无图标，不阻塞页面。
    }
  }
}

function iconElement(reference, size) {
  const body = iconCache.get(iconCacheKey(reference))
  if (!body) return null
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  svg.setAttribute('viewBox', '0 0 24 24')
  svg.setAttribute('width', String(size || 18))
  svg.setAttribute('height', String(size || 18))
  svg.setAttribute('aria-hidden', 'true')
  svg.style.display = 'block'
  svg.innerHTML = body
  return svg
}

// 宿主 Nuxt UI 的 Toast 通道（与论坛 core 同一套轮子）：
// app.vue 的 <UApp :toaster> 渲染 useState('toasts')，对应 payload.state.$stoasts
// （宿主侧是 reactive 代理），在 Vue 上下文外向该数组投递即可复用同一渲染管线。
function resolveNuxtApp() {
  if (typeof window.useNuxtApp === 'function') {
    try { return window.useNuxtApp() } catch { /* 落到实例反查 */ }
  }
  // 全局名缺失（水化异常/构建差异）时从 Vue 应用实例反查：nuxt 把实例挂在 $nuxt。
  const host = document.querySelector('#__nuxt')
  const vueApp = host && host.__vue_app__
  return vueApp && vueApp.$nuxt ? vueApp.$nuxt : null
}

function pushHostToast(toast) {
  try {
    const nuxtApp = resolveNuxtApp()
    if (!nuxtApp || !nuxtApp.payload || !nuxtApp.payload.state) return false
    const state = nuxtApp.payload.state
    // Toaster 尚未初始化该 state 时就地创建（reactive 代理保证后续 useState 复用同一数组）。
    if (!Array.isArray(state.$stoasts)) state.$stoasts = []
    state.$stoasts.push({
      id: `sfo-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
      open: true,
      _animatedAt: Date.now(),
      ...toast
    })
    if (state.$stoasts.length > 5) state.$stoasts.splice(0, state.$stoasts.length - 5)
    return true
  } catch {
    return false
  }
}

export function mount(target, bridge) {
  const locale = text(bridge && bridge.locale).startsWith('en') ? 'en-US' : 'zh-CN'
  const t = MESSAGES[locale]

  const root = document.createElement('div')
  root.className = 'sfp-root'
  root.innerHTML = `
    <div class="sfp-modal sfp-hidden" data-role="modal" hidden>
      <div class="sfp-modal__backdrop" data-action="close-modal"></div>
      <div class="sfp-modal__dialog" role="dialog" aria-modal="true" aria-labelledby="sfp-modal-title">
        <header class="sfp-modal__head">
          <h3 class="sfp-modal__title" id="sfp-modal-title" data-role="modal-title"></h3>
          <p class="sfp-modal__sub sfp-hidden" data-role="modal-sub" hidden></p>
        </header>

        <div class="sfp-modal__body" data-view="form">
          <form class="sfp-form" data-role="form" novalidate>
            <div class="sfp-field">
              <label class="sfp-field__label" for="sfp-name">${t.fieldName}<span class="sfp-field__tag">${t.tagRequired}</span></label>
              <p class="sfp-field__hint">${t.fieldNameHint}</p>
              <input class="sfp-input" id="sfp-name" name="name" maxlength="120" autocomplete="off" required>
            </div>
            <div class="sfp-field">
              <label class="sfp-field__label" for="sfp-description">${t.fieldDescription}<span class="sfp-field__tag">${t.tagOptional}</span></label>
              <p class="sfp-field__hint">${t.fieldDescriptionHint}</p>
              <input class="sfp-input" id="sfp-description" name="description" maxlength="500" autocomplete="off">
            </div>
            <div class="sfp-field">
              <span class="sfp-field__label">${t.fieldIcon}<span class="sfp-field__tag">${t.tagOptional}</span></span>
              <p class="sfp-field__hint">${t.fieldIconHint}</p>
              <div class="sfp-logo-row">
                <span class="sfp-avatar sfp-avatar--preview" data-role="logo-preview" aria-hidden="true"></span>
                <div class="sfp-logo-actions">
                  <label class="sf-button sf-button--sm sf-button--secondary sfp-file-pick">
                    <span class="sf-button__label">${t.iconUpload}</span>
                    <input type="file" id="sfp-logo-file" accept="image/png,image/jpeg,image/webp" data-role="logo-file" hidden>
                  </label>
                  <button type="button" class="sf-button sf-button--sm sf-button--ghost sfp-hidden" data-action="clear-icon" data-role="icon-clear" hidden>${t.iconRemove}</button>
                  <p class="sfp-field__hint sfp-icon-status" data-role="icon-status" aria-live="polite"></p>
                </div>
              </div>
              <details class="sfp-logo-url" data-role="logo-url">
                <summary class="sfp-logo-url__summary">${t.iconUseURL}</summary>
                <label class="sfp-field__label" for="sfp-logo">${t.fieldLogo}</label>
                <input class="sfp-input" id="sfp-logo" name="logoUrl" maxlength="2048" autocomplete="off" spellcheck="false" placeholder="https://example.com/logo.png">
                <p class="sfp-field__hint">${t.fieldLogoHint}</p>
              </details>
            </div>
            <div class="sfp-field">
              <span class="sfp-field__label">${t.fieldType}</span>
              <input type="hidden" id="sfp-type" name="clientType" value="confidential">
              <div class="sfp-typecards" role="radiogroup" aria-label="${t.fieldType}" data-role="type-cards">
                <label class="sfp-typecard">
                  <input type="radio" name="sfp-type-radio" value="confidential" checked>
                  <b>${t.typeConfidentialShort}</b>
                  <small>${t.typeConfidentialHint}</small>
                </label>
                <label class="sfp-typecard">
                  <input type="radio" name="sfp-type-radio" value="public">
                  <b>${t.typePublicShort}</b>
                  <small>${t.typePublicHint}</small>
                </label>
              </div>
            </div>
            <div class="sfp-field">
              <label class="sfp-field__label" for="sfp-redirect">${t.fieldRedirect}<span class="sfp-field__tag">${t.tagRequired}</span></label>
              <p class="sfp-field__hint">${t.fieldRedirectHint}</p>
              <textarea class="sfp-input" id="sfp-redirect" name="redirectUris" rows="3" spellcheck="false"></textarea>
            </div>
            <div class="sfp-field">
              <span class="sfp-field__label">${t.fieldScopes}</span>
              <p class="sfp-field__hint">${t.fieldScopesHint}</p>
              <div class="sfp-chips" data-role="scopes-chips" role="group" aria-label="${t.fieldScopes}"></div>
              <p class="sfp-field__hint sfp-scopes-note" data-role="scopes-note" aria-live="polite"></p>
            </div>
          </form>
        </div>

        <div class="sfp-modal__body sfp-hidden" data-view="secret" hidden>
          <div class="sfp-callout sfp-callout--warning">
            <p class="sfp-callout__title">${t.secretTitle}</p>
            <p class="sfp-callout__body">${t.secretHint}</p>
          </div>
          <dl class="sfp-pairs" data-role="secret-pairs"></dl>
        </div>

        <div class="sfp-modal__body sfp-hidden" data-view="confirm" hidden>
          <div class="sfp-callout sfp-callout--danger">
            <p class="sfp-callout__title" data-role="confirm-title"></p>
            <p class="sfp-callout__body" data-role="confirm-hint"></p>
          </div>
        </div>

        <footer class="sfp-modal__foot">
          <button type="button" class="sf-button sf-button--md sf-button--secondary" data-action="close-modal" data-role="foot-cancel">${t.cancel}</button>
          <button type="button" class="sf-button sf-button--md sf-button--primary" data-role="modal-submit"></button>
        </footer>
      </div>
    </div>
  `

  // 模板已经把三栏 chrome 渲染好了；组件只填充宿主无法预渲染的部分。
  // 左栏是宿主岛，本组件完全不碰。
  const page = target.closest('.sforum-settings') || document.querySelector('.sforum-settings')
  if (!page) return () => {}

  const nodes = {}
  const missing = []
  for (const [key, selector] of Object.entries(MOUNT_POINTS)) {
    const element = page.querySelector(selector)
    if (!element) missing.push(selector)
    nodes[key] = element
  }
  // 制品漂移要在页面上看得见：以前缺挂载点时静默空转，页面只剩骨架。
  // 主通道同样是宿主 Toast（duration 0 = 常驻到手动关闭，文案较长不抢时间）；
  // 宿主 Toast 管线也不可用时才退回页面内横幅，故障信息不能两头都丢。
  if (missing.length > 0) {
    const message = t.mountMismatch.replace('%s', missing.join(', '))
    console.error(message)
    if (pushHostToast({ color: 'error', icon: 'i-lucide-circle-alert', title: message, duration: 0 })) {
      return () => {}
    }
    const banner = document.createElement('p')
    banner.className = 'sfp-status sfp-status--error'
    banner.setAttribute('role', 'alert')
    banner.textContent = message
    page.querySelector('.sforum-settings__main')?.append(banner) ?? page.append(banner)
    return () => banner.remove()
  }

  const { appsBody, authBody } = nodes
  const statusEl = nodes.status
  const headActions = page.querySelector('.sforum-settings__head-actions')

  // 宿主约定：页面标题的 h1 带 data-slot="title"，以此豁免 main.css 里
  // `h1:not([data-slot="title"])` 的巨字号规则。模板净化器剥掉 data-*，且未豁免的
  // h1 首帧就是 56px 巨字（L2 CSS 到位前闪一帧大字），所以模板先输出 <p> 占位，
  // 这里升级成带豁免的 h1；apps.css 的 (0,2,0) 选择器对两种元素给同一字号。
  // 标签页标题完全交给宿主 SEO 管道渲染（贡献声明 label → {pageTitle} | {seoSiteName}）。
  // 这里绝不能手写 document.title：直改 DOM 会绕过宿主 unhead，把带站名后缀的
  // 合规标题覆盖成裸页面名，且在 unhead 之后的执行时序下永远不会被恢复。
  const headTitle = page.querySelector('.sfp-head__title')
  if (headTitle && headTitle.tagName !== 'H1') {
    const upgraded = document.createElement('h1')
    upgraded.className = headTitle.className
    upgraded.id = headTitle.id || 'sfp-page-title'
    upgraded.setAttribute('data-slot', 'title')
    upgraded.textContent = headTitle.textContent
    headTitle.replaceWith(upgraded)
  } else if (headTitle) {
    headTitle.setAttribute('data-slot', 'title')
  }

  page.append(root)

  const modal = root.querySelector('[data-role="modal"]')
  const modalTitle = root.querySelector('[data-role="modal-title"]')
  const modalSub = root.querySelector('[data-role="modal-sub"]')
  const modalSubmit = root.querySelector('[data-role="modal-submit"]')
  const formEl = root.querySelector('[data-role="form"]')
  const typeCards = root.querySelector('[data-role="type-cards"]')
  const secretPairs = root.querySelector('[data-role="secret-pairs"]')
  const confirmTitle = root.querySelector('[data-role="confirm-title"]')
  const confirmHint = root.querySelector('[data-role="confirm-hint"]')
  const footCancel = root.querySelector('[data-role="foot-cancel"]')
  // 显式引用字段：form.name 会命中 HTMLFormElement 自带属性，不能用 form.<name> 取值。
  const fields = {
    name: root.querySelector('#sfp-name'),
    description: root.querySelector('#sfp-description'),
    logoUrl: root.querySelector('#sfp-logo'),
    logoFile: root.querySelector('#sfp-logo-file'),
    clientType: root.querySelector('#sfp-type'),
    redirectUris: root.querySelector('#sfp-redirect')
  }
  const logoUrlBlock = root.querySelector('[data-role="logo-url"]')
  const iconStatus = root.querySelector('[data-role="icon-status"]')
  const iconClear = root.querySelector('[data-role="icon-clear"]')
  // 图标来源（与后端 attachment > url > generated 一一对应）：
  //   keep       —— 这次没动图标：两个字段都不提交，服务端保留原图；
  //   attachment —— 本次上传：提交 attachmentPublicId + attachmentRevision，claim；
  //   url        —— 用图片地址：提交 logoUrl（服务端会先 release 旧附件）；
  //   generated  —— 显式清除：两个字段都传空串，回到生成图标。
  let iconChoice = { kind: 'keep', publicId: '', revision: '', previewURL: '' }
  // 旧宿主（或账号没有 attachment.upload）时置真：上传控件降级，URL 区展开。
  let iconUploadBlocked = false
  // logo 实时预览：与卡片同一 renderIcon 通道，输入即重绘（element 会被替换，引用随之更新）。
  let logoPreview = root.querySelector('[data-role="logo-preview"]')
  // 权限范围：chip 多选（选项 = 站点 allowed_scopes，来自 /clients 响应；不再手填）。
  // scopeCatalog 是插件下发的 scope 目录（名称 + i18n 描述 + alwaysOn），
  // 描述优先读目录，MESSAGES 里的文案只做目录缺失时的回退。
  const scopesChips = root.querySelector('[data-role="scopes-chips"]')
  const scopesNote = root.querySelector('[data-role="scopes-note"]')
  let allowedScopes = []
  let scopeCatalog = []
  let pickedScopes = new Set()
  let customScopes = []
  const views = {
    form: root.querySelector('[data-view="form"]'),
    secret: root.querySelector('[data-view="secret"]'),
    confirm: root.querySelector('[data-view="confirm"]')
  }

  // 标签页：模板只给出结构（role=tab + href=#锚点），aria 状态与 tabindex 在这里补齐，
  // 因为宿主模板净化器会剥掉 aria-selected/aria-controls/tabindex。
  const tabs = [
    { id: 'apps', tab: nodes.tabApps, panel: nodes.panelApps },
    { id: 'authorized', tab: nodes.tabAuths, panel: nodes.panelAuths }
  ]
  for (const item of tabs) {
    item.tab.setAttribute('aria-controls', item.panel.id)
    item.tab.id = item.tab.id || `sfp-tab-${item.id}`
    item.panel.setAttribute('tabindex', '0')
  }
  const tabsNav = nodes.tabApps.parentElement
  if (tabsNav) tabsNav.setAttribute('aria-label', t.tabsLabel)
  nodes.tabAppsLabel.textContent = t.appsTab
  nodes.tabAuthsLabel.textContent = t.authTab

  // 宿主 CSS 给 .sfp-modal/.sfp-status 等设了 display，[hidden] 会被盖掉，
  // 因此隐藏状态一律用 sfp-hidden 类（apps.css 里 display:none!important）。
  // 同时必须同步原生 hidden 属性：模板里的 hidden 属性若留着，弹窗本体永远是
  // display:none（此前只切类名，导致前台「新建/重置/删除」弹窗完全不可见）。
  const setHidden = (element, value) => {
    if (!element) return
    const hidden = Boolean(value)
    element.classList.toggle('sfp-hidden', hidden)
    if (hidden) element.setAttribute('hidden', '')
    else element.removeAttribute('hidden')
    if (hidden) element.setAttribute('aria-hidden', 'true')
    else element.removeAttribute('aria-hidden')
  }
  const isHidden = (element) => !element || element.classList.contains('sfp-hidden')

  // 「站点已关闭自助创建」提示不再放进模板：L2 CSS 到位前 .sfp-hidden 不生效，
  // 静态块会在每次加载时对着已开启自助创建的站点闪一帧错误内容。改为挂载后创建
  // （此时阻塞式 apps.css 已随组件资源图加载），可见性仍由 load() 按 canCreate 切换。
  const gate = document.createElement('div')
  gate.className = 'sfp-notice sfp-gate sfp-hidden'
  gate.innerHTML = `<p class="sfp-notice__title">${t.gateTitle}</p><p class="sfp-notice__body">${t.gateBody}</p>`
  // 工具行（tabs 现在包在 .sfp-toolbar 里）之前插入，避免 gate 掉进 flex 行内
  const toolbarEl = page.querySelector('.sfp-toolbar')
  if (toolbarEl) toolbarEl.before(gate)
  else tabsNav?.before(gate)

  let clients = []
  let authorizations = []
  let canCreate = false
  // 页面现在对全体登录用户开放：没有 create_clients 的用户 /clients 会回 403，
  // 此时只看「已授权」，不把 403 当成加载失败弹错误。
  let canReadApps = true
  // 首次数据到达前列表必须保持模板骨架：图标请求先于 /clients 返回时，
  // 旧实现会把骨架换成「还没有应用」空态再换成真实行——多闪两帧错误内容。
  let appsLoaded = false
  let authsLoaded = false
  let busy = false
  let view = 'form'
  let mode = 'create'
  let activeTab = 'apps'
  let activeClient = null
  let activeAuthorization = null
  let lastTrigger = null
  let csrfPromise = null
  let storeSource = 'database'

  const activateTab = (id, options = {}) => {
    const next = tabs.find((item) => item.id === id) || tabs[0]
    activeTab = next.id
    for (const item of tabs) {
      const selected = item === next
      item.tab.classList.toggle('is-active', selected)
      item.tab.setAttribute('aria-selected', selected ? 'true' : 'false')
      item.tab.setAttribute('tabindex', selected ? '0' : '-1')
      item.panel.classList.toggle('sfp-hidden', !selected)
    }
    if (options.focus) next.tab.focus()
    if (options.syncHash !== false && window.location.hash !== `#${next.id}`) {
      // 深链可分享，但不为每次切换留一条历史记录。
      window.history.replaceState(null, '', `#${next.id}`)
    }
  }

  // 降级通道：正常路径的反馈全部走 Toast（见 notify），只有宿主 Toast 不可用时
  // 才把文字写回页面顶部这条 .sfp-status，保证错误至少还能被看见。
  const setStatus = (message, kind) => {
    if (!message) {
      statusEl.classList.add('sfp-hidden')
      statusEl.textContent = ''
      return
    }
    statusEl.classList.remove('sfp-hidden')
    statusEl.textContent = message
    statusEl.className = 'sfp-status sfp-status-mount' + (kind === 'error' ? ' sfp-status--error' : '')
  }

  const setBusy = (value) => {
    busy = value
    for (const button of root.querySelectorAll('button')) {
      if (button.dataset.role === 'static') continue
      button.disabled = value
    }
    if (view === 'form') {
      modalSubmit.textContent = value ? t.submitting : (mode === 'create' ? t.submitCreate : t.submitSave)
    }
  }

  // 非浏览器客户端要显式握手拿 csrf_ cookie；页面上该 cookie 可能尚未种下。
  const ensureCsrf = () => {
    if (readCookie('csrf_')) return Promise.resolve()
    if (!csrfPromise) {
      csrfPromise = fetch(API + '/csrf', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
        .catch(() => {})
        .then(() => {
          csrfPromise = null
        })
    }
    return csrfPromise
  }

  const api = async (path, options = {}) => {
    const method = (options.method || 'GET').toUpperCase()
    const headers = { Accept: 'application/json' }
    if (options.body !== undefined) headers['Content-Type'] = 'application/json'
    if (options.idempotencyKey) headers['Idempotency-Key'] = options.idempotencyKey
    if (method !== 'GET' && method !== 'HEAD') {
      await ensureCsrf()
      headers['X-Csrf-Token'] = readCookie('csrf_')
    }
    const response = await fetch(API + path, {
      method,
      credentials: 'same-origin',
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body)
    })
    const payload = await response.json().catch(() => ({}))
    const data = unwrap(payload)
    if (!response.ok) {
      const reason = data.reason || data.error || `HTTP ${response.status}`
      const detail = payload.message || data.errorDescription || ''
      const error = new Error(detail && detail !== reason ? `${reason}: ${detail}` : reason)
      // 稳定错误码要能被调用方读到（图标上传降级、修订冲突都靠它分流）。
      error.code = reason
      throw error
    }
    return data
  }

  const button = (label, variant, size, handler) => {
    const element = document.createElement('button')
    element.type = 'button'
    element.className = `sf-button sf-button--${size || 'sm'} sf-button--${variant || 'secondary'}`
    const span = document.createElement('span')
    span.className = 'sf-button__label'
    span.textContent = label
    element.append(span)
    element.addEventListener('click', handler)
    return element
  }

  const wrapIcon = (svg) => {
    const span = document.createElement('span')
    span.className = 'sf-button__icon'
    span.append(svg)
    return span
  }

  const mono = (value, fullValue) => {
    const code = document.createElement('code')
    code.className = 'sfp-mono'
    code.textContent = value
    code.title = fullValue || value
    return code
  }

  // 复制成功反馈：图标钮短暂打 ✓，不看状态条也能确认。
  const flashCopyDone = (trigger) => {
    if (!trigger) return
    const slot = trigger.querySelector('.sf-button__icon')
    const done = slot && iconElement('lucide:check', 13)
    if (!done) return
    slot.replaceChildren(done)
    trigger.classList.add('sfp-copy--done')
    window.setTimeout(() => {
      const copy = iconElement('lucide:copy', 13)
      if (copy) slot.replaceChildren(copy)
      trigger.classList.remove('sfp-copy--done')
    }, 1400)
  }

  // 统一反馈出口：所有操作结果都走宿主 Toast（见模块顶部 pushHostToast），页面不再显示
  // 文字状态条。只有宿主 Toast 管线不可用时（水化异常 / 无 toaster），才降级回页面内的
  // .sfp-status 条——错误不能静默丢失。success/info 4s 自动消失，error 6s。
  const clearStatus = () => setStatus('')
  const notify = (title, options = {}) => {
    const text = String(title == null ? '' : title).trim()
    if (!text) return
    const kind = options.kind === 'error' ? 'error' : options.kind === 'info' ? 'info' : 'success'
    const toast = {
      color: kind,
      icon: kind === 'error' ? 'i-lucide-circle-alert' : kind === 'info' ? 'i-lucide-info' : 'i-lucide-check',
      title: text,
      duration: options.duration == null ? (kind === 'error' ? 6000 : 4000) : options.duration
    }
    const description = String(options.description == null ? '' : options.description).trim()
    if (description && description !== text) toast.description = description
    if (pushHostToast(toast)) {
      clearStatus()
      return
    }
    setStatus(description && description !== text ? `${text}：${description}` : text, kind === 'error' ? 'error' : undefined)
  }

  // 成功轻提示 + 失败显眼报错；文案不回显凭据值（密钥场景不泄漏）。
  const copyFeedback = async (value, trigger) => {
    const ok = await copyToClipboard(value)
    if (ok) {
      flashCopyDone(trigger)
      notify(t.copiedToast)
    } else {
      notify(t.copyFailed, { kind: 'error' })
    }
    return ok
  }

  // client_id 芯片：完整显示，整块可点击复制（触屏上 hover title 不可用，点击是主通道）。
  const idChip = (value, trigger) => {
    const code = mono(value, value)
    code.classList.add('sfp-mono--copy')
    code.setAttribute('role', 'button')
    code.tabIndex = 0
    const copy = () => copyFeedback(value, trigger)
    code.addEventListener('click', copy)
    code.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault()
        copy()
      }
    })
    return code
  }

  const copyButton = (value) => button(t.copy, 'secondary', 'sm', () => copyFeedback(value))

  // 卡片上的复制钮：图标占位 + 晚到的图标补丁（见 applyCopyIcons）。
  const copyIconButton = (value) => {
    const element = document.createElement('button')
    element.type = 'button'
    element.className = 'sf-button sf-button--sm sf-button--ghost sfp-copy'
    element.title = `${t.copy} client_id`
    element.setAttribute('aria-label', `${t.copy} client_id`)
    const slot = document.createElement('span')
    slot.className = 'sf-button__icon'
    const svg = iconElement('lucide:copy', 13)
    if (svg) slot.append(svg)
    element.append(slot)
    element.addEventListener('click', () => copyFeedback(value, element))
    return element
  }

  // 图标晚于卡片渲染到达时就地补 icon span，不重建卡片（重建会闪一帧并丢焦点）。
  const applyCopyIcons = () => {
    page.querySelectorAll('.sfp-copy .sf-button__icon').forEach((slot) => {
      if (slot.childElementCount > 0) return
      const svg = iconElement('lucide:copy', 13)
      if (svg) slot.append(svg)
    })
  }

  // ⋯ 菜单按钮的图标同样是晚到的：卡片可能先于图标缓存渲染，这里就地补，
  // 顺手清掉 moreMenu 写下的兜底字形。
  const applyMoreIcons = () => {
    page.querySelectorAll('.sfp-card__more-btn').forEach((toggle) => {
      if (toggle.childElementCount > 0) return
      const svg = iconElement('lucide:ellipsis', 15)
      if (!svg) return
      toggle.textContent = ''
      toggle.append(svg)
    })
  }

  const fillIconSlots = () => {
    const searchSlot = page.querySelector('.sfp-cmdbar__icon')
    if (searchSlot && searchSlot.childElementCount === 0) {
      const svg = iconElement('lucide:search', 15)
      if (svg) searchSlot.append(svg)
    }
    const apiSlot = page.querySelector('.sfp-api-toggle .sfp-icon-slot')
    if (apiSlot && apiSlot.childElementCount === 0) {
      const svg = iconElement('lucide:info', 13)
      if (svg) apiSlot.append(svg)
    }
    page.querySelectorAll('.sfp-add-tile__plus').forEach((slot) => {
      if (slot.childElementCount > 0) return
      const svg = iconElement('lucide:plus', 16)
      if (svg) slot.append(svg)
    })
  }

  // 权限范围 chip 多选：选项 = 站点 allowed_scopes（/clients 响应）。
  // profile 由后端 normalizeScopes 无条件并入，属「必选」：锁定 chip、恒为选中、
  // 提交时恒含。历史遗留的清单外 scope 以锁定 chip 呈现并提示保存后移除。
  const scopeInfo = (scope) => {
    const entry = scopeCatalog.find((item) => item && item.name === scope) || null
    const lang = locale === 'en-US' ? 'en-US' : 'zh-CN'
    const raw = entry && entry.description ? entry.description[lang] || entry.description['en-US'] || entry.description['zh-CN'] : ''
    return {
      desc: text(raw) || (scope === 'profile' ? t.scopeDescProfile : ''),
      alwaysOn: Boolean(entry && entry.alwaysOn) || scope === 'profile'
    }
  }
  const scopeNote = () => {
    if (!scopesNote) return
    if (mode === 'edit' && customScopes.length > 0) {
      scopesNote.textContent = t.scopeDropWarning(customScopes.join(' '))
      return
    }
    scopesNote.textContent = ''
  }
  const renderScopeChips = () => {
    if (!scopesChips) return
    scopesChips.textContent = ''
    for (const scope of allowedScopes) {
      const info = scopeInfo(scope)
      const on = pickedScopes.has(scope)
      const name = document.createElement('span')
      name.className = 'sfp-chip__name'
      name.textContent = scope
      const chip = document.createElement(info.alwaysOn ? 'span' : 'button')
      chip.className = 'sfp-chip' + (on || info.alwaysOn ? ' is-on' : '') + (info.alwaysOn ? ' sfp-chip--locked' : '')
      if (!info.alwaysOn) {
        chip.type = 'button'
        chip.setAttribute('aria-pressed', on ? 'true' : 'false')
      }
      chip.append(name)
      if (info.desc) {
        const hint = document.createElement('span')
        hint.className = 'sfp-chip__desc'
        hint.textContent = info.desc
        chip.append(hint)
      }
      const tag = document.createElement('span')
      if (info.alwaysOn) {
        tag.className = 'sfp-chip__tag sfp-chip__tag--accent'
        tag.textContent = t.scopeRequiredTag
        chip.append(tag)
      }
      if (info.alwaysOn) {
        chip.title = t.scopeAlwaysOnNote
      } else {
        chip.addEventListener('click', () => {
          if (pickedScopes.has(scope)) pickedScopes.delete(scope)
          else pickedScopes.add(scope)
          renderScopeChips()
        })
      }
      scopesChips.append(chip)
    }
    for (const scope of customScopes) {
      const chip = document.createElement('span')
      chip.className = 'sfp-chip sfp-chip--locked'
      const name = document.createElement('span')
      name.className = 'sfp-chip__name'
      name.textContent = scope
      const tag = document.createElement('span')
      tag.className = 'sfp-chip__tag'
      tag.textContent = t.scopeCustomTag
      chip.append(name, tag)
      chip.title = t.scopeDropWarning(scope)
      scopesChips.append(chip)
    }
    if (allowedScopes.length > 1) {
      const presets = document.createElement('div')
      presets.className = 'sfp-scope-presets'
      const preset = (label, apply) => {
        const btn = document.createElement('button')
        btn.type = 'button'
        btn.className = 'sfp-chip-preset'
        btn.textContent = label
        btn.addEventListener('click', () => { apply(); renderScopeChips() })
        return btn
      }
      const presetBtns = [preset(t.scopePresetAll, () => { for (const scope of allowedScopes) pickedScopes.add(scope) })]
      if (allowedScopes.length > 2 && allowedScopes.includes('profile')) {
        // 只有 profile + 一个可选项时，「仅基础资料」与「清空」等价，不再重复展示。
        presetBtns.push(preset(t.scopePresetProfile, () => { pickedScopes.clear(); pickedScopes.add('profile') }))
      }
      presetBtns.push(preset(t.scopePresetNone, () => { pickedScopes.clear(); pickedScopes.add('profile') }))
      presets.append(...presetBtns)
      scopesChips.append(presets)
    }
    scopeNote()
  }

  const emptyState = (title, hint, withCreate) => {
    const wrap = document.createElement('div')
    wrap.className = 'sf-empty-state'
    const icon = document.createElement('div')
    icon.className = 'sf-empty-state__icon'
    const glyph = iconElement('lucide:key-round', 22)
    icon.append(glyph || document.createTextNode('OAuth2'))
    const t1 = document.createElement('h3')
    t1.className = 'sf-empty-state__title'
    t1.textContent = title
    const t2 = document.createElement('p')
    t2.className = 'sf-empty-state__description'
    t2.textContent = hint
    wrap.append(icon, t1, t2)
    if (withCreate) {
      wrap.append(button(t.create, 'primary', 'md', (event) => openForm('create', null, event.currentTarget)))
    }
    return wrap
  }

  // 统一 icon：后端 /clients、/authorizations、consent summary 都下发同构的
  // icon 描述符（backend/icon.go 是单一事实源），这里只按描述符渲染：
  // kind=attachment/url → 生成 tile 上叠 <img>（加载失败移除 img 露出 tile）；
  // kind=generated → 方型纯色底 + 5×5 identicon 像素块 + SFAvatar 首字母。
  // 本地镜像（stableHash / identiconPattern）只服务表单预览与描述符缺失兜底，
  // 算法与 backend/identicon.go 逐位一致（哈希原语同站点 SFAvatar）。
  const ICON_BACKGROUND = '#f0f0f0'

  const stableHash = (seed) => {
    const source = text(seed)
    let hash = 0
    for (let index = 0; index < source.length; index += 1) {
      hash = (Math.imul(hash, 31) + source.charCodeAt(index)) >>> 0
    }
    return hash
  }

  // 与 backend/identicon.go 的 identiconColor 逐字一致（S/L 取自 GitHub 实测区间）。
  const identiconColor = (seed) => {
    const base = text(seed).trim() || 'sforum'
    return 'hsl(' + (stableHash(base) % 360) + ' 60% 63%)'
  }

  const identiconPattern = (seed) => {
    const base = text(seed).trim() || 'sforum'
    const primary = stableHash(base)
    const secondary = stableHash(base + '\u001f')
    const filled = []
    for (let index = 0; index < 15; index += 1) {
      const nibble = index < 8
        ? (primary >>> (4 * index)) & 0xf
        : (secondary >>> (4 * (index - 8))) & 0xf
      filled.push(nibble % 2 === 0)
    }
    const cells = new Array(25).fill('0')
    let painted = false
    for (let row = 0; row < 5; row += 1) {
      for (let column = 0; column < 3; column += 1) {
        if (!filled[row * 3 + column]) continue
        cells[row * 5 + column] = '1'
        painted = true
      }
      cells[row * 5 + 3] = cells[row * 5 + 1]
      cells[row * 5 + 4] = cells[row * 5 + 0]
    }
    if (!painted) cells[12] = '1'
    return cells.join('')
  }

  const localDescriptor = (seed, name, iconUrl) => {
    const trimmedUrl = text(iconUrl).trim()
    const key = text(seed).trim() || text(name).trim() || 'sfo_client_preview'
    const descriptor = {
      kind: 'generated',
      pattern: identiconPattern(key),
      background: ICON_BACKGROUND,
      color: identiconColor(key)
    }
    if (trimmedUrl) {
      // 本地预览不区分 attachment / url：两者都只是「叠一张 <img>」。
      descriptor.kind = 'url'
      descriptor.url = trimmedUrl
    }
    return descriptor
  }

  // 5×5 像素块用内联 SVG 绘制（单节点、随 tile 缩放、无额外图片请求）。
  const renderIdenticon = (pattern, color) => {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
    svg.setAttribute('class', 'sfp-avatar__ident')
    svg.setAttribute('viewBox', '0 0 5 5')
    svg.setAttribute('aria-hidden', 'true')
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path')
    let data = ''
    for (let index = 0; index < 25; index += 1) {
      if (pattern.charAt(index) !== '1') continue
      data += 'M' + (index % 5) + ' ' + Math.floor(index / 5) + 'h1v1h-1z'
    }
    path.setAttribute('d', data)
    path.setAttribute('fill', text(color) || identiconColor(pattern))
    path.setAttribute('shape-rendering', 'crispEdges')
    svg.append(path)
    return svg
  }

  const renderIcon = (descriptor, seed, name, clientType) => {
    const icon = descriptor && descriptor.kind ? descriptor : localDescriptor(seed, name, '')
    const span = document.createElement('span')
    span.className = 'sfp-avatar sfp-avatar--' + (clientType === 'public' ? 'pub' : 'conf')
    span.setAttribute('aria-hidden', 'true')
    const tile = document.createElement('span')
    tile.className = 'sfp-avatar__tile'
    tile.style.background = text(icon.background) || ICON_BACKGROUND
    const pattern = text(icon.pattern) || identiconPattern(text(seed) || name)
    tile.append(renderIdenticon(pattern, text(icon.color) || identiconColor(text(seed) || name)))
    span.append(tile)
    if (icon.url) {
      const img = document.createElement('img')
      img.className = 'sfp-avatar__img'
      img.src = icon.url
      img.alt = ''
      img.loading = 'lazy'
      img.referrerPolicy = 'no-referrer'
      img.addEventListener('error', () => img.remove())
      span.append(img)
    }
    return span
  }

  // 表单里的图标预览：种子 = 已有 clientId（编辑态，颜色与保存后一致）或名称。
  const renderLogoPreview = () => {
    if (!logoPreview) return
    const name = fields.name.value.trim()
    const seed = text(activeClient && activeClient.clientId) || name || 'sfo_client_preview'
    const next = renderIcon(localDescriptor(seed, name, iconChoice.previewURL), seed, name, fields.clientType.value)
    next.classList.add('sfp-avatar--preview')
    logoPreview.replaceWith(next)
    logoPreview = next
  }

  const setIconStatus = (message, kind) => {
    if (!iconStatus) return
    iconStatus.textContent = text(message)
    iconStatus.classList.toggle('sfp-icon-status--error', kind === 'error')
  }

  // 每次来源变化都重画预览、同步「移除」按钮与状态文案。
  const applyIconState = (message, kind) => {
    renderLogoPreview()
    const hasIcon = Boolean(iconChoice.previewURL) || iconChoice.kind === 'attachment'
    setHidden(iconClear, !hasIcon)
    setIconStatus(message === undefined ? '' : message, kind)
  }

  // 把 Host / Core 的失败翻译成可读文案。上传不支持时展开 URL 输入作为降级，
  // 绝不静默退回 base64 内联。
  const iconMessage = (code) => {
    switch (code) {
      case 'icon_upload_unsupported': return t.iconUnsupported
      case 'idempotency_key_required': return t.iconRetry
      case 'icon_revision_conflict': return t.iconRevisionConflict
      case 'icon_too_large': return t.iconTooLarge('512 KiB')
      case 'icon_forbidden': return t.iconForbidden
      case 'icon_reference_limit': return t.iconReferenceLimit
      case 'icon_attachment_missing': return t.iconAttachmentMissing
      case 'icon_invalid': return t.iconTypeInvalid
      default: return t.iconUploadFailed
    }
  }

  const handleIconFailure = (error) => {
    const code = text(error && error.code)
    if (!code || code.indexOf('icon_') !== 0 && code !== 'idempotency_key_required') return false
    iconUploadBlocked = code === 'icon_upload_unsupported'
    if (iconUploadBlocked && logoUrlBlock) logoUrlBlock.open = true
    setIconStatus(iconMessage(code), 'error')
    return true
  }

  // 图标上传直连 Core 的附件接口：浏览器带着自己的会话把字节交给宿主，
  // 插件只拿 publicId + createdAt 去 claim（不在插件里存字节，也不加转发路由）。
  const uploadAttachment = async (file) => {
    await ensureCsrf()
    const body = new FormData()
    body.append('file', file)
    const headers = { Accept: 'application/json' }
    const token = readCookie('csrf_')
    if (token) headers['X-Csrf-Token'] = token
    const response = await fetch(CORE_API + '/attachments', {
      method: 'POST',
      credentials: 'same-origin',
      headers,
      body
    })
    const payload = await response.json().catch(() => ({}))
    const data = unwrap(payload)
    if (!response.ok) {
      throw new Error(text(payload.message) || text(data.reason) || text(data.error) || `HTTP ${response.status}`)
    }
    const publicId = text(data.publicId)
    if (!publicId) throw new Error(t.iconUploadFailed)
    return {
      publicId,
      // claim 的 expectedRevision 必须等于附件当前的 updated_at；上传响应里的
      // createdAt 与它来自同一条 INSERT 的两个 DEFAULT now()，取值相同。
      revision: text(data.createdAt),
      url: text(data.url) || `/media/attachments/${publicId}`
    }
  }

  const onIconFileChange = async () => {
    const file = fields.logoFile && fields.logoFile.files ? fields.logoFile.files[0] : null
    if (!file) return
    if (!ICON_TYPES.includes(file.type)) {
      setIconStatus(t.iconTypeInvalid, 'error')
      fields.logoFile.value = ''
      return
    }
    if (file.size > ICON_MAX_BYTES) {
      setIconStatus(t.iconTooLarge('512 KiB'), 'error')
      fields.logoFile.value = ''
      return
    }
    setIconStatus(t.iconUploading)
    try {
      const uploaded = await uploadAttachment(file)
      iconChoice = {
        kind: 'attachment',
        publicId: uploaded.publicId,
        revision: uploaded.revision,
        previewURL: uploaded.url
      }
      // 互斥：选了上传就清空地址输入，并收起 URL 区。
      fields.logoUrl.value = ''
      if (logoUrlBlock) logoUrlBlock.open = false
      applyIconState(t.iconUploaded)
    } catch (error) {
      setIconStatus(error.message || t.iconUploadFailed, 'error')
    } finally {
      if (fields.logoFile) fields.logoFile.value = ''
    }
  }

  // 地址输入是「次选」入口：一旦填了地址就丢掉待 claim 的上传（互斥）。
  const onLogoURLInput = () => {
    const value = fields.logoUrl.value.trim()
    if (value) {
      iconChoice = { kind: 'url', publicId: '', revision: '', previewURL: value }
      applyIconState('')
      return
    }
    if (iconChoice.kind === 'url') {
      iconChoice = { kind: 'generated', publicId: '', revision: '', previewURL: '' }
      applyIconState('')
    }
  }

  const onClearIcon = () => {
    iconChoice = { kind: 'generated', publicId: '', revision: '', previewURL: '' }
    fields.logoUrl.value = ''
    if (fields.logoFile) fields.logoFile.value = ''
    applyIconState('')
  }

  // 打开表单时把图标状态复位到「未改动」，预览沿用服务端下发的描述符地址。
  const resetIconChoice = (client) => {
    iconUploadBlocked = false
    iconChoice = {
      kind: 'keep',
      publicId: '',
      revision: '',
      previewURL: text(client && client.icon && client.icon.url)
    }
    if (logoUrlBlock) logoUrlBlock.open = Boolean(client && client.logoUrl && !client.attachmentPublicId)
    renderLogoPreview()
    setHidden(iconClear, !iconChoice.previewURL)
    setIconStatus(client && client.attachmentPublicId ? t.iconUploadedSaved : '')
  }

  // ---------- Gallery 画廊卡：封面（类型 / 活跃 / 水印）→ 跨界图标 + 正文 → 贴底操作 ----------

  // 回调值展示：协议与本地端口不值得占位，压成 host[:port]/path，完整 URI 在 title。
  const uriBrief = (value) => {
    const trimmed = text(value)
    if (!trimmed) return ''
    try {
      const url = new URL(trimmed)
      const path = url.pathname === '/' ? '' : url.pathname
      return url.host + path
    } catch {
      return trimmed
    }
  }

  const isRecent = (value) => {
    const stamp = Date.parse(text(value))
    return Number.isFinite(stamp) && Date.now() - stamp < 24 * 60 * 60 * 1000
  }

  // ⋯ 菜单：重置密钥 / 删除收进来（毛玻璃浮层，悬停显形），卡底只留主操作。
  const moreMenu = (items) => {
    const wrap = document.createElement('div')
    wrap.className = 'sfp-card__more'
    const toggle = document.createElement('button')
    toggle.type = 'button'
    toggle.className = 'sfp-card__more-btn'
    toggle.title = t.moreActions
    toggle.setAttribute('aria-label', t.moreActions)
    const svg = iconElement('lucide:ellipsis', 15)
    if (svg) toggle.append(svg)
    // 图标端点不可用时至少留一个可点的字形，而不是一块 27px 空白。
    else toggle.textContent = '⋯'
    const menu = document.createElement('div')
    menu.className = 'sfp-card__menu sfp-hidden'
    let previous = null
    for (const item of items) {
      if (item === '|') {
        if (previous && previous !== '|') menu.append(document.createElement('hr'))
        previous = item
        continue
      }
      if (item === null) continue
      const row = document.createElement('button')
      row.type = 'button'
      if (item.danger) row.className = 'sfp-menu--danger'
      row.textContent = item.label
      row.addEventListener('click', (event) => {
        event.stopPropagation()
        closeAllMenus()
        item.onClick(event)
      })
      menu.append(row)
      previous = item
    }
    toggle.addEventListener('click', (event) => {
      event.stopPropagation()
      const open = isHidden(menu)
      closeAllMenus()
      if (open) setHidden(menu, false)
    })
    wrap.append(toggle, menu)
    return wrap
  }

  const closeAllMenus = () => {
    page.querySelectorAll('.sfp-card__menu').forEach((menu) => setHidden(menu, true))
  }

  // 画廊卡：权限 chips（最多 4 个 + N）与封面水印（复用 identicon 图案，不额外请求）。
  const scopeChips = (scopeList) => {
    const wrap = document.createElement('div')
    wrap.className = 'sfp-card__scopes'
    const list = (Array.isArray(scopeList) ? scopeList : []).map(text).filter(Boolean)
    if (list.length === 0) return wrap
    for (const scope of list.slice(0, 4)) {
      const chip = document.createElement('span')
      chip.className = 'sfp-scope-chip'
      chip.textContent = scope
      wrap.append(chip)
    }
    if (list.length > 4) {
      const more = document.createElement('span')
      more.className = 'sfp-scope-chip sfp-scope-chip--more'
      more.textContent = `+${list.length - 4}`
      more.title = list.join(' ')
      wrap.append(more)
    }
    wrap.title = list.join(' ')
    return wrap
  }

  const cardWatermark = (descriptor, seed, name) => {
    const icon = descriptor && descriptor.kind ? descriptor : localDescriptor(seed, name, '')
    const pattern = text(icon.pattern) || identiconPattern(text(seed) || text(name))
    const wrap = document.createElement('span')
    wrap.className = 'sfp-card__watermark'
    wrap.setAttribute('aria-hidden', 'true')
    wrap.append(renderIdenticon(pattern, text(icon.color) || identiconColor(text(seed) || text(name))))
    return wrap
  }

  const cardCover = (options) => {
    const cover = document.createElement('div')
    cover.className = 'sfp-card__cover'
    const tag = document.createElement('span')
    tag.className = 'sfp-card__tag' + (options.isPublic ? ' is-public' : '')
    tag.textContent = options.tag
    const status = document.createElement('span')
    status.className = 'sfp-card__status' + (options.live ? ' is-live' : '')
    status.textContent = options.status
    status.title = options.statusTitle || options.status
    cover.append(tag, status, options.watermark)
    return cover
  }

  const clientCard = (client) => {
    const card = document.createElement('article')
    card.className = 'sfp-card'
    const fullClientId = text(client.clientId)
    const uris = Array.isArray(client.redirectUris) ? client.redirectUris : []
    const scopeList = Array.isArray(client.scopes) ? client.scopes : []
    const isPublic = client.clientType === 'public'
    const description = text(client.description)
    const used = formatTimestamp(client.lastUsedAt)

    card.append(cardCover({
      tag: isPublic ? `${t.typePublicShort} · PKCE` : t.typeConfidentialShort,
      isPublic,
      status: used || t.footNever,
      statusTitle: used ? t.footUsed(used) : t.footNever,
      live: isRecent(client.lastUsedAt),
      watermark: cardWatermark(client.icon, fullClientId, text(client.name))
    }))

    const body = document.createElement('div')
    body.className = 'sfp-card__body'
    const icon = document.createElement('span')
    icon.className = 'sfp-card__icon'
    icon.append(renderIcon(client.icon, fullClientId, text(client.name), client.clientType))
    body.append(icon)

    const title = document.createElement('h3')
    title.className = 'sfp-card__title'
    title.textContent = text(client.name)
    title.title = text(client.name)
    body.append(title)

    // 公开 / 机密都占同一行徽章位：机密显示密钥已设置，公开显示无密钥，
    // 保证两类卡片的标题、说明、client_id 与权限 chips 垂直对齐。
    const badges = document.createElement('div')
    badges.className = 'sfp-card__badges'
    if (isPublic) {
      const noSecret = document.createElement('span')
      noSecret.className = 'sf-badge sf-badge--neutral sfp-badge--secret'
      noSecret.textContent = t.secretUnset
      noSecret.title = t.typePublicHint
      badges.append(noSecret)
    } else if (client.hasSecret !== false) {
      const secretBadge = document.createElement('span')
      secretBadge.className = 'sf-badge sf-badge--neutral sfp-badge--secret'
      secretBadge.textContent = t.secretSet
      secretBadge.title = t.secretSetHint
      badges.append(secretBadge)
    }
    body.append(badges)

    const desc = document.createElement('p')
    desc.className = 'sfp-card__desc' + (description ? '' : ' is-empty')
    desc.textContent = description || t.noDescription
    if (description) desc.title = description
    body.append(desc)

    const idLabel = document.createElement('p')
    idLabel.className = 'sfp-card__idlabel'
    idLabel.textContent = t.idLabel
    body.append(idLabel)

    const idLine = document.createElement('div')
    idLine.className = 'sfp-card__id'
    idLine.title = t.idHint
    const copy = copyIconButton(fullClientId)
    const idBox = document.createElement('div')
    idBox.className = 'sfp-card__idbox'
    idBox.append(idChip(fullClientId, copy), copy)
    idLine.append(idBox)
    body.append(idLine)

    body.append(scopeChips(scopeList))
    card.append(body)

    const foot = document.createElement('div')
    foot.className = 'sfp-card__foot'
    const uriText = uris.join(' · ')
    const uriLine = document.createElement('span')
    uriLine.className = 'sfp-card__used'
    uriLine.textContent = uriBrief(uris[0]) || '—'
    if (uriText) uriLine.title = uriText
    foot.append(uriLine)
    foot.append(button(t.edit, 'ghost', 'sm', (event) => openForm('edit', client, event.currentTarget)))
    foot.append(moreMenu([
      { label: t.editApp, onClick: (event) => openForm('edit', client, event.currentTarget) },
      { label: t.copyClientId, onClick: () => copyFeedback(fullClientId) },
      '|',
      { label: t.rotate, onClick: (event) => openConfirm('rotate', client, event.currentTarget) },
      { label: t.remove, danger: true, onClick: (event) => openConfirm('delete', client, event.currentTarget) }
    ]))
    card.append(foot)

    card.dataset.search = [
      client.name, fullClientId, client.description, uriText, scopeList.join(' ')
    ].map(text).join(' ').toLowerCase()
    return card
  }

  const renderApps = () => {
    appsBody.textContent = ''
    if (!canReadApps) {
      appsBody.append(emptyState(t.appsForbiddenTitle, t.appsForbiddenHint, false))
      applySearchFilter()
      return
    }
    if (clients.length === 0) {
      appsBody.append(emptyState(t.emptyTitle, t.emptyHint, canCreate))
      return
    }
    for (const client of clients) appsBody.append(clientCard(client))
    if (canCreate) {
      // 虚线「新建应用」占位卡：与琉璃卡同语言的第二个创建入口（空态里已有主按钮）。
      const tile = document.createElement('button')
      tile.type = 'button'
      tile.className = 'sfp-add-tile'
      const plus = document.createElement('span')
      plus.className = 'sfp-add-tile__plus'
      const label = document.createElement('span')
      label.textContent = t.create
      tile.append(plus, label)
      tile.addEventListener('click', (event) => openForm('create', null, event.currentTarget))
      appsBody.append(tile)
    }
    applyMoreIcons()
    applyCopyIcons()
    fillIconSlots()
    applySearchFilter()
  }

  const authCard = (authorization) => {
    const card = document.createElement('article')
    card.className = 'sfp-card'

    const name = text(authorization.clientName) || text(authorization.clientId)
    const clientId = text(authorization.clientId)
    const scopeList = Array.isArray(authorization.scopes) ? authorization.scopes : []
    const granted = formatTimestamp(authorization.updatedAt || authorization.createdAt)

    card.append(cardCover({
      tag: t.authTab,
      status: granted || '—',
      statusTitle: `${t.footGranted} ${granted || '—'}`,
      live: false,
      watermark: cardWatermark(authorization.icon, clientId, name)
    }))

    const body = document.createElement('div')
    body.className = 'sfp-card__body'
    const icon = document.createElement('span')
    icon.className = 'sfp-card__icon'
    icon.append(renderIcon(authorization.icon, clientId, name, 'public'))
    body.append(icon)

    const title = document.createElement('h3')
    title.className = 'sfp-card__title'
    title.textContent = name
    title.title = name
    body.append(title)

    const idLabel = document.createElement('p')
    idLabel.className = 'sfp-card__idlabel'
    idLabel.textContent = t.idLabel
    body.append(idLabel)

    const idLine = document.createElement('div')
    idLine.className = 'sfp-card__id'
    idLine.title = t.idHint
    const idBox = document.createElement('div')
    idBox.className = 'sfp-card__idbox'
    idBox.append(idChip(clientId))
    idLine.append(idBox)
    body.append(idLine)

    body.append(scopeChips(scopeList))
    card.append(body)

    const foot = document.createElement('div')
    foot.className = 'sfp-card__foot'
    const grantedLine = document.createElement('span')
    grantedLine.className = 'sfp-card__used'
    grantedLine.textContent = `${t.footGranted} ${granted || '—'}`
    foot.append(grantedLine)
    foot.append(button(t.revoke, 'danger-ghost', 'sm', (event) => openRevokeConfirm(authorization, event.currentTarget)))
    card.append(foot)

    card.dataset.search = [name, clientId, scopeList.join(' ')].map(text).join(' ').toLowerCase()
    return card
  }

  const renderAuthorizations = () => {
    authBody.textContent = ''
    if (authorizations.length === 0) {
      authBody.append(emptyState(t.authEmptyTitle, t.authEmptyHint, false))
      return
    }
    for (const authorization of authorizations) authBody.append(authCard(authorization))
    fillIconSlots()
    applySearchFilter()
  }

  const renderHeadActions = () => {
    if (!headActions) return
    headActions.textContent = ''
    const refresh = button(t.refresh, 'secondary', 'md', () => manualRefresh())
    const refreshIcon = iconElement('lucide:refresh-cw', 16)
    if (refreshIcon) refresh.prepend(wrapIcon(refreshIcon))
    const create = button(t.create, 'primary', 'md', (event) => openForm('create', null, event.currentTarget))
    const createIcon = iconElement('lucide:plus', 16)
    if (createIcon) create.prepend(wrapIcon(createIcon))
    setHidden(create, !canCreate)
    headActions.append(refresh, create)
  }

  const renderCounts = () => {
    nodes.countApps.textContent = String(clients.length)
    nodes.countAuths.textContent = String(authorizations.length)
    nodes.appsDesc.textContent = canReadApps ? t.appsHint(clients.length, storeSource) : t.appsForbiddenDesc
    nodes.authDesc.textContent = t.authHint(authorizations.length)
  }

  // 图标晚于数据到达时就地补 icon span，不重建按钮（重建会闪一帧并丢焦点）。
  // 顺序与 renderHeadActions 一致：[刷新, 新建应用]。
  const applyHeadActionIcons = () => {
    if (!headActions) return
    const specs = [['lucide:refresh-cw', 16], ['lucide:plus', 16]]
    const buttons = headActions.querySelectorAll('button')
    buttons.forEach((element, index) => {
      const spec = specs[index]
      if (!spec || element.querySelector('.sf-button__icon')) return
      const svg = iconElement(spec[0], spec[1])
      if (svg) element.prepend(wrapIcon(svg))
    })
  }

  const openModal = (trigger) => {
    lastTrigger = trigger || null
    setHidden(modal, false)
    window.requestAnimationFrame(() => {
      const focusable = views[view].querySelector('input:not([type="hidden"]), textarea, button')
      if (focusable) focusable.focus()
      else modalSubmit.focus()
    })
  }

  // 客户端类型 = 隐藏 input（#sfp-type）+ radio 卡片；读写都走 hidden，卡片只做呈现。
  const typeRadios = Array.from(root.querySelectorAll('input[name="sfp-type-radio"]'))
  const syncTypeCards = () => {
    for (const radio of typeRadios) radio.checked = radio.value === fields.clientType.value
  }
  const onTypeCardsChange = (event) => {
    if (event.target.name !== 'sfp-type-radio') return
    fields.clientType.value = event.target.value
    renderLogoPreview()
  }

  const closeModal = () => {
    setHidden(modal, true)
    setBusy(false)
    if (lastTrigger && document.contains(lastTrigger)) lastTrigger.focus()
    lastTrigger = null
  }

  const showView = (next) => {
    view = next
    for (const [key, element] of Object.entries(views)) setHidden(element, key !== next)
    setHidden(footCancel, next === 'secret')
    setHidden(modalSubmit, false)
    // 确认类操作（删除/重置/撤销）用窄规格弹窗，对齐 demo 01-console 的 confirm。
    modal.classList.toggle('sfp-modal--narrow', next === 'confirm')
    if (next === 'form') {
      modalSub.textContent = t.formSub
      setHidden(modalSub, false)
      modalSubmit.textContent = mode === 'create' ? t.submitCreate : t.submitSave
      modalSubmit.className = 'sf-button sf-button--md sf-button--primary'
    } else {
      setHidden(modalSub, true)
      if (next === 'secret') {
        modalSubmit.textContent = t.secretDone
        modalSubmit.className = 'sf-button sf-button--md sf-button--primary'
      } else if (mode === 'delete' || mode === 'revoke') {
        modalSubmit.textContent = mode === 'delete' ? t.deleteConfirm : t.revokeConfirm
        modalSubmit.className = 'sf-button sf-button--md sf-button--danger'
      } else {
        modalSubmit.textContent = t.rotateConfirm
        modalSubmit.className = 'sf-button sf-button--md sf-button--primary'
      }
    }
  }

  const openForm = (nextMode, client, trigger) => {
    mode = nextMode
    activeClient = client || null
    modalTitle.textContent = nextMode === 'create' ? t.createTitle : t.editTitle
    formEl.reset()
    if (client) {
      fields.name.value = text(client.name)
      fields.description.value = text(client.description)
      fields.clientType.value = client.clientType === 'public' ? 'public' : 'confidential'
      fields.redirectUris.value = (client.redirectUris || []).join('\n')
    }
    syncTypeCards()
    fields.logoUrl.value = text(client && client.logoUrl)
    resetIconChoice(client)
    const clientScopes = client ? (client.scopes || []).map(text) : []
    pickedScopes = new Set(clientScopes.filter((scope) => allowedScopes.includes(scope)))
    pickedScopes.add('profile')
    customScopes = clientScopes.filter((scope) => !allowedScopes.includes(scope))
    renderScopeChips()
    showView('form')
    openModal(trigger)
  }

  const openConfirm = (nextMode, client, trigger) => {
    mode = nextMode
    activeClient = client
    activeAuthorization = null
    const isDelete = nextMode === 'delete'
    modalTitle.textContent = isDelete ? t.deleteTitle : t.rotateTitle
    confirmTitle.textContent = `${isDelete ? t.deleteTitle : t.rotateTitle}：${text(client.name)}`
    confirmHint.textContent = isDelete ? t.deleteHint : t.rotateHint
    showView('confirm')
    openModal(trigger)
  }

  const openRevokeConfirm = (authorization, trigger) => {
    mode = 'revoke'
    activeAuthorization = authorization
    activeClient = null
    modalTitle.textContent = t.revokeTitle
    confirmTitle.textContent = `${t.revokeTitle}：${text(authorization.clientName) || text(authorization.clientId)}`
    confirmHint.textContent = t.revokeHint
    showView('confirm')
    openModal(trigger)
  }

  // 一次性凭据弹窗：每行给出中文名 + 可公开/保密标签 + 一句话说明，
  // 免得把 client_id 当成需要保密的密钥（两者不是公私钥对）。
  const openSecret = (clientId, clientSecret, title) => {
    modalTitle.textContent = title || t.secretTitle
    secretPairs.textContent = ''
    const entries = [
      [t.secretLabelId, t.idTagPublic, 'sfp-tag--public', clientId, t.secretIdNote],
      [t.secretLabelKey, t.secretTagPrivate, 'sfp-tag--private', clientSecret, t.secretKeyNote]
    ]
    for (const [label, tagText, tagClass, value, note] of entries) {
      const row = document.createElement('div')
      row.className = 'sfp-pair'
      const dt = document.createElement('dt')
      dt.append(document.createTextNode(label))
      const tag = document.createElement('span')
      tag.className = `sfp-tag ${tagClass}`
      tag.textContent = tagText
      dt.append(tag)
      const dd = document.createElement('dd')
      dd.append(idChip(value), copyButton(value))
      const hint = document.createElement('p')
      hint.className = 'sfp-pair__note'
      hint.textContent = note
      row.append(dt, dd, hint)
      secretPairs.append(row)
    }
    const summary = document.createElement('div')
    summary.className = 'sfp-pair__note sfp-pair__note--summary'
    summary.textContent = t.secretPairsNote
    secretPairs.append(summary)
    showView('secret')
    setBusy(false)
    window.requestAnimationFrame(() => modalSubmit.focus())
  }

  const load = async () => {
    setBusy(true)
    try {
      const data = await api('/clients')
      clients = Array.isArray(data.clients) ? data.clients : []
      allowedScopes = Array.isArray(data.allowedScopes) ? data.allowedScopes.map(text).filter(Boolean) : []
      scopeCatalog = Array.isArray(data.scopeCatalog) ? data.scopeCatalog : []
      storeSource = text(data.source) || 'database'
      canCreate = data.canCreate !== false
      setHidden(gate, canCreate)
      renderHeadActions()
      renderApps()
      renderCounts()
      appsLoaded = true
    } catch (error) {
      // 后端 routeFailure 会把 reason 的 `oauth2.` 前缀剥掉，所以线上看到的是
      // `client_list_forbidden`；两种形态都认，避免宿主信封变化时静默退回错误提示。
      if (error && (error.code === 'client_list_forbidden' || error.code === 'oauth2.client_list_forbidden')) {
        // 无创建权限的登录用户：页面仍然可用，只是没有「我的应用」这一层。
        canReadApps = false
        canCreate = false
        clients = []
        setHidden(gate, true)
        renderHeadActions()
        renderApps()
        renderCounts()
        appsLoaded = true
        // 没有显式深链时默认落到「已授权」——那才是这些用户来这一页的目的。
        if (!window.location.hash) activateTab('authorized')
        return
      }
      if (clients.length === 0) appsBody.textContent = ''
      notify(t.loadAppsFailed, { kind: 'error', description: error.message })
    } finally {
      setBusy(false)
    }
  }

  const loadAuthorizations = async () => {
    try {
      const data = await api('/authorizations')
      authorizations = Array.isArray(data.authorizations) ? data.authorizations : []
      renderAuthorizations()
      renderCounts()
      authsLoaded = true
    } catch (error) {
      if (authorizations.length === 0) authBody.textContent = ''
      notify(t.loadAuthsFailed, { kind: 'error', description: error.message })
    }
  }

  const refreshAll = async () => {
    await load()
    await loadAuthorizations()
  }

  // 只有用户主动点「刷新」才清掉降级状态条；正常路径的提示是 Toast，会自己消失。
  const manualRefresh = async () => {
    clearStatus()
    await refreshAll()
  }

  const collectForm = () => {
    const name = fields.name.value.trim()
    if (!name) return { error: t.nameRequired }
    const redirectUris = fields.redirectUris.value
      .split('\n')
      .map((item) => item.trim())
      .filter(Boolean)
    if (redirectUris.length === 0) return { error: t.redirectRequired }
    for (const uri of redirectUris) {
      if (!validRedirectURI(uri)) return { error: t.redirectInvalid(uri) }
    }
    const payload = {
      name,
      description: fields.description.value.trim(),
      clientType: fields.clientType.value,
      redirectUris
    }
    // 上传与 URL 互斥：一次保存只提交一个来源；「没动图标」则两个字段都不提交，
    // 服务端据此保留原图（否则每次编辑都会重 claim 一次并撞修订号）。
    if (iconChoice.kind === 'attachment') {
      payload.attachmentPublicId = iconChoice.publicId
      payload.attachmentRevision = iconChoice.revision
      payload.logoUrl = ''
    } else if (iconChoice.kind === 'url') {
      const logoUrl = fields.logoUrl.value.trim()
      if (!validLogoURL(logoUrl)) return { error: t.logoInvalid(logoUrl) }
      payload.logoUrl = logoUrl
      payload.attachmentPublicId = ''
    } else if (iconChoice.kind === 'generated') {
      payload.logoUrl = ''
      payload.attachmentPublicId = ''
    }
    // profile 恒在（必选）：提交值 = profile + 勾选的额外范围。
    const picked = allowedScopes.filter((scope) => pickedScopes.has(scope))
    if (!picked.includes('profile')) picked.unshift('profile')
    payload.scopes = picked
    return { payload }
  }

  const submitForm = async () => {
    const { payload, error } = collectForm()
    if (error) {
      notify(error, { kind: 'error' })
      return
    }
    setBusy(true)
    // Host 只在带 Idempotency-Key 时才签发 claim/release 的 actor 委派。
    const idempotencyKey = newIdempotencyKey()
    try {
      if (mode === 'create') {
        const created = await api('/clients/create', { method: 'POST', body: payload, idempotencyKey })
        await load()
        notify(`${t.created}：${payload.name}`)
        if (created && created.clientSecret) {
          openSecret(text(created.client && created.client.clientId), text(created.clientSecret), t.created)
        } else {
          closeModal()
        }
      } else {
        const updated = await api('/clients/update', {
          method: 'POST',
          body: { ...payload, clientId: text(activeClient && activeClient.clientId) },
          idempotencyKey
        })
        await load()
        notify(`${t.saved}：${payload.name}`)
        if (updated && updated.clientSecret) {
          openSecret(text(updated.client && updated.client.clientId), text(updated.clientSecret), t.saved)
        } else {
          closeModal()
        }
      }
    } catch (requestError) {
      setBusy(false)
      // 图标类失败已经在表单内就地说明（并展开 URL 入口），不必再弹一条 Toast。
      if (!handleIconFailure(requestError)) {
        notify(t.requestFailed, { kind: 'error', description: requestError.message })
      }
    }
  }

  const submitConfirm = async () => {
    setBusy(true)
    try {
      if (mode === 'rotate') {
        const clientId = text(activeClient && activeClient.clientId)
        const result = await api('/clients/rotate-secret', { method: 'POST', body: { clientId } })
        await load()
        notify(`${t.rotated}：${clientId}`)
        if (result && result.clientSecret) {
          openSecret(clientId, text(result.clientSecret), t.rotated)
        } else {
          closeModal()
        }
      } else if (mode === 'delete') {
        const clientId = text(activeClient && activeClient.clientId)
        await api('/clients/delete', { method: 'POST', body: { clientId }, idempotencyKey: newIdempotencyKey() })
        await refreshAll()
        notify(`${t.deleted}：${clientId}`)
        closeModal()
      } else {
        const clientId = text(activeAuthorization && activeAuthorization.clientId)
        await api('/authorizations/revoke', { method: 'POST', body: { clientId } })
        await refreshAll()
        notify(`${t.revoked}：${clientId}`)
        closeModal()
      }
    } catch (error) {
      setBusy(false)
      closeModal()
      notify(t.requestFailed, { kind: 'error', description: error.message })
    }
  }

  const onRootClick = (event) => {
    const action = event.target.closest('[data-action]')
    if (!action) return
    if (action.dataset.action === 'close-modal') closeModal()
  }
  root.addEventListener('click', onRootClick)

  const onFormSubmit = (event) => {
    event.preventDefault()
    submitForm()
  }
  formEl.addEventListener('submit', onFormSubmit)

  const onModalSubmit = () => {
    if (busy) return
    if (view === 'form') submitForm()
    else if (view === 'confirm') submitConfirm()
    else closeModal()
  }
  modalSubmit.addEventListener('click', onModalSubmit)

  typeCards.addEventListener('change', onTypeCardsChange)
  fields.logoUrl.addEventListener('input', onLogoURLInput)
  fields.name.addEventListener('input', renderLogoPreview)
  if (fields.logoFile) fields.logoFile.addEventListener('change', onIconFileChange)
  if (iconClear) iconClear.addEventListener('click', onClearIcon)

  const onKeydown = (event) => {
    if (event.key === 'Escape' && !isHidden(modal)) {
      event.stopPropagation()
      closeModal()
    }
  }
  document.addEventListener('keydown', onKeydown)

  const onTabsClick = (event) => {
    const anchor = event.target.closest('.sfp-tab')
    if (!anchor) return
    event.preventDefault()
    activateTab(anchor.getAttribute('href').replace('#', ''), { focus: true })
  }
  tabsNav?.addEventListener('click', onTabsClick)

  const onTabsKeydown = (event) => {
    const keys = ['ArrowLeft', 'ArrowRight', 'Home', 'End']
    if (!keys.includes(event.key)) return
    const index = tabs.findIndex((item) => item.id === activeTab)
    const last = tabs.length - 1
    const nextIndex = event.key === 'Home' ? 0
      : event.key === 'End' ? last
        : event.key === 'ArrowRight' ? (index + 1 > last ? 0 : index + 1)
          : (index - 1 < 0 ? last : index - 1)
    event.preventDefault()
    activateTab(tabs[nextIndex].id, { focus: true })
  }
  tabsNav?.addEventListener('keydown', onTabsKeydown)

  const onHashChange = () => {
    const id = window.location.hash.replace('#', '')
    if (tabs.some((item) => item.id === id)) activateTab(id, { syncHash: false })
  }
  window.addEventListener('hashchange', onHashChange)

  // ---------- 工具行：⌘K 命令栏搜索（纯前端过滤）+ 接入信息 popover + 菜单外点关闭 ----------
  // 宿主模板净化器会剥掉 label / input / button，只保留它们的子节点，所以这两个控件
  // 在挂载时构建（与 head actions、gate 同一条路子），模板里只留空容器当挂载点。
  const cmdbarHost = page.querySelector('.sfp-cmdbar')
  let searchInput = null
  if (cmdbarHost) {
    cmdbarHost.textContent = ''
    const searchIcon = document.createElement('span')
    searchIcon.className = 'sfp-cmdbar__icon'
    searchIcon.setAttribute('aria-hidden', 'true')
    const input = document.createElement('input')
    input.className = 'sfp-cmdbar__input'
    input.type = 'search'
    input.autocomplete = 'off'
    input.placeholder = t.searchPlaceholder
    input.setAttribute('aria-label', t.searchPlaceholder)
    const kbd = document.createElement('span')
    kbd.className = 'sfp-cmdbar__kbd'
    kbd.setAttribute('aria-hidden', 'true')
    kbd.textContent = '⌘K'
    cmdbarHost.append(searchIcon, input, kbd)
    searchInput = input
  }

  const apiHost = page.querySelector('.sfp-apiinfo')
  let apiToggle = null
  let apiPop = null
  if (apiHost) {
    apiHost.textContent = ''
    const toggle = document.createElement('button')
    toggle.type = 'button'
    toggle.className = 'sfp-api-toggle'
    const slot = document.createElement('span')
    slot.className = 'sfp-icon-slot'
    slot.setAttribute('aria-hidden', 'true')
    toggle.append(slot, document.createTextNode(t.apiInfo))
    const pop = document.createElement('div')
    pop.className = 'sfp-pop sfp-api-pop sfp-hidden'
    pop.setAttribute('role', 'dialog')
    pop.setAttribute('aria-label', t.apiInfo)
    const title = document.createElement('p')
    title.className = 'sfp-pop__title'
    title.textContent = t.apiInfoTitle
    const list = document.createElement('dl')
    list.className = 'sfp-pop__eps'
    for (const [key, value] of [['授权', '/api/oauth/authorize'], ['令牌', '/api/oauth/token'], ['资料', '/api/oauth/userinfo']]) {
      const row = document.createElement('div')
      const dt = document.createElement('dt')
      dt.textContent = key
      const dd = document.createElement('dd')
      dd.textContent = value
      row.append(dt, dd)
      list.append(row)
    }
    const note = document.createElement('p')
    note.className = 'sfp-pop__note'
    note.textContent = t.apiInfoNote
    pop.append(title, list, note)
    apiHost.append(toggle, pop)
    apiToggle = toggle
    apiPop = pop
  }

  // 搜索只在当前面板内过滤卡片（数据已全量在内存，不发请求）；
  // 无结果时在栅格里插一条空态，清空即恢复。
  const currentBody = () => (activeTab === 'authorized' ? authBody : appsBody)
  const applySearchFilter = () => {
    if (!searchInput) return
    const query = searchInput.value.trim().toLowerCase()
    const body = currentBody()
    let shown = 0
    body.querySelectorAll(':scope > .sfp-card').forEach((card) => {
      const hit = !query || text(card.dataset.search).includes(query) || card.textContent.toLowerCase().includes(query)
      setHidden(card, !hit)
      if (hit) shown += 1
    })
    const existing = body.querySelector(':scope > .sfp-search-empty')
    if (existing) existing.remove()
    if (query && shown === 0 && body.childElementCount > 0) {
      const wrap = document.createElement('div')
      wrap.className = 'sf-empty-state sfp-search-empty'
      const title = document.createElement('h3')
      title.className = 'sf-empty-state__title'
      title.textContent = t.searchNoMatchTitle
      const hint = document.createElement('p')
      hint.className = 'sf-empty-state__description'
      hint.textContent = t.searchNoMatchHint
      wrap.append(title, hint)
      body.append(wrap)
    }
  }
  const onSearchInput = () => applySearchFilter()
  searchInput?.addEventListener('input', onSearchInput)

  const onSearchKeydown = (event) => {
    // ⌘K / Ctrl+K 聚焦搜索；搜索框内 Esc 清空退出
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault()
      searchInput?.focus()
      searchInput?.select()
      return
    }
    if (event.key === 'Escape' && event.target === searchInput) {
      searchInput.value = ''
      applySearchFilter()
      searchInput.blur()
    }
  }
  document.addEventListener('keydown', onSearchKeydown)

  const closeApiPop = () => setHidden(apiPop, true)
  const onApiToggle = (event) => {
    event.stopPropagation()
    const open = isHidden(apiPop)
    closeAllMenus()
    setHidden(apiPop, !open)
  }
  apiToggle?.addEventListener('click', onApiToggle)

  const onDocumentClick = () => {
    closeAllMenus()
    closeApiPop()
  }
  document.addEventListener('click', onDocumentClick)
  apiPop?.addEventListener('click', (event) => event.stopPropagation())

  // 首帧就该落在正确的面板上（深链 #authorized 直达）。
  activateTab(window.location.hash.replace('#', '') || 'apps', { syncHash: false })
  ensureIcons(['lucide:refresh-cw', 'lucide:plus', 'lucide:key-round', 'lucide:copy', 'lucide:search', 'lucide:info', 'lucide:check', 'lucide:ellipsis']).then(() => {
    applyHeadActionIcons()
    applyCopyIcons()
    applyMoreIcons()
    fillIconSlots()
    // 数据未到前不动列表（保持模板骨架）；空态是唯一带图标的形态，数据已在
    // 且列表为空时才值得重绘一次。非空列表的行不含图标，重绘纯属闪烁。
    if (appsLoaded && clients.length === 0) renderApps()
    if (authsLoaded && authorizations.length === 0) renderAuthorizations()
  })
  renderHeadActions()
  refreshAll()

  return () => {
    root.removeEventListener('click', onRootClick)
    formEl.removeEventListener('submit', onFormSubmit)
    typeCards.removeEventListener('change', onTypeCardsChange)
    fields.logoUrl.removeEventListener('input', onLogoURLInput)
    fields.name.removeEventListener('input', renderLogoPreview)
    if (fields.logoFile) fields.logoFile.removeEventListener('change', onIconFileChange)
    if (iconClear) iconClear.removeEventListener('click', onClearIcon)
    modalSubmit.removeEventListener('click', onModalSubmit)
    document.removeEventListener('keydown', onKeydown)
    tabsNav?.removeEventListener('click', onTabsClick)
    tabsNav?.removeEventListener('keydown', onTabsKeydown)
    window.removeEventListener('hashchange', onHashChange)
    searchInput?.removeEventListener('input', onSearchInput)
    document.removeEventListener('keydown', onSearchKeydown)
    document.removeEventListener('click', onDocumentClick)
    apiToggle?.removeEventListener('click', onApiToggle)
    gate.remove()
    root.remove()
  }
}

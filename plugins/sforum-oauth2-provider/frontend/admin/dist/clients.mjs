export const apiVersion = 1

// OAuth2 应用管理面板（Admin Micro-frontend API v1，无构建步骤的纯 ESM）。
// 约束与约定：
// - 只调用插件自身的 /api/oauth 路由；unsafe 方法按 Host 的 double-submit 规则带 X-Csrf-Token。
// - 主题沿用 Host 管理端变量（--bg-admin-card/--border-admin/--text-admin-main/--sf-accent…），
//   暗色由祖先 .dark 类驱动，因此外观切换无需重新挂载。
// - 插件微前端拿不到 iconify 运行时（i-tabler-* 类不生效），因此统一使用文字按钮，
//   不使用 emoji 或自绘图标。

const API = '/api/oauth'
// 图标上传直连 Core 的附件接口：字节交给宿主的附件体系，插件只拿 publicId 去 claim。
const CORE_API = '/api/v1'
// 前端前置校验只做体验（权威判定在 Host）：PNG / JPEG / WebP，≤512 KiB。
const ICON_MAX_BYTES = 512 * 1024
const ICON_TYPES = ['image/png', 'image/jpeg', 'image/webp']

const MESSAGES = {
  'zh-CN': {
    sectionTitle: '应用列表',
    sectionHint: (count, source) => `共 ${count} 个应用 · 存储：${source}`,
    refresh: '刷新',
    create: '新建应用',
    docs: '接入文档',
    colApp: '应用',
    colType: '类型',
    colRedirect: '回调地址',
    colScopes: '权限',
    colOwner: '归属',
    colUsed: '最近使用',
    colActions: '操作',
    edit: '编辑',
    rotate: '重置密钥',
    remove: '删除',
    copy: '复制',
    copied: '已复制',
    typeConfidential: '机密',
    typePublic: '公开',
    never: '从未使用',
    emptyTitle: '还没有 OAuth2 应用',
    emptyHint: '创建应用后，第三方服务即可通过授权码 + PKCE 读取用户资料。',
    createTitle: '新建 OAuth2 应用',
    editTitle: '编辑 OAuth2 应用',
    fieldName: '应用名称',
    fieldNameHint: '展示在用户授权确认页上，建议使用真实服务名。',
    fieldDescription: '应用说明',
    fieldDescriptionHint: '可选，显示在确认页标题下方。',
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
    typeConfidentialHint: '机密客户端持有 client_secret，适合有服务端的应用。',
    typePublicHint: '公开客户端不使用密钥，必须使用 PKCE，适合纯前端应用。',
    fieldRedirect: '回调地址',
    fieldRedirectHint: '每行一个，必须是 https；本地开发可用 http://127.0.0.1 或 http://localhost。',
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
    cancel: '取消',
    submitCreate: '创建',
    submitSave: '保存',
    submitting: '处理中…',
    idLabel: '客户端 ID',
    idTagPublic: '可公开',
    idHint: '客户端 ID 是公开标识，可以对外展示；需要保密的是客户端密钥 client_secret。',
    secretTitle: '请立即保存客户端密钥（client_secret）',
    secretHint: '客户端密钥只显示这一次；关闭后无法再次查看，遗失时只能重置。',
    secretLabelId: '客户端 ID',
    secretLabelKey: '客户端密钥',
    secretTagPrivate: '保密',
    secretIdNote: '公开标识：可以写进应用配置、也可以被用户看到，泄漏没有风险。',
    secretKeyNote: '保密凭据：只放在可信服务端，不要提交到代码仓库、也不要下发到浏览器。',
    secretPairsNote: '两者不是公私钥对：客户端 ID 相当于用户名（公开），客户端密钥相当于密码（保密），没有需要分发的公钥。',
    secretDone: '我已保存',
    rotateTitle: '重置密钥',
    rotateHint: '重置后旧密钥立即失效，正在使用它的应用需要改成新密钥。',
    rotateConfirm: '确认重置',
    deleteTitle: '删除应用',
    deleteHint: '删除会同时撤销该应用的全部令牌与用户授权，且无法恢复。',
    deleteConfirm: '确认删除',
    created: '应用已创建',
    saved: '应用已保存',
    rotated: '密钥已重置',
    deleted: '应用已删除',
    requestFailed: '请求失败',
    close: '关闭',
    redirectRequired: '至少填写一个回调地址。',
    nameRequired: '请填写应用名称。',
    redirectInvalid: (value) => `回调地址不合法：${value}（需 https，或本地 http://127.0.0.1 / http://localhost，且不带 #fragment）`
  },
  'en-US': {
    sectionTitle: 'Applications',
    sectionHint: (count, source) => `${count} app${count === 1 ? '' : 's'} · storage: ${source}`,
    refresh: 'Refresh',
    create: 'New application',
    docs: 'Integration guide',
    colApp: 'Application',
    colType: 'Type',
    colRedirect: 'Redirect URIs',
    colScopes: 'Scopes',
    colOwner: 'Owner',
    colUsed: 'Last used',
    colActions: 'Actions',
    edit: 'Edit',
    rotate: 'Reset secret',
    remove: 'Delete',
    copy: 'Copy',
    copied: 'Copied',
    typeConfidential: 'Confidential',
    typePublic: 'Public',
    never: 'Never used',
    emptyTitle: 'No OAuth2 applications yet',
    emptyHint: 'Create an app so third-party services can read profile data through authorization code + PKCE.',
    createTitle: 'New OAuth2 application',
    editTitle: 'Edit OAuth2 application',
    fieldName: 'Application name',
    fieldNameHint: 'Shown on the user consent page; use the real service name.',
    fieldDescription: 'Description',
    fieldDescriptionHint: 'Optional; displayed under the consent page title.',
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
    typeConfidentialHint: 'A confidential client holds a client_secret and fits server-side apps.',
    typePublicHint: 'A public client has no secret and must use PKCE; fits browser-only apps.',
    fieldRedirect: 'Redirect URIs',
    fieldRedirectHint: 'One per line, https required; http://127.0.0.1 or http://localhost is allowed for local development.',
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
    cancel: 'Cancel',
    submitCreate: 'Create',
    submitSave: 'Save',
    submitting: 'Working…',
    idLabel: 'Client ID',
    idTagPublic: 'Shareable',
    idHint: 'The client ID is a public identifier and is safe to show; the client_secret is the part that must stay private.',
    secretTitle: 'Store the client secret (client_secret) now',
    secretHint: 'The client secret is shown once; it cannot be viewed again and must be reset if lost.',
    secretLabelId: 'Client ID',
    secretLabelKey: 'Client secret',
    secretTagPrivate: 'Private',
    secretIdNote: 'Public identifier: safe in app configuration and visible to users.',
    secretKeyNote: 'Private credential: keep it on a trusted server — never in a repository or shipped to a browser.',
    secretPairsNote: 'This is not a public/private key pair: the client ID is like a username (public) and the client secret like a password (private); there is no public key to distribute.',
    secretDone: 'I saved it',
    rotateTitle: 'Reset client secret',
    rotateHint: 'The old secret stops working immediately; update it in the app that uses it.',
    rotateConfirm: 'Reset secret',
    deleteTitle: 'Delete application',
    deleteHint: 'Deleting revokes every token and user grant for this app, and cannot be undone.',
    deleteConfirm: 'Delete',
    created: 'Application created',
    saved: 'Application saved',
    rotated: 'Secret reset',
    deleted: 'Application deleted',
    requestFailed: 'Request failed',
    close: 'Close',
    redirectRequired: 'Add at least one redirect URI.',
    nameRequired: 'Application name is required.',
    redirectInvalid: (value) => `Invalid redirect URI: ${value} (use https, or http://127.0.0.1 / http://localhost locally, without a #fragment)`
  }
}

// 保存/删除请求必须带 Idempotency-Key：Host 只在它存在时才签发 Host Command 的
// actor 委派（claim / release），缺了它命令永远调不通。
function newIdempotencyKey() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `sfo-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
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
  try {
    await navigator.clipboard.writeText(value)
    return true
  } catch (error) {
    // 非安全上下文或权限被拒时的回退路径。
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

export function mount(target, bridge) {
  const locale = text(bridge && bridge.locale).startsWith('en') ? 'en-US' : 'zh-CN'
  const t = MESSAGES[locale]
  const toast = (title, description, kind) => {
    if (bridge && typeof bridge.toast === 'function') {
      bridge.toast({ title, description, kind: kind || 'success' })
    }
  }

  const root = document.createElement('div')
  root.className = 'sfo-page'
  root.innerHTML = `
    <section class="sfo-section">
      <header class="sfo-section__header">
        <div class="sfo-section__heading">
          <h2 class="sfo-section__title">${t.sectionTitle}</h2>
          <p class="sfo-section__hint" data-role="hint"></p>
        </div>
        <div class="sfo-section__actions">
          <a class="sfo-btn" href="/docs/oauth2-provider" target="_blank" rel="noopener noreferrer">${t.docs}</a>
          <button type="button" class="sfo-btn" data-action="refresh">${t.refresh}</button>
          <button type="button" class="sfo-btn sfo-btn--primary" data-action="create">${t.create}</button>
        </div>
      </header>
      <div class="sfo-section__body">
        <p class="sfo-status" data-role="status" hidden></p>
        <div class="sfo-table-wrap" data-role="table-wrap" hidden>
          <table class="sfo-table">
            <thead>
              <tr>
                <th scope="col">${t.colApp}</th>
                <th scope="col">${t.colType}</th>
                <th scope="col">${t.colRedirect}</th>
                <th scope="col">${t.colScopes}</th>
                <th scope="col">${t.colOwner}</th>
                <th scope="col">${t.colUsed}</th>
                <th scope="col" class="sfo-cell--end">${t.colActions}</th>
              </tr>
            </thead>
            <tbody data-role="rows"></tbody>
          </table>
        </div>
        <div class="sfo-empty" data-role="empty" hidden>
          <p class="sfo-empty__title">${t.emptyTitle}</p>
          <p class="sfo-empty__desc">${t.emptyHint}</p>
          <button type="button" class="sfo-btn sfo-btn--primary" data-action="create">${t.create}</button>
        </div>
      </div>
    </section>

    <div class="sfo-modal" data-role="modal" hidden>
      <div class="sfo-modal__backdrop" data-action="close-modal"></div>
      <div class="sfo-modal__dialog" role="dialog" aria-modal="true" aria-labelledby="sfo-modal-title">
        <header class="sfo-modal__header">
          <h3 class="sfo-modal__title" id="sfo-modal-title" data-role="modal-title"></h3>
          <button type="button" class="sfo-btn sfo-btn--sm" data-action="close-modal" data-role="static" aria-label="${t.close}" title="${t.close}">${t.close}</button>
        </header>

        <div class="sfo-modal__body" data-view="form">
          <form class="sfo-form" data-role="form" novalidate>
            <div class="sfo-field">
              <label class="sfo-field__label" for="sfo-name">${t.fieldName}</label>
              <p class="sfo-field__hint">${t.fieldNameHint}</p>
              <input class="sfo-input" id="sfo-name" name="name" maxlength="120" autocomplete="off" required>
            </div>
            <div class="sfo-field">
              <label class="sfo-field__label" for="sfo-description">${t.fieldDescription}</label>
              <p class="sfo-field__hint">${t.fieldDescriptionHint}</p>
              <input class="sfo-input" id="sfo-description" name="description" maxlength="500" autocomplete="off">
            </div>
            <div class="sfo-field">
              <span class="sfo-field__label">${t.fieldIcon}</span>
              <p class="sfo-field__hint">${t.fieldIconHint}</p>
              <div class="sfo-logo-row">
                <span class="sfo-avatar sfo-avatar--preview" data-role="logo-preview" aria-hidden="true"></span>
                <div class="sfo-logo-actions">
                  <label class="sfo-btn sfo-btn--sm sfo-file-pick">
                    <span>${t.iconUpload}</span>
                    <input type="file" id="sfo-logo-file" accept="image/png,image/jpeg,image/webp" data-role="logo-file" hidden>
                  </label>
                  <button type="button" class="sfo-btn sfo-btn--sm" data-action="clear-icon" data-role="icon-clear" hidden>${t.iconRemove}</button>
                  <p class="sfo-field__hint sfo-icon-status" data-role="icon-status" aria-live="polite"></p>
                </div>
              </div>
              <details class="sfo-logo-url" data-role="logo-url">
                <summary class="sfo-logo-url__summary">${t.iconUseURL}</summary>
                <label class="sfo-field__label" for="sfo-logo">${t.fieldLogo}</label>
                <input class="sfo-input" id="sfo-logo" name="logoUrl" maxlength="2048" autocomplete="off" spellcheck="false" placeholder="https://example.com/logo.png">
                <p class="sfo-field__hint">${t.fieldLogoHint}</p>
              </details>
            </div>
            <div class="sfo-field">
              <label class="sfo-field__label" for="sfo-type">${t.fieldType}</label>
              <select class="sfo-input" id="sfo-type" name="clientType">
                <option value="confidential">${t.typeConfidential}</option>
                <option value="public">${t.typePublic}</option>
              </select>
              <p class="sfo-field__hint" data-role="type-hint"></p>
            </div>
            <div class="sfo-field">
              <label class="sfo-field__label" for="sfo-redirect">${t.fieldRedirect}</label>
              <p class="sfo-field__hint">${t.fieldRedirectHint}</p>
              <textarea class="sfo-input sfo-textarea" id="sfo-redirect" name="redirectUris" rows="3" spellcheck="false"></textarea>
            </div>
            <div class="sfo-field">
              <span class="sfo-field__label">${t.fieldScopes}</span>
              <p class="sfo-field__hint">${t.fieldScopesHint}</p>
              <div class="sfo-chips" data-role="scopes-chips" role="group" aria-label="${t.fieldScopes}"></div>
              <p class="sfo-field__hint sfo-scopes-note" data-role="scopes-note"></p>
            </div>
            <p class="sfo-form__error" data-role="form-error" role="alert" hidden></p>
          </form>
        </div>

        <div class="sfo-modal__body" data-view="secret" hidden>
          <div class="sfo-callout sfo-callout--warning">
            <p class="sfo-callout__title">${t.secretTitle}</p>
            <p class="sfo-callout__body">${t.secretHint}</p>
          </div>
          <dl class="sfo-pairs" data-role="secret-pairs"></dl>
        </div>

        <div class="sfo-modal__body" data-view="confirm" hidden>
          <div class="sfo-callout sfo-callout--danger">
            <p class="sfo-callout__title" data-role="confirm-title"></p>
            <p class="sfo-callout__body" data-role="confirm-hint"></p>
          </div>
        </div>

        <footer class="sfo-modal__footer">
          <button type="button" class="sfo-btn" data-action="close-modal" data-role="footer-cancel">${t.cancel}</button>
          <button type="button" class="sfo-btn sfo-btn--primary" data-role="modal-submit"></button>
        </footer>
      </div>
    </div>
  `
  target.append(root)

  const hintEl = root.querySelector('[data-role="hint"]')
  const statusEl = root.querySelector('[data-role="status"]')
  const tableWrap = root.querySelector('[data-role="table-wrap"]')
  const rowsEl = root.querySelector('[data-role="rows"]')
  const emptyEl = root.querySelector('[data-role="empty"]')
  const modal = root.querySelector('[data-role="modal"]')
  const modalTitle = root.querySelector('[data-role="modal-title"]')
  const modalSubmit = root.querySelector('[data-role="modal-submit"]')
  const formEl = root.querySelector('[data-role="form"]')
  const formError = root.querySelector('[data-role="form-error"]')
  const typeHint = root.querySelector('[data-role="type-hint"]')
  const secretPairs = root.querySelector('[data-role="secret-pairs"]')
  const confirmTitle = root.querySelector('[data-role="confirm-title"]')
  const confirmHint = root.querySelector('[data-role="confirm-hint"]')
  const footerCancel = root.querySelector('[data-role="footer-cancel"]')
  // 显式引用字段：form.name 会命中 HTMLFormElement 自带属性，不能用 form.<name> 取值。
  const fields = {
    name: root.querySelector('#sfo-name'),
    description: root.querySelector('#sfo-description'),
    logoUrl: root.querySelector('#sfo-logo'),
    logoFile: root.querySelector('#sfo-logo-file'),
    clientType: root.querySelector('#sfo-type'),
    redirectUris: root.querySelector('#sfo-redirect')
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
  // logo 实时预览：与前台同语言；element 会被替换，引用随之更新。
  let logoPreview = root.querySelector('[data-role="logo-preview"]')
  // 权限范围：chip 多选（选项 = 站点 allowed_scopes，来自 /clients 响应；不再手填）。
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

  let clients = []
  let busy = false
  let view = 'form'
  let mode = 'create'
  let activeClient = null
  let lastTrigger = null

  // 权限范围 chip 多选：与前台弹窗同语义（profile 必选锁定；历史遗留的清单外
  // scope 以锁定 chip 呈现，保存时显式提示将被移除）。描述读插件下发的 scopeCatalog。
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
      name.className = 'sfo-chip__name'
      name.textContent = scope
      const chip = document.createElement(info.alwaysOn ? 'span' : 'button')
      chip.className = 'sfo-chip' + (on || info.alwaysOn ? ' is-on' : '') + (info.alwaysOn ? ' sfo-chip--locked' : '')
      if (!info.alwaysOn) {
        chip.type = 'button'
        chip.setAttribute('aria-pressed', on ? 'true' : 'false')
      }
      chip.append(name)
      if (info.desc) {
        const hint = document.createElement('span')
        hint.className = 'sfo-chip__desc'
        hint.textContent = info.desc
        chip.append(hint)
      }
      const tag = document.createElement('span')
      if (info.alwaysOn) {
        tag.className = 'sfo-chip__tag sfo-chip__tag--accent'
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
      chip.className = 'sfo-chip sfo-chip--locked'
      const name = document.createElement('span')
      name.className = 'sfo-chip__name'
      name.textContent = scope
      const tag = document.createElement('span')
      tag.className = 'sfo-chip__tag'
      tag.textContent = t.scopeCustomTag
      chip.append(name, tag)
      chip.title = t.scopeDropWarning(scope)
      scopesChips.append(chip)
    }
    if (allowedScopes.length > 1) {
      const presets = document.createElement('div')
      presets.className = 'sfo-scope-presets'
      const preset = (label, apply) => {
        const btn = document.createElement('button')
        btn.type = 'button'
        btn.className = 'sfo-chip-preset'
        btn.textContent = label
        btn.addEventListener('click', () => { apply(); renderScopeChips() })
        return btn
      }
      const presetBtns = [preset(t.scopePresetAll, () => { for (const scope of allowedScopes) pickedScopes.add(scope) })]
      if (allowedScopes.length > 2 && allowedScopes.includes('profile')) {
        presetBtns.push(preset(t.scopePresetProfile, () => { pickedScopes.clear(); pickedScopes.add('profile') }))
      }
      presetBtns.push(preset(t.scopePresetNone, () => { pickedScopes.clear(); pickedScopes.add('profile') }))
      presets.append(...presetBtns)
      scopesChips.append(presets)
    }
    scopeNote()
  }

  const setStatus = (message, kind) => {
    if (!message) {
      statusEl.hidden = true
      statusEl.textContent = ''
      return
    }
    statusEl.hidden = false
    statusEl.textContent = message
    statusEl.className = 'sfo-status' + (kind === 'error' ? ' sfo-status--error' : '')
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

  const api = async (path, options = {}) => {
    const method = (options.method || 'GET').toUpperCase()
    const headers = { Accept: 'application/json' }
    if (options.body !== undefined) headers['Content-Type'] = 'application/json'
    if (options.idempotencyKey) headers['Idempotency-Key'] = options.idempotencyKey
    if (method !== 'GET' && method !== 'HEAD') headers['X-Csrf-Token'] = readCookie('csrf_')
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

  const mono = (value, fullValue) => {
    const code = document.createElement('code')
    code.className = 'sfo-mono'
    code.textContent = value
    code.title = fullValue || value
    return code
  }

  // 表格里的 client_id 完整显示；芯片整块可点击复制（触屏上 hover title 不可用）。
  const idChip = (value) => {
    const code = mono(value, value)
    code.classList.add('sfo-mono--copy')
    code.setAttribute('role', 'button')
    code.tabIndex = 0
    const copy = async () => {
      const ok = await copyToClipboard(value)
      toast(ok ? t.copied : t.requestFailed, ok ? value : '', ok ? 'success' : 'error')
    }
    code.addEventListener('click', copy)
    code.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault()
        copy()
      }
    })
    return code
  }

  const copyButton = (value, label) => {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'sfo-btn sfo-btn--sm'
    button.textContent = label
    button.addEventListener('click', async () => {
      const ok = await copyToClipboard(value)
      toast(ok ? t.copied : t.requestFailed, ok ? value : '', ok ? 'success' : 'error')
    })
    return button
  }

  const cell = (label) => {
    const td = document.createElement('td')
    td.dataset.label = label
    return td
  }

  // 统一 icon：与前台、确认页同源（backend/icon.go 下发描述符，这里只渲染）。
  // kind=attachment/url → 生成 tile 上叠 <img>；kind=generated → 方型纯色底 +
  // 5×5 identicon 像素块 + SFAvatar 首字母。本地镜像只服务表单预览与描述符缺失兜底，
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

  const localDescriptor = (seed, name, logoUrl) => {
    const trimmedUrl = text(logoUrl).trim()
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
    svg.setAttribute('class', 'sfo-avatar__ident')
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
    span.className = 'sfo-avatar sfo-avatar--' + (clientType === 'public' ? 'pub' : 'conf')
    span.setAttribute('aria-hidden', 'true')
    const tile = document.createElement('span')
    tile.className = 'sfo-avatar__tile'
    tile.style.background = text(icon.background) || ICON_BACKGROUND
    const pattern = text(icon.pattern) || identiconPattern(text(seed) || name)
    tile.append(renderIdenticon(pattern, text(icon.color) || identiconColor(text(seed) || name)))
    span.append(tile)
    if (icon.url) {
      const img = document.createElement('img')
      img.className = 'sfo-avatar__img'
      img.src = icon.url
      img.alt = ''
      img.loading = 'lazy'
      img.referrerPolicy = 'no-referrer'
      img.addEventListener('error', () => img.remove())
      span.append(img)
    }
    return span
  }

  const renderLogoPreview = () => {
    if (!logoPreview) return
    const name = fields.name.value.trim()
    const seed = text(activeClient && activeClient.clientId) || name || 'sfo_client_preview'
    const next = renderIcon(localDescriptor(seed, name, iconChoice.previewURL), seed, name, fields.clientType.value)
    next.classList.add('sfo-avatar--preview')
    logoPreview.replaceWith(next)
    logoPreview = next
  }

  const setIconStatus = (message, kind) => {
    if (!iconStatus) return
    iconStatus.textContent = text(message)
    iconStatus.classList.toggle('sfo-icon-status--error', kind === 'error')
  }

  // 每次来源变化都重画预览、同步「移除」按钮与状态文案。
  const applyIconState = (message, kind) => {
    renderLogoPreview()
    const hasIcon = Boolean(iconChoice.previewURL) || iconChoice.kind === 'attachment'
    if (iconClear) iconClear.hidden = !hasIcon
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

  // 管理端页面始终带 csrf_ cookie；缺失时先打插件自己的握手端点补上。
  const ensureCsrf = () => {
    if (readCookie('csrf_')) return Promise.resolve()
    return fetch(API + '/csrf', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
      .catch(() => {})
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
    if (iconClear) iconClear.hidden = !iconChoice.previewURL
    setIconStatus(client && client.attachmentPublicId ? t.iconUploadedSaved : '')
  }

  const renderRows = () => {
    rowsEl.textContent = ''
    for (const client of clients) {
      const row = document.createElement('tr')

      const appCell = cell(t.colApp)
      const appHead = document.createElement('div')
      appHead.className = 'sfo-app__head'
      appHead.append(renderIcon(client.icon, text(client.clientId), text(client.name), client.clientType))
      const name = document.createElement('p')
      name.className = 'sfo-app__name'
      name.textContent = text(client.name)
      appHead.append(name)
      const idLine = document.createElement('div')
      idLine.className = 'sfo-app__id'
      const fullClientId = text(client.clientId)
      // client_id 前面必须有可见文字：以前是一串无标签的字符串，让人误以为是密钥。
      // 标签独占一行（flex-basis:100%），值再换行显示，窄单元格里也不会挤成一团。
      const idLabel = document.createElement('span')
      idLabel.className = 'sfo-app__idlabel'
      idLabel.title = t.idHint
      idLabel.append(document.createTextNode(t.idLabel))
      const idTag = document.createElement('span')
      idTag.className = 'sfo-tag sfo-tag--public'
      idTag.textContent = t.idTagPublic
      idTag.title = t.idHint
      idLabel.append(idTag)
      idLine.append(idLabel, idChip(fullClientId))
      idLine.append(copyButton(text(client.clientId), t.copy))
      const description = document.createElement('p')
      description.className = 'sfo-app__desc'
      description.textContent = text(client.description)
      appCell.append(appHead, idLine)
      if (description.textContent) appCell.append(description)
      row.append(appCell)

      const typeCell = cell(t.colType)
      const badge = document.createElement('span')
      const isPublic = client.clientType === 'public'
      badge.className = 'sfo-badge' + (isPublic ? ' sfo-badge--public' : '')
      badge.textContent = isPublic ? t.typePublic : t.typeConfidential
      typeCell.append(badge)
      row.append(typeCell)

      const redirectCell = cell(t.colRedirect)
      const uris = Array.isArray(client.redirectUris) ? client.redirectUris : []
      const shown = uris.slice(0, 2)
      for (const uri of shown) {
        const line = document.createElement('p')
        line.className = 'sfo-uri'
        line.textContent = uri
        line.title = uri
        redirectCell.append(line)
      }
      if (uris.length > shown.length) {
        const more = document.createElement('p')
        more.className = 'sfo-uri sfo-uri--more'
        more.textContent = `+${uris.length - shown.length}`
        redirectCell.append(more)
      }
      row.append(redirectCell)

      const scopeCell = cell(t.colScopes)
      const scopeList = Array.isArray(client.scopes) ? client.scopes : []
      if (scopeList.length === 0) {
        scopeCell.append(document.createTextNode('—'))
      } else {
        const wrap = document.createElement('div')
        wrap.className = 'sfo-chips'
        for (const scope of scopeList) {
          const chip = document.createElement('span')
          chip.className = 'sfo-chip'
          chip.textContent = scope
          wrap.append(chip)
        }
        scopeCell.append(wrap)
      }
      row.append(scopeCell)

      const ownerCell = cell(t.colOwner)
      ownerCell.append(document.createTextNode(text(client.ownerName) || `#${text(client.ownerUserId)}`))
      row.append(ownerCell)

      const usedCell = cell(t.colUsed)
      const used = formatTimestamp(client.lastUsedAt)
      if (used) {
        usedCell.append(document.createTextNode(used))
      } else {
        const never = document.createElement('span')
        never.className = 'sfo-muted'
        never.textContent = t.never
        usedCell.append(never)
      }
      row.append(usedCell)

      const actionCell = cell(t.colActions)
      actionCell.classList.add('sfo-cell--end')
      const actions = document.createElement('div')
      actions.className = 'sfo-row-actions'

      const edit = document.createElement('button')
      edit.type = 'button'
      edit.className = 'sfo-btn sfo-btn--sm'
      edit.textContent = t.edit
      edit.addEventListener('click', () => openForm('edit', client, edit))

      const rotate = document.createElement('button')
      rotate.type = 'button'
      rotate.className = 'sfo-btn sfo-btn--sm'
      rotate.textContent = t.rotate
      rotate.disabled = client.clientType === 'public'
      if (rotate.disabled) rotate.title = t.typePublicHint
      rotate.addEventListener('click', () => openConfirm('rotate', client, rotate))

      const remove = document.createElement('button')
      remove.type = 'button'
      remove.className = 'sfo-btn sfo-btn--sm sfo-btn--danger'
      remove.textContent = t.remove
      remove.addEventListener('click', () => openConfirm('delete', client, remove))

      actions.append(edit, rotate, remove)
      actionCell.append(actions)
      row.append(actionCell)

      rowsEl.append(row)
    }
  }

  const openModal = (trigger) => {
    lastTrigger = trigger || null
    modal.hidden = false
    document.documentElement.classList.add('sfo-modal-open')
    window.requestAnimationFrame(() => {
      const focusable = views[view].querySelector('input, select, textarea, button')
      if (focusable) focusable.focus()
      else modalSubmit.focus()
    })
  }

  const closeModal = () => {
    modal.hidden = true
    document.documentElement.classList.remove('sfo-modal-open')
    setBusy(false)
    if (lastTrigger && document.contains(lastTrigger)) lastTrigger.focus()
    lastTrigger = null
  }

  const showView = (next) => {
    view = next
    for (const [key, element] of Object.entries(views)) element.hidden = key !== next
    formError.hidden = true
    footerCancel.hidden = next === 'secret'
    if (next === 'form') {
      modalSubmit.hidden = false
      modalSubmit.textContent = mode === 'create' ? t.submitCreate : t.submitSave
      typeHint.textContent = fields.clientType.value === 'public' ? t.typePublicHint : t.typeConfidentialHint
    } else if (next === 'secret') {
      modalSubmit.hidden = false
      modalSubmit.textContent = t.secretDone
      modalSubmit.className = 'sfo-btn sfo-btn--primary'
    } else {
      modalSubmit.hidden = false
      modalSubmit.textContent = mode === 'rotate' ? t.rotateConfirm : t.deleteConfirm
      modalSubmit.className = mode === 'delete' ? 'sfo-btn sfo-btn--danger' : 'sfo-btn sfo-btn--primary'
    }
  }

  const openForm = (nextMode, client, trigger) => {
    mode = nextMode
    activeClient = client || null
    modalTitle.textContent = nextMode === 'create' ? t.createTitle : t.editTitle
    formEl.reset()
    formError.hidden = true
    if (client) {
      fields.name.value = text(client.name)
      fields.description.value = text(client.description)
      fields.clientType.value = client.clientType === 'public' ? 'public' : 'confidential'
      fields.redirectUris.value = (client.redirectUris || []).join('\n')
    }
    fields.logoUrl.value = text(client && client.logoUrl)
    resetIconChoice(client)
    const clientScopes = client ? (client.scopes || []).map(text) : []
    pickedScopes = new Set(clientScopes.filter((scope) => allowedScopes.includes(scope)))
    pickedScopes.add('profile')
    customScopes = clientScopes.filter((scope) => !allowedScopes.includes(scope))
    renderScopeChips()
    modalSubmit.className = 'sfo-btn sfo-btn--primary'
    showView('form')
    openModal(trigger)
  }

  const openConfirm = (nextMode, client, trigger) => {
    mode = nextMode
    activeClient = client
    const isDelete = nextMode === 'delete'
    modalTitle.textContent = isDelete ? t.deleteTitle : t.rotateTitle
    confirmTitle.textContent = isDelete ? `${t.deleteTitle}：${text(client.name)}` : `${t.rotateTitle}：${text(client.name)}`
    confirmHint.textContent = isDelete ? t.deleteHint : t.rotateHint
    showView('confirm')
    openModal(trigger)
  }

  // 一次性凭据弹窗：每一行都带中文名 + 公开/保密标签 + 一句话说明，
  // 避免把 client_id 误当成需要保密的密钥（两者不是公私钥对）。
  const openSecret = (clientId, clientSecret, title) => {
    modalTitle.textContent = title || t.secretTitle
    secretPairs.textContent = ''
    const entries = [
      [t.secretLabelId, t.idTagPublic, 'sfo-tag--public', clientId, t.secretIdNote],
      [t.secretLabelKey, t.secretTagPrivate, 'sfo-tag--private', clientSecret, t.secretKeyNote]
    ]
    for (const [label, tagText, tagClass, value, note] of entries) {
      const row = document.createElement('div')
      row.className = 'sfo-pair'
      const dt = document.createElement('dt')
      dt.append(document.createTextNode(label))
      const tag = document.createElement('span')
      tag.className = `sfo-tag ${tagClass}`
      tag.textContent = tagText
      dt.append(tag)
      const dd = document.createElement('dd')
      dd.append(mono(value))
      dd.append(copyButton(value, t.copy))
      const hint = document.createElement('p')
      hint.className = 'sfo-pair__note'
      hint.textContent = note
      row.append(dt, dd, hint)
      secretPairs.append(row)
    }
    const summary = document.createElement('div')
    summary.className = 'sfo-pair__note sfo-pair__note--summary'
    summary.textContent = t.secretPairsNote
    secretPairs.append(summary)
    showView('secret')
    setBusy(false)
    window.requestAnimationFrame(() => modalSubmit.focus())
  }

  const load = async () => {
    setBusy(true)
    setStatus('', 'error')
    try {
      const data = await api('/clients')
      clients = Array.isArray(data.clients) ? data.clients : []
      allowedScopes = Array.isArray(data.allowedScopes) ? data.allowedScopes.map(text).filter(Boolean) : []
      scopeCatalog = Array.isArray(data.scopeCatalog) ? data.scopeCatalog : []
      const source = text(data.source) || 'database'
      hintEl.textContent = t.sectionHint(clients.length, source)
      renderRows()
      const hasRows = clients.length > 0
      tableWrap.hidden = !hasRows
      emptyEl.hidden = hasRows
    } catch (error) {
      tableWrap.hidden = clients.length === 0
      emptyEl.hidden = clients.length > 0
      setStatus(error.message, 'error')
    } finally {
      setBusy(false)
    }
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
      formError.textContent = error
      formError.hidden = false
      return
    }
    setBusy(true)
    // Host 只在带 Idempotency-Key 时才签发 claim/release 的 actor 委派。
    const idempotencyKey = newIdempotencyKey()
    try {
      if (mode === 'create') {
        const created = await api('/clients/create', { method: 'POST', body: payload, idempotencyKey })
        toast(t.created, payload.name)
        await load()
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
        toast(t.saved, payload.name)
        await load()
        if (updated && updated.clientSecret) {
          openSecret(text(updated.client && updated.client.clientId), text(updated.clientSecret), t.saved)
        } else {
          closeModal()
        }
      }
    } catch (requestError) {
      // 图标类失败已经在字段旁就地说明（并展开 URL 入口），不必再重复一条表单错误。
      if (!handleIconFailure(requestError)) {
        formError.textContent = requestError.message
        formError.hidden = false
      }
      setBusy(false)
    }
  }

  const submitConfirm = async () => {
    const clientId = text(activeClient && activeClient.clientId)
    setBusy(true)
    try {
      if (mode === 'rotate') {
        const result = await api('/clients/rotate-secret', { method: 'POST', body: { clientId } })
        toast(t.rotated, clientId)
        await load()
        if (result && result.clientSecret) {
          openSecret(clientId, text(result.clientSecret), t.rotated)
        } else {
          closeModal()
        }
      } else {
        await api('/clients/delete', { method: 'POST', body: { clientId }, idempotencyKey: newIdempotencyKey() })
        toast(t.deleted, clientId)
        await load()
        closeModal()
      }
    } catch (error) {
      setBusy(false)
      setStatus(error.message, 'error')
      closeModal()
    }
  }

  const onRootClick = (event) => {
    const button = event.target.closest('[data-action]')
    if (!button) return
    const action = button.dataset.action
    if (action === 'refresh') load()
    if (action === 'create') openForm('create', null, button)
    if (action === 'close-modal') closeModal()
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

  const onTypeChange = () => {
    typeHint.textContent = fields.clientType.value === 'public' ? t.typePublicHint : t.typeConfidentialHint
    renderLogoPreview()
  }
  fields.clientType.addEventListener('change', onTypeChange)
  fields.logoUrl.addEventListener('input', onLogoURLInput)
  if (fields.logoFile) fields.logoFile.addEventListener('change', onIconFileChange)
  if (iconClear) iconClear.addEventListener('click', onClearIcon)
  fields.name.addEventListener('input', renderLogoPreview)

  const onKeydown = (event) => {
    if (event.key === 'Escape' && !modal.hidden) {
      event.stopPropagation()
      closeModal()
    }
  }
  document.addEventListener('keydown', onKeydown)

  const onAuthFailed = (event) => {
    if (event.target !== root && event.target !== modal && !root.contains(event.target) && !modal.contains(event.target)) return
    setStatus(event.detail && event.detail.message ? event.detail.message : t.requestFailed, 'error')
  }
  window.addEventListener('sforum:auth-failed', onAuthFailed)

  load()

  return () => {
    root.removeEventListener('click', onRootClick)
    formEl.removeEventListener('submit', onFormSubmit)
    fields.clientType.removeEventListener('change', onTypeChange)
    fields.logoUrl.removeEventListener('input', onLogoURLInput)
    if (fields.logoFile) fields.logoFile.removeEventListener('change', onIconFileChange)
    if (iconClear) iconClear.removeEventListener('click', onClearIcon)
    fields.name.removeEventListener('input', renderLogoPreview)
    modalSubmit.removeEventListener('click', onModalSubmit)
    document.removeEventListener('keydown', onKeydown)
    window.removeEventListener('sforum:auth-failed', onAuthFailed)
    document.documentElement.classList.remove('sfo-modal-open')
    root.remove()
  }
}

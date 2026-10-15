export const apiVersion = 1

// OAuth2 应用管理面板（Admin Micro-frontend API v1，无构建步骤的纯 ESM）。
// 约束与约定：
// - 只调用插件自身的 /api/oauth 路由；unsafe 方法按 Host 的 double-submit 规则带 X-Csrf-Token。
// - 主题沿用 Host 管理端变量（--bg-admin-card/--border-admin/--text-admin-main/--sf-accent…），
//   暗色由祖先 .dark 类驱动，因此外观切换无需重新挂载。
// - 插件微前端拿不到 iconify 运行时（i-tabler-* 类不生效），因此统一使用文字按钮，
//   不使用 emoji 或自绘图标。

const API = '/api/oauth'

const MESSAGES = {
  'zh-CN': {
    sectionTitle: '应用列表',
    sectionHint: (count, source) => `共 ${count} 个应用 · 存储：${source}`,
    refresh: '刷新',
    create: '新建应用',
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
    fieldType: '客户端类型',
    typeConfidentialHint: '机密客户端持有 client_secret，适合有服务端的应用。',
    typePublicHint: '公开客户端不使用密钥，必须使用 PKCE，适合纯前端应用。',
    fieldRedirect: '回调地址',
    fieldRedirectHint: '每行一个，必须是 https；本地开发可用 http://127.0.0.1 或 http://localhost。',
    fieldScopes: '权限范围',
    fieldScopesHint: '留空使用站点默认（profile：读取用户名、昵称与账号状态）。',
    cancel: '取消',
    submitCreate: '创建',
    submitSave: '保存',
    submitting: '处理中…',
    secretTitle: '请立即保存 client_secret',
    secretHint: '密钥只显示这一次；关闭后无法再次查看，遗失时只能重置。',
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
    fieldType: 'Client type',
    typeConfidentialHint: 'A confidential client holds a client_secret and fits server-side apps.',
    typePublicHint: 'A public client has no secret and must use PKCE; fits browser-only apps.',
    fieldRedirect: 'Redirect URIs',
    fieldRedirectHint: 'One per line, https required; http://127.0.0.1 or http://localhost is allowed for local development.',
    fieldScopes: 'Scopes',
    fieldScopesHint: 'Leave empty to use the site default (profile: username, display name, account status).',
    cancel: 'Cancel',
    submitCreate: 'Create',
    submitSave: 'Save',
    submitting: 'Working…',
    secretTitle: 'Store the client_secret now',
    secretHint: 'The secret is shown once; it cannot be viewed again and must be reset if lost.',
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
              <label class="sfo-field__label" for="sfo-scopes">${t.fieldScopes}</label>
              <p class="sfo-field__hint">${t.fieldScopesHint}</p>
              <input class="sfo-input" id="sfo-scopes" name="scopes" autocomplete="off" placeholder="profile">
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
    clientType: root.querySelector('#sfo-type'),
    redirectUris: root.querySelector('#sfo-redirect'),
    scopes: root.querySelector('#sfo-scopes')
  }
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
      throw new Error(detail && detail !== reason ? `${reason}: ${detail}` : reason)
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

  // 表格里只展示可读的短形式；完整值靠 title 与「复制」按钮获取。
  const shortenId = (value) => (value.length > 20 ? `${value.slice(0, 12)}…${value.slice(-4)}` : value)

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

  const renderRows = () => {
    rowsEl.textContent = ''
    for (const client of clients) {
      const row = document.createElement('tr')

      const appCell = cell(t.colApp)
      const name = document.createElement('p')
      name.className = 'sfo-app__name'
      name.textContent = text(client.name)
      const idLine = document.createElement('div')
      idLine.className = 'sfo-app__id'
      const fullClientId = text(client.clientId)
      idLine.append(mono(shortenId(fullClientId), fullClientId))
      idLine.append(copyButton(text(client.clientId), t.copy))
      const description = document.createElement('p')
      description.className = 'sfo-app__desc'
      description.textContent = text(client.description)
      appCell.append(name, idLine)
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
      fields.scopes.value = (client.scopes || []).join(' ')
    }
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

  const openSecret = (clientId, clientSecret, title) => {
    modalTitle.textContent = title || t.secretTitle
    secretPairs.textContent = ''
    const entries = [
      ['client_id', clientId],
      ['client_secret', clientSecret]
    ]
    for (const [key, value] of entries) {
      const dt = document.createElement('dt')
      dt.textContent = key
      const dd = document.createElement('dd')
      dd.append(mono(value))
      dd.append(copyButton(value, t.copy))
      secretPairs.append(dt, dd)
    }
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
    const scopes = fields.scopes.value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean)
    const payload = {
      name,
      description: fields.description.value.trim(),
      clientType: fields.clientType.value,
      redirectUris
    }
    if (scopes.length > 0) payload.scopes = scopes
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
    try {
      if (mode === 'create') {
        const created = await api('/clients/create', { method: 'POST', body: payload })
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
          body: { ...payload, clientId: text(activeClient && activeClient.clientId) }
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
      formError.textContent = requestError.message
      formError.hidden = false
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
        await api('/clients/delete', { method: 'POST', body: { clientId } })
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
  }
  fields.clientType.addEventListener('change', onTypeChange)

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
    modalSubmit.removeEventListener('click', onModalSubmit)
    document.removeEventListener('keydown', onKeydown)
    window.removeEventListener('sforum:auth-failed', onAuthFailed)
    document.documentElement.classList.remove('sfo-modal-open')
    root.remove()
  }
}

export const apiVersion = 1

// 接入说明页的三张表（scope / 端点 / 错误码）。
//
// 页面本身是**宿主模板**（templates/oauth-docs.html）：正文、标题、表格骨架都在模板里
// SSR 输出，首帧即有完整内容，不会出现「空白卡片 → 整页文字」的布局跳动。
// 本组件只做两件模板做不到的事：
//   1. 把示例里的 `https://站点域名` 换成当前站点地址；
//   2. 从 GET /api/oauth/docs 取三张表的行并写进模板预留的 tbody。
// 外壳（navbar / footer / 布局 / 主题 / 排版）全部由 Core 提供，组件不渲染任何页面骨架。

const API = '/api/oauth'

// 动态数据一律转义后再进 innerHTML：后端字段虽然是自家生成的，也不假设它安全。
function escapeHTML(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function scopeRows(scopes) {
  if (!Array.isArray(scopes) || scopes.length === 0) return ''
  return scopes.map(function (scope) {
    const name = escapeHTML(scope && scope.name)
    const description = escapeHTML(scope && scope.description && scope.description['zh-CN'])
    const requirement = scope && scope.alwaysOn ? '必选' : '按需'
    return '<tr><td><code>' + name + '</code></td><td>' + description + '</td><td>' + requirement + '</td></tr>'
  }).join('')
}

function endpointRows(endpoints, internal) {
  if (!Array.isArray(endpoints)) return ''
  return endpoints.filter(function (item) {
    return Boolean(item && item.internal) === Boolean(internal)
  }).map(function (item) {
    const guard = escapeHTML(String(item.guard || '').replace('core.guard.', ''))
    return '<tr>'
      + '<td><span class="sfp-docs__method">' + escapeHTML(item.method) + '</span></td>'
      + '<td><code>' + escapeHTML(item.path) + '</code></td>'
      + '<td>' + guard + '</td>'
      + '<td>' + escapeHTML(item.note) + '</td>'
      + '</tr>'
  }).join('')
}

function errorRows(errors) {
  if (!Array.isArray(errors)) return ''
  return errors.map(function (item) {
    return '<tr>'
      + '<td><code>' + escapeHTML(item.code) + '</code></td>'
      + '<td>' + escapeHTML(item.meaning) + '</td>'
      + '<td>' + escapeHTML(item.when) + '</td>'
      + '</tr>'
  }).join('')
}

export function mount(target, bridge) {
  const doc = (target && target.ownerDocument) || document

  // 挂载锚点：组件本身不渲染可见内容，只用来在卸载时收尾。
  const anchor = doc.createElement('div')
  anchor.className = 'sfp-docs__anchor'
  if (target) target.appendChild(anchor)

  // 示例域名：同步替换，不等网络，避免二次排版。
  const origin = location.origin
  for (const node of doc.querySelectorAll('.sfp-docs__origin')) {
    node.textContent = origin
  }

  let disposed = false

  const fill = function (id, html) {
    const body = doc.getElementById(id)
    if (!body || !html) return
    body.innerHTML = html
  }

  fetch(API + '/docs', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    .then(function (response) { return response.json() })
    .then(function (parsed) {
      if (disposed) return
      const data = parsed && parsed.data && typeof parsed.data === 'object' ? parsed.data : parsed
      if (!data || data.ok !== true) return
      fill('sfp-scope-rows', scopeRows(data.scopes))
      fill('sfp-error-rows', errorRows(data.errors))
      fill('sfp-endpoint-rows', endpointRows(data.endpoints, false))
      fill('sfp-internal-endpoint-rows', endpointRows(data.endpoints, true))
    })
    .catch(function () {})

  return function unmount() {
    disposed = true
    anchor.remove()
  }
}

package main

import (
	"html"
	"strings"
)

// renderConsentPage 输出授权确认页。页面自行读取 request 令牌、展示应用与权限，
// 并把用户的决定提交给 /api/oauth/consent/decision。
// 说明：Host 插件 HTTP 路由只允许 JSON 文档，因此确认页由 stream 模式承载。
func renderConsentPage(requestToken, returnURL string) string {
	page := strings.ReplaceAll(consentPageTemplate, "__REQUEST_TOKEN__", html.EscapeString(requestToken))
	page = strings.ReplaceAll(page, "__RETURN_URL__", html.EscapeString(returnURL))
	return page
}

const consentPageTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>授权确认</title>
<style>
:root { color-scheme: light dark; --sf-border: #e4e4e7; --sf-muted: #71717a; --sf-bg: #fff; --sf-fg: #18181b; --sf-accent: #2563eb; }
@media (prefers-color-scheme: dark) { :root { --sf-border: #3f3f46; --sf-muted: #a1a1aa; --sf-bg: #18181b; --sf-fg: #f4f4f5; --sf-accent: #60a5fa; } }
* { box-sizing: border-box; }
body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
  background: var(--sf-bg); color: var(--sf-fg);
  font: 15px/1.6 system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; }
main { width: 100%; max-width: 26rem; margin: 2rem 1rem; border: 1px solid var(--sf-border); border-radius: 12px; padding: 1.5rem; }
h1 { margin: 0 0 .25rem; font-size: 1.125rem; font-weight: 600; }
p.lead { margin: 0 0 1rem; color: var(--sf-muted); font-size: .875rem; }
ul { margin: 0 0 1rem; padding: 0; list-style: none; border: 1px solid var(--sf-border); border-radius: 8px; }
li { padding: .5rem .75rem; border-bottom: 1px solid var(--sf-border); font-size: .875rem; }
li:last-child { border-bottom: 0; }
label.remember { display: flex; gap: .5rem; align-items: center; font-size: .875rem; margin-bottom: 1rem; }
.actions { display: flex; gap: .5rem; }
button { flex: 1; padding: .55rem .9rem; border-radius: 8px; border: 1px solid var(--sf-border);
  background: transparent; color: inherit; font: inherit; cursor: pointer; }
button.primary { background: var(--sf-accent); border-color: var(--sf-accent); color: #fff; font-weight: 600; }
button[disabled] { opacity: .55; cursor: not-allowed; }
.status { margin-top: .75rem; color: var(--sf-muted); font-size: .8125rem; min-height: 1.25rem; }
.status.error { color: #dc2626; }
a { color: var(--sf-accent); }
</style>
</head>
<body>
<main>
  <h1>授权 <span id="client-name">…</span></h1>
  <p class="lead" id="client-description"></p>
  <p class="lead" id="user-line"></p>
  <ul id="scope-list"></ul>
  <label class="remember"><input type="checkbox" id="remember" checked> 记住这次授权，之后不再询问</label>
  <div class="actions">
    <button type="button" id="deny">拒绝</button>
    <button type="button" id="allow" class="primary">允许</button>
  </div>
  <p class="status" id="status"></p>
</main>
<script type="module">
const requestToken = "__REQUEST_TOKEN__"
const returnURL = "__RETURN_URL__"
const statusEl = document.getElementById('status')
const allowButton = document.getElementById('allow')
const denyButton = document.getElementById('deny')

function unwrapDocument(parsed) {
  const enveloped = parsed && parsed.data && typeof parsed.data === 'object'
    && (parsed.code !== undefined || parsed.message !== undefined)
  return enveloped ? parsed.data : parsed
}

function readCookie(name) {
  for (const item of document.cookie.split(';')) {
    const [key, ...rest] = item.trim().split('=')
    if (key === name) return decodeURIComponent(rest.join('='))
  }
  return ''
}

function setStatus(text, isError) {
  statusEl.textContent = text || ''
  statusEl.className = isError ? 'status error' : 'status'
}

function setBusy(busy) {
  allowButton.disabled = busy
  denyButton.disabled = busy
}

async function fetchSummary() {
  const response = await fetch('/api/oauth/consent/summary?request=' + encodeURIComponent(requestToken), {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' }
  })
  const text = await response.text()
  let parsed = {}
  try { parsed = JSON.parse(text) || {} } catch (error) { parsed = {} }
  // 插件路由成功时返回裸文档；Host 的错误信封则是 {code,message,data}。
  const data = unwrapDocument(parsed)
  return {
    ok: response.ok,
    status: response.status,
    data,
    reason: (parsed.data && parsed.data.reason) || parsed.reason || data.error || null
  }
}

async function loadSummary() {
  // 页面正文由 stream 模式输出，紧随其后的首次数据请求偶尔会撞上运行时切换，
  // 因此失败时先重试一次，再把真实 HTTP 状态显示出来，而不是笼统地说“已失效”。
  let result = await fetchSummary()
  if (!result.ok || result.data.status !== 'ok') {
    await new Promise((resolve) => setTimeout(resolve, 600))
    result = await fetchSummary()
  }
  const data = result.data
  if (!result.ok || data.status !== 'ok') {
    const detail = result.reason ? '（HTTP ' + result.status + ' · ' + result.reason + '）' : ''
    setStatus((data.error || '授权请求已失效，请重新发起授权。') + detail, true)
    setBusy(true)
    return
  }
  document.getElementById('client-name').textContent = data.clientName || data.clientId || ''
  document.getElementById('client-description').textContent = data.clientDescription || ''
  document.getElementById('user-line').textContent =
    '以 ' + (data.user.displayName || data.user.username) + ' 的身份授权'
  const list = document.getElementById('scope-list')
  list.textContent = ''
  for (const scope of data.scopes || []) {
    const item = document.createElement('li')
    item.textContent = scope.label || scope.key
    list.append(item)
  }
}

async function decide(decision) {
  setBusy(true)
  setStatus('正在处理…', false)
  try {
    const response = await fetch('/api/oauth/consent/decision', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'X-Csrf-Token': readCookie('csrf_')
      },
      body: JSON.stringify({
        request: requestToken,
        decision,
        remember: document.getElementById('remember').checked
      })
    })
    const parsed = await response.json().catch(() => ({}))
    const data = unwrapDocument(parsed)
    if (data.status === 'redirect' && data.location) {
      window.location.replace(data.location)
      return
    }
    setStatus(data.errorDescription || data.error || '处理失败，请重试。', true)
    setBusy(false)
  } catch (error) {
    setStatus('网络错误，请重试。', true)
    setBusy(false)
  }
}

allowButton.addEventListener('click', () => decide('allow'))
denyButton.addEventListener('click', () => decide('deny'))
loadSummary().catch(() => {
  setStatus('无法加载授权信息，请重新发起授权。', true)
  setBusy(true)
})
if (returnURL) {
  document.getElementById('client-description').dataset.returnUrl = returnURL
}
</script>
</body>
</html>
`

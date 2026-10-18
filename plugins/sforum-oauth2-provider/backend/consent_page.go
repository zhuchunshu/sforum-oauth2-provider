package main

import (
	"encoding/json"
	"strings"
)

// renderConsentPage 输出授权确认页。页面自行读取 request 令牌、站点品牌、
// 外观偏好并展示应用与权限，把用户的决定提交给 /api/oauth/consent/decision。
// 说明：Host 插件 HTTP 路由只允许 JSON 文档，因此确认页由 stream 模式承载。
//
// 注入方式：两个值都落在 <script> 的 JS 字符串字面量里，必须用 JSON 编码而不是
// HTML 转义——<script> 内容不做实体解码，`html.EscapeString` 会把 returnURL 里的
// `&` 变成字面量 `&amp;`，直接把授权地址拼坏。json.Marshal 默认把 < > & 转成
// \u003c 之类的转义序列，同时免疫 `</script>` 截断。
func renderConsentPage(requestToken, returnURL string) string {
	page := strings.ReplaceAll(consentPageTemplate, "__REQUEST_TOKEN_JSON__", jsonLiteral(requestToken))
	return strings.ReplaceAll(page, "__RETURN_URL_JSON__", jsonLiteral(returnURL))
}

// jsonLiteral 把值编码成可安全内联进 <script> 的 JS 字符串字面量。
func jsonLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// consentPageTemplate 是自包含的单文件页面：无外部字体 / CDN / 框架。
// 主题与品牌在运行时从宿主公开配置（/api/v1/web-options）与当前会话
// （/api/v1/auth/session）读取；注入前保持中性灰底色，绝不闪站点之外的强调色。
// 明暗模式跟随站点 nuxt-color-mode 偏好（localStorage，system/light/dark）。
// 注意：Go 原始字符串不能出现反引号，模板内的 JS 只用单引号与字符串拼接。
const consentPageTemplate = `<!DOCTYPE html>
<html lang="zh-CN" data-mode="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="robots" content="noindex, nofollow">
<meta name="color-scheme" content="light dark">
<title>授权确认 · SForum</title>
<script>
(function () {
  var preference = ''
  try { preference = localStorage.getItem('nuxt-color-mode') || '' } catch (error) { preference = '' }
  var mode = 'light'
  if (preference === 'dark') mode = 'dark'
  else if (preference === 'light') mode = 'light'
  else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) mode = 'dark'
  document.documentElement.setAttribute('data-mode', mode)
})()
</script>
<style>
:root {
  --font: Inter, "Noto Sans SC", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;

  /* 主题变量由运行时注入；注入前保持中性灰，不预设任何站点主题色。 */
  --sf-accent: #52525b;
  --sf-accent-hover: #3f3f46;
  --sf-accent-soft-light: #f4f4f5;
  --sf-accent-dark: #a1a1aa;
  --sf-accent-rgb: 82 82 91;

  /* 站点浅色背景（appearance.light_background）；注入前用 mist_gray 近似。 */
  --lb-bg: #f3f5f7;
  --lb-surface: #ffffff;
  --lb-muted: #eaedf1;
  --lb-border: #dde1e6;

  --bg: var(--lb-bg);
  --surface: var(--lb-surface);
  --surface-muted: var(--lb-muted);
  --text: #0b0b0e;
  --muted: #63636d;
  --faint: #9a9aa4;
  --border: var(--lb-border);
  --border-strong: #d8d8de;
  --danger: #dc2626;
  --sf-accent-soft: var(--sf-accent-soft-light);
  --accent-fg: var(--sf-accent);
  color-scheme: light;
}
:root[data-mode="dark"] {
  --bg: #08080a;
  --surface: #16161a;
  --surface-muted: #1d1d22;
  --text: #f4f4f5;
  --muted: #a1a1aa;
  --faint: #6f6f79;
  --border: #26262c;
  --border-strong: #34343c;
  --danger: #f87171;
  --sf-accent-soft: rgb(var(--sf-accent-rgb) / .16);
  --accent-fg: var(--sf-accent-dark);
  color-scheme: dark;
}

* { box-sizing: border-box; }
a { color: var(--accent-fg); text-underline-offset: 3px; }
html, body { height: 100%; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font: 15px/1.6 var(--font);
  -webkit-font-smoothing: antialiased;
  overflow: hidden;
}

/* ---------- 背景：主色柔光 + 站点品牌 ---------- */
.scene { position: fixed; inset: 0; display: grid; place-items: center; overflow: hidden; }
.backdrop { position: absolute; inset: 0; background: var(--bg); overflow: hidden; }
.glow { position: absolute; border-radius: 50%; filter: blur(70px); pointer-events: none; }
.glow-a {
  width: 62vmax; height: 62vmax; left: -20vmax; top: -28vmax;
  background: radial-gradient(circle at 50% 50%, rgb(var(--sf-accent-rgb) / .42), transparent 70%);
}
.glow-b {
  width: 54vmax; height: 54vmax; right: -22vmax; bottom: -26vmax;
  background: radial-gradient(circle at 50% 50%, rgb(var(--sf-accent-rgb) / .3), transparent 70%);
}
.backdrop::after {
  content: "";
  position: absolute; inset: 0;
  background-image: radial-gradient(rgb(0 0 0 / .5) 1px, transparent 1px);
  background-size: 3px 3px;
  opacity: .04;
}
:root[data-mode="dark"] .backdrop::after {
  background-image: radial-gradient(rgb(255 255 255 / .5) 1px, transparent 1px);
}

/* topbar：站点名 + 后台设置的站点 icon；点击新标签页打开站点首页 */
.brandline {
  position: absolute; top: 22px; left: 50%;
  transform: translateX(-50%);
  display: inline-flex; align-items: center; gap: 8px;
  max-width: calc(100vw - 32px);
  font-size: 12px; font-weight: 500; letter-spacing: .01em;
  color: var(--faint);
  text-decoration: none;
  white-space: nowrap;
  border-radius: 8px;
}
.brandline:hover { color: var(--text); }
.brandline:focus-visible { outline: none; box-shadow: 0 0 0 3px rgb(var(--sf-accent-rgb) / .3); }
.brand-icon-wrap {
  width: 22px; height: 22px; flex: none;
  position: relative;
  display: grid; place-items: center;
  border-radius: 6px;
  overflow: hidden;
}
.brand-icon {
  position: absolute; inset: 0;
  width: 100%; height: 100%;
  object-fit: contain;
}
.brand-icon[hidden] { display: none; }
.brandline .mark {
  width: 100%; height: 100%;
  display: grid; place-items: center;
  background: var(--sf-accent); color: #fff;
  font-size: 12px; font-weight: 700;
}
.brandline .mark[hidden] { display: none; }
.brand-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; }

/* ---------- 抽屉 ---------- */
.sheet {
  position: relative;
  width: 100%; max-width: 480px; margin: 0 auto;
  background: var(--surface);
  border-radius: 22px 22px 0 0;
  box-shadow: 0 -20px 60px -30px rgb(0 0 0 / .5), 0 0 0 1px rgb(0 0 0 / .04);
  display: flex; flex-direction: column;
  max-height: 92vh;
  align-self: end;
  animation: rise .4s cubic-bezier(.22, 1, .36, 1) both;
}
@keyframes rise { from { transform: translateY(22px); opacity: 0; } to { transform: none; opacity: 1; } }
:root[data-mode="dark"] .sheet { box-shadow: 0 -24px 70px -30px rgb(0 0 0 / .95), 0 0 0 1px rgb(255 255 255 / .09); }
@media (prefers-reduced-motion: reduce) {
  .sheet { animation: none; }
  *, *::before, *::after { animation-duration: .001ms !important; transition-duration: .001ms !important; }
}

.handle {
  width: 38px; height: 4px; border-radius: 999px;
  background: var(--border-strong);
  margin: 9px auto 0; flex: none;
}
.sheet-body { padding: 14px 20px 12px; overflow-y: auto; overscroll-behavior: contain; }

/* 头部 */
.head { display: flex; gap: 12px; align-items: center; }
.tile {
  width: 46px; height: 46px; flex: none; border-radius: 14px;
  position: relative; overflow: hidden;
  box-shadow: 0 10px 22px -14px rgb(var(--sf-accent-rgb) / 1);
}
/* 统一 icon（单一事实源在后端）：__mark 是方型纯色底（#f0f0f0）+ 5×5
   GitHub 风格 identicon 像素块（icon.pattern，颜色 icon.color）；应用设置了
   logo 时叠加 __img，加载失败移除 img 露出底色，永不空白。与前台卡片、
   管理端列表共用同一套描述符。 */
.tile__mark {
  position: absolute; inset: 0;
  display: grid; place-items: center;
  font: 700 1.0625rem/1 var(--font); color: var(--muted);
  background: #f0f0f0;
  box-shadow: inset 0 0 0 1px rgb(0 0 0 / .06);
  user-select: none;
}
.tile__ident {
  position: absolute; inset: 8%; width: 84%; height: 84%;
  pointer-events: none;
}
.tile__img {
  position: absolute; inset: 0; width: 100%; height: 100%;
  object-fit: cover;
  /* 透明 logo 不透出渐变底：白实底 + 发丝描边 */
  background: #fff;
  box-shadow: inset 0 0 0 1px rgb(0 0 0 / .08);
}
.head-text { min-width: 0; flex: 1; }
.head-text h1 {
  margin: 0; font-size: 1.0625rem; font-weight: 700; letter-spacing: -.015em; line-height: 1.35;
  display: flex; align-items: center; gap: 6px;
  overflow: hidden;
}
.head-text h1 span {
  min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.head-text h1 svg { width: 15px; height: 15px; flex: none; color: var(--accent-fg); }
.head-text p { margin: 2px 0 0; font-size: .75rem; color: var(--muted); }
.close {
  width: 30px; height: 30px; flex: none; border-radius: 999px;
  display: grid; place-items: center;
  border: 0; background: var(--surface-muted); color: var(--muted);
  cursor: pointer;
}
.close:hover { color: var(--text); }
.close svg { width: 14px; height: 14px; }

/* 账号 */
.account {
  margin: 13px 0 0;
  display: flex; align-items: center; gap: 9px;
  padding: 8px 10px;
  border-radius: 12px;
  background: var(--surface-muted);
  font-size: .8125rem;
  min-width: 0;
}
.avatar {
  width: 26px; height: 26px; flex: none; border-radius: 999px;
  position: relative; overflow: hidden;
  display: grid; place-items: center;
  font-size: .625rem; font-weight: 700; color: #fff;
  background: var(--sf-accent);
}
.avatar__fallback { position: absolute; inset: 0; display: grid; place-items: center; }
.avatar__img {
  position: absolute; inset: 0; width: 100%; height: 100%;
  object-fit: cover;
}
.account-id { min-width: 0; flex: 1; display: flex; flex-direction: column; }
.account-id b {
  font-weight: 600;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.account-id .handle-name {
  color: var(--muted); font-size: .75rem;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.account-id .handle-name:empty { display: none; }
.account .switch-account {
  margin-left: auto; flex: none;
  font-size: .75rem; font-weight: 600; color: var(--accent-fg); text-decoration: none;
}
.account .switch-account:hover { text-decoration: underline; text-underline-offset: 3px; }

/* 权限 */
.sec-head {
  display: flex; align-items: baseline; justify-content: space-between; gap: 10px;
  margin: 16px 0 7px;
}
.sec-title { font-size: .75rem; font-weight: 600; letter-spacing: .04em; color: var(--faint); }
.sec-count { font-size: .6875rem; color: var(--faint); font-variant-numeric: tabular-nums; }
.sec-count b { color: var(--accent-fg); font-weight: 700; }

.perms { border: 1px solid var(--border); border-radius: 13px; overflow: hidden; }
.perm-toggle {
  width: 100%;
  display: flex; align-items: center; gap: 9px;
  padding: 11px 12px;
  border: 0; background: var(--surface); color: inherit; font: inherit;
  text-align: left; cursor: pointer;
}
.perm-toggle:hover { background: var(--surface-muted); }
.perm-toggle .badge {
  width: 24px; height: 24px; flex: none; border-radius: 7px;
  display: grid; place-items: center;
  color: var(--accent-fg); background: var(--sf-accent-soft);
  font-size: .6875rem; font-weight: 700;
}
.perm-toggle .t { flex: 1; min-width: 0; }
.perm-toggle .t b { display: block; font-size: .8125rem; font-weight: 600; }
.perm-toggle .t small {
  display: block; font-size: .6875rem; color: var(--muted);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.perm-toggle .chev { flex: none; color: var(--faint); transition: transform .2s ease; }
.perm-toggle .chev svg { width: 14px; height: 14px; display: block; }
.perm-toggle[aria-expanded="true"] .chev { transform: rotate(180deg); }

.perm-list { display: none; border-top: 1px solid var(--border); }
.perm-list[data-open="true"] { display: block; }
.perm { display: flex; gap: 10px; align-items: flex-start; padding: 10px 12px; }
.perm + .perm { border-top: 1px solid var(--border); }
.perm-icon {
  width: 24px; height: 24px; flex: none; border-radius: 7px;
  display: grid; place-items: center;
  color: var(--accent-fg); background: var(--sf-accent-soft);
}
.perm-icon svg { width: 13px; height: 13px; }
.perm-text { min-width: 0; }
.perm-text b { display: block; font-size: .8125rem; font-weight: 600; line-height: 1.4; }
.perm-text code {
  display: block; margin-top: 1px;
  font-family: var(--mono); font-size: .625rem; color: var(--faint);
}

.note {
  margin: 9px 0 0;
  display: flex; gap: 6px; align-items: flex-start;
  font-size: .6875rem; line-height: 1.6; color: var(--faint);
}
.note svg { width: 12px; height: 12px; flex: none; margin-top: 2px; }

/* 粘底操作区 */
.sheet-foot {
  flex: none;
  padding: 10px 20px calc(14px + env(safe-area-inset-bottom, 0px));
  border-top: 1px solid var(--border);
  background: var(--surface);
  display: grid; gap: 9px;
}
.status {
  margin: 0;
  font-size: .75rem; line-height: 1.5; color: var(--muted);
}
.status:empty { display: none; }
.status.error { color: var(--danger); }

.remember {
  display: flex; align-items: center; gap: 8px;
  font-size: .75rem; color: var(--muted);
  cursor: pointer; user-select: none;
}
.remember input { position: absolute; opacity: 0; pointer-events: none; }
.switch {
  width: 34px; height: 19px; flex: none; border-radius: 999px;
  background: var(--border-strong); position: relative;
  transition: background-color .18s ease;
}
.switch::after {
  content: ""; position: absolute; top: 2px; left: 2px;
  width: 15px; height: 15px; border-radius: 999px;
  background: #fff; box-shadow: 0 1px 2px rgb(0 0 0 / .25);
  transition: transform .18s cubic-bezier(.4, 0, .2, 1);
}
.remember input:checked + .switch { background: var(--sf-accent); }
.remember input:checked + .switch::after { transform: translateX(15px); }
.remember input:focus-visible + .switch { box-shadow: 0 0 0 3px rgb(var(--sf-accent-rgb) / .28); }

.buttons { display: grid; grid-template-columns: 1fr 1.35fr; gap: 8px; }
.btn {
  height: 46px; border-radius: 12px;
  display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  border: 1px solid var(--border-strong);
  background: var(--surface); color: var(--text);
  font: 600 .9375rem/1 var(--font);
  cursor: pointer;
  transition: background-color .16s ease, border-color .16s ease, transform .1s ease, box-shadow .16s ease;
}
.btn:hover { background: var(--surface-muted); }
.btn:active { transform: scale(.99); }
.btn:focus-visible { outline: none; box-shadow: 0 0 0 3px rgb(var(--sf-accent-rgb) / .3); }
.btn-primary {
  background: var(--sf-accent); border-color: var(--sf-accent); color: #fff;
  box-shadow: 0 12px 24px -16px rgb(var(--sf-accent-rgb) / 1);
}
.btn-primary:hover { background: var(--sf-accent-hover); border-color: var(--sf-accent-hover); }
.btn[disabled] { opacity: .6; cursor: progress; transform: none; }

/* ---------- 状态 ---------- */
.state { display: none; }
body[data-state="loading"] .state-loading,
body[data-state="expired"] .state-expired { display: block; }
body[data-state="loading"] .live,
body[data-state="expired"] .live { display: none; }
body[data-state="expired"] .sheet-foot { display: none; }

.skel {
  border-radius: 8px;
  background: linear-gradient(90deg, var(--surface-muted) 25%, var(--border) 37%, var(--surface-muted) 63%);
  background-size: 400% 100%;
  animation: shimmer 1.4s ease-in-out infinite;
}
@keyframes shimmer { 0% { background-position: 100% 0; } 100% { background-position: 0 0; } }
.skel-head { display: flex; gap: 12px; align-items: center; margin-bottom: 14px; }
.skel-head .skel:first-child { width: 46px; height: 46px; border-radius: 14px; flex: none; }
.skel-lines { flex: 1; display: grid; gap: 7px; }
.skel-lines .skel { height: 11px; }
.skel-lines .skel:last-child { width: 60%; }
.skel-row { height: 40px; margin-bottom: 8px; }
.skel-btn { height: 46px; border-radius: 12px; }

.expired { text-align: center; padding: 22px 6px 14px; }
.expired .ico {
  width: 44px; height: 44px; margin: 0 auto 12px; border-radius: 999px;
  display: grid; place-items: center;
  color: var(--muted); background: var(--surface-muted);
}
.expired h2 { margin: 0 0 6px; font-size: 1rem; font-weight: 700; }
.expired p { margin: 0; font-size: .8125rem; color: var(--muted); line-height: 1.7; }
.expired code {
  display: block; margin-top: 10px;
  font-family: var(--mono); font-size: .65625rem; color: var(--faint);
  word-break: break-all;
}
.expired code:empty { display: none; }

.noscript {
  margin: 12px 0 0;
  font-size: .8125rem; color: var(--danger); text-align: center;
}

/* ---------- 桌面：收成居中面板 ---------- */
@media (min-width: 760px) {
  .scene { place-items: center; }
  .brandline { top: 28px; font-size: 13px; }
  .sheet {
    align-self: center;
    max-width: 430px;
    border-radius: 22px;
    max-height: none;
    box-shadow: 0 0 0 1px rgb(0 0 0 / .05), 0 40px 90px -50px rgb(0 0 0 / .45);
  }
  .handle { margin-top: 12px; }
  .sheet-body { padding: 16px 24px 14px; }
  .sheet-foot { padding: 12px 24px 20px; }
}
@media (min-width: 760px) {
  :root[data-mode="dark"] .sheet { box-shadow: 0 0 0 1px rgb(255 255 255 / .08), 0 50px 110px -50px rgb(0 0 0 / .95); }
}
</style>
</head>
<body data-state="loading">

<div class="scene">
  <div class="backdrop" aria-hidden="true">
    <span class="glow glow-a"></span>
    <span class="glow glow-b"></span>
    <a class="brandline" id="site-brand" href="/" target="_blank" rel="noopener noreferrer" aria-label="打开站点首页（新标签页）">
      <span class="brand-icon-wrap" aria-hidden="true">
        <img class="brand-icon" id="site-icon" src="/brand/sforum-logo.svg" alt="" referrerpolicy="no-referrer" decoding="async">
        <span class="mark" id="site-mark" hidden>S</span>
      </span>
      <span class="brand-name" id="site-name">SForum</span>
    </a>
  </div>

  <section class="sheet" role="dialog" aria-modal="true" aria-labelledby="sheet-title">
    <div class="handle" aria-hidden="true"></div>

    <div class="sheet-body">
      <div class="state state-loading" aria-hidden="true">
        <div class="skel-head"><div class="skel"></div><div class="skel-lines"><div class="skel"></div><div class="skel"></div></div></div>
        <div class="skel skel-row"></div>
        <div class="skel skel-row"></div>
        <div class="skel skel-btn"></div>
      </div>

      <div class="state state-expired">
        <div class="expired">
          <div class="ico">
            <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 7.5V13l3.5 2"/></svg>
          </div>
          <h2>授权已失效</h2>
          <p>这次授权请求已过期或已被使用，请回到应用重新发起登录。</p>
          <code id="expired-reason"></code>
        </div>
      </div>

      <div class="live">
        <div class="head">
          <div class="tile" id="client-tile" aria-hidden="true"><span class="tile__mark">SF</span></div>
          <div class="head-text">
            <h1 id="sheet-title">
              <span id="client-name">第三方应用</span>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2 4 6v6c0 5 3.4 9.4 8 10 4.6-.6 8-5 8-10V6l-8-4Z"/><path d="m9 12 2 2 4-4"/></svg>
            </h1>
            <p id="client-desc">请求访问你的 SForum 账号</p>
          </div>
          <button class="close" type="button" id="close" aria-label="取消授权">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M6 6l12 12M18 6 6 18"/></svg>
          </button>
        </div>

        <div class="account">
          <span class="avatar" id="user-avatar" aria-hidden="true"></span>
          <div class="account-id">
            <b id="user-name">当前账号</b>
            <span class="handle-name" id="user-handle"></span>
          </div>
          <a class="switch-account" href="/login" hidden id="switch-account">切换账号</a>
        </div>

        <div class="sec-head">
          <div class="sec-title">此应用将获得</div>
          <div class="sec-count"><b id="scope-count">0</b> 项权限</div>
        </div>

        <div class="perms">
          <button class="perm-toggle" type="button" id="perm-toggle" aria-expanded="false" aria-controls="perm-list">
            <span class="badge" id="perm-badge" aria-hidden="true">0</span>
            <span class="t"><b id="perm-title">读取账号资料</b><small id="perm-summary"></small></span>
            <span class="chev" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>
            </span>
          </button>
          <div class="perm-list" id="perm-list" data-open="false"></div>
        </div>

        <p class="note">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="4" y="10" width="16" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg>
          <span>应用不会获得你的密码。授权后可随时在 <a href="/settings/oauth-apps">设置 → OAuth 应用 → 已授权</a> 中撤销。</span>
        </p>
      </div>
    </div>

    <div class="sheet-foot">
      <p class="status" id="status" role="status" aria-live="polite"></p>
      <label class="remember">
        <input type="checkbox" id="remember" checked>
        <span class="switch" aria-hidden="true"></span>
        <span>记住这次授权，之后不再询问</span>
      </label>
      <div class="buttons">
        <button class="btn" type="button" id="deny">拒绝</button>
        <button class="btn btn-primary" type="button" id="allow">允许访问</button>
      </div>
    </div>

    <noscript>
      <p class="noscript">这个页面需要 JavaScript 才能完成授权，请开启后重试。</p>
    </noscript>
  </section>
</div>

<script type="module">
const requestToken = __REQUEST_TOKEN_JSON__
const returnURL = __RETURN_URL_JSON__

const statusEl = document.getElementById('status')
const allowButton = document.getElementById('allow')
const denyButton = document.getElementById('deny')
const closeButton = document.getElementById('close')
const toggleButton = document.getElementById('perm-toggle')
const listEl = document.getElementById('perm-list')

let scopes = []
let expanded = false

const ICONS = {
  profile: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/></svg>',
  email: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3 7 9 6 9-6"/></svg>',
  avatar: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="3" width="18" height="18" rx="4"/><circle cx="12" cy="10" r="3"/><path d="M6.5 19a6 6 0 0 1 11 0"/></svg>',
  'topic.count': '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 8h8M8 12h8M8 16h5"/></svg>',
  'comment.count': '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.5a3 3 0 0 1-3 3H8.5L4 21V6a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3Z"/><path d="M8 9h8M8 13h5"/></svg>',
  'users.groups': '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="9" cy="8" r="3.2"/><path d="M3 20a6 6 0 0 1 12 0"/><path d="M16 5.6a3 3 0 0 1 0 5.6"/><path d="M17.6 14.4A5.2 5.2 0 0 1 21 19.2"/></svg>',
  'topics.read': '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 5h16M4 12h16M4 19h10"/></svg>',
  'bookmarks.read': '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 4h12v17l-6-4-6 4V4Z"/></svg>',
  'notify.write': '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 8a6 6 0 1 0-12 0c0 7-3 8-3 8h18s-3-1-3-8"/><path d="M10.5 21a2 2 0 0 0 3 0"/></svg>',
  default: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 3 4 7v5c0 5 3.4 9 8 9s8-4 8-9V7l-8-4Z"/></svg>'
}

function iconFor(key) {
  return ICONS[key] || ICONS.default
}


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
  // 决策载荷可能不是字符串（错误对象 / 布尔值），绝不能把它渲染进状态条。
  const value = typeof text === 'string' ? text : ''
  statusEl.textContent = value
  statusEl.className = value && isError ? 'status error' : 'status'
}

function setBusy(busy) {
  allowButton.disabled = busy
  denyButton.disabled = busy
  closeButton.disabled = busy
}

function setState(name) {
  document.body.dataset.state = name
}

/* ---------- 站点品牌 / 主题 / 色彩模式 ----------
   主题表与浅色背景表是宿主 apps/web 外观契约的镜像：
   - 预设值来源 apps/web/app/assets/css/main.css 的 data-sforum-theme 块
   - custom:#rrggbb 混合规则来源 apps/web/app/utils/settings/appearance.ts
   宿主新增预设时这里需要同步更新（NOTES.md 有记录）。 */
const THEME_PRESETS = {
  pine_teal: { accent: '#0f766e', hover: '#0b5f59', soft: '#e6f4f1', dark: '#2dd4bf', rgb: '15 118 110' },
  ocean_blue: { accent: '#2563eb', hover: '#1d4ed8', soft: '#eff6ff', dark: '#60a5fa', rgb: '37 99 235' },
  violet: { accent: '#7c3aed', hover: '#6d28d9', soft: '#f3e8ff', dark: '#a78bfa', rgb: '124 58 237' },
  rose: { accent: '#e11d48', hover: '#be123c', soft: '#fff1f2', dark: '#fb7185', rgb: '225 29 72' },
  amber: { accent: '#d97706', hover: '#b45309', soft: '#fffbeb', dark: '#fbbf24', rgb: '217 119 6' }
}

const LIGHT_BACKGROUNDS = {
  pure_white: { background: '#ffffff', surface: '#ffffff', muted: '#f5f5f8', border: '#e9e8ed' },
  porcelain: { background: '#f8faf9', surface: '#ffffff', muted: '#eff3f1', border: '#dce5e1' },
  paper: { background: '#f7f5ef', surface: '#fffdf8', muted: '#f1ede4', border: '#ded8cb' },
  parchment: { background: '#f4eedf', surface: '#fcf8ef', muted: '#ebe2cf', border: '#d8ccb5' },
  mist_gray: { background: '#f3f5f7', surface: '#ffffff', muted: '#eaedf1', border: '#dde1e6' },
  cool_frost: { background: '#f4f7fa', surface: '#fbfcfe', muted: '#eaf0f5', border: '#d9e2ea' },
  cloud_blue: { background: '#f1f6fb', surface: '#f9fcff', muted: '#e6eff8', border: '#d3e0ec' },
  mint_mist: { background: '#f1f8f5', surface: '#fbfffd', muted: '#e4f0eb', border: '#cfe1d9' },
  sage: { background: '#f2f5ef', surface: '#fbfcfa', muted: '#e6ebe1', border: '#d2dccb' },
  sakura: { background: '#fcf4f5', surface: '#fffbfb', muted: '#f7e7e9', border: '#ebd4d8' },
  lilac_mist: { background: '#f7f4fa', surface: '#fdfbff', muted: '#ede7f3', border: '#dfd4e8' },
  morning_apricot: { background: '#fcf4eb', surface: '#fffaf5', muted: '#f6e7d7', border: '#ebd5bd' }
}

const CUSTOM_THEME_PREFIX = 'custom:'
const RECOMMENDED_THEME = 'pine_teal'
const RECOMMENDED_LIGHT_BACKGROUND = 'pure_white'

function firstNonEmpty() {
  for (const value of arguments) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function normalizeHexColor(value) {
  const raw = String(value || '').trim().toLowerCase().replace(/^#/, '')
  return /^[0-9a-f]{6}$/.test(raw) ? '#' + raw : null
}

function hexToRgb(hex) {
  const value = (normalizeHexColor(hex) || '#52525b').slice(1)
  return {
    r: parseInt(value.slice(0, 2), 16),
    g: parseInt(value.slice(2, 4), 16),
    b: parseInt(value.slice(4, 6), 16)
  }
}

function mixHex(from, to, amount) {
  const start = hexToRgb(from)
  const end = hexToRgb(to)
  const mix = function (a, b) { return Math.round(a + (b - a) * amount) }
  const toHex = function (n) { return n.toString(16).padStart(2, '0') }
  return '#' + toHex(mix(start.r, end.r)) + toHex(mix(start.g, end.g)) + toHex(mix(start.b, end.b))
}

// 解析单个主题值；无法解析时返回空串，交给下一级来源。
function normalizeThemeValue(value) {
  const raw = String(value || '').trim().toLowerCase()
  if (Object.prototype.hasOwnProperty.call(THEME_PRESETS, raw)) return raw
  let colorSource = ''
  if (raw.indexOf(CUSTOM_THEME_PREFIX) === 0) colorSource = raw.slice(CUSTOM_THEME_PREFIX.length)
  else if (/^#?[0-9a-f]{6}$/.test(raw)) colorSource = raw.replace(/^#/, '')
  const color = normalizeHexColor(colorSource)
  if (color) return CUSTOM_THEME_PREFIX + color
  return ''
}

function normalizeLightBackgroundValue(value) {
  const raw = String(value || '').trim().toLowerCase()
  return Object.prototype.hasOwnProperty.call(LIGHT_BACKGROUNDS, raw) ? raw : ''
}

function applyTheme(theme, background) {
  const root = document.documentElement
  root.setAttribute('data-site-theme', theme)
  if (theme.indexOf(CUSTOM_THEME_PREFIX) === 0) {
    const accent = theme.slice(CUSTOM_THEME_PREFIX.length)
    const rgb = hexToRgb(accent)
    root.style.setProperty('--sf-accent', accent)
    root.style.setProperty('--sf-accent-hover', mixHex(accent, '#000000', 0.16))
    root.style.setProperty('--sf-accent-soft-light', mixHex(accent, '#ffffff', 0.92))
    root.style.setProperty('--sf-accent-dark', mixHex(accent, '#ffffff', 0.32))
    root.style.setProperty('--sf-accent-rgb', rgb.r + ' ' + rgb.g + ' ' + rgb.b)
  } else {
    const preset = THEME_PRESETS[theme]
    root.style.setProperty('--sf-accent', preset.accent)
    root.style.setProperty('--sf-accent-hover', preset.hover)
    root.style.setProperty('--sf-accent-soft-light', preset.soft)
    root.style.setProperty('--sf-accent-dark', preset.dark)
    root.style.setProperty('--sf-accent-rgb', preset.rgb)
  }
  root.setAttribute('data-light-background', background)
  const palette = LIGHT_BACKGROUNDS[background]
  root.style.setProperty('--lb-bg', palette.background)
  root.style.setProperty('--lb-surface', palette.surface)
  root.style.setProperty('--lb-muted', palette.muted)
  root.style.setProperty('--lb-border', palette.border)
}

// 优先级：用户外观偏好 > 站点外观设置 > 推荐默认值。任一级无效值自动落到下一级。
function applyThemeFromSources(session, values) {
  const appearance = session && session.appearance && typeof session.appearance === 'object' ? session.appearance : null
  const theme = normalizeThemeValue(appearance && appearance.theme)
    || normalizeThemeValue(values['appearance.theme'])
    || RECOMMENDED_THEME
  const background = normalizeLightBackgroundValue(appearance && appearance.lightBackground)
    || normalizeLightBackgroundValue(values['appearance.light_background'])
    || RECOMMENDED_LIGHT_BACKGROUND
  applyTheme(theme, background)
}

function applyColorMode() {
  let preference = ''
  try { preference = localStorage.getItem('nuxt-color-mode') || '' } catch (error) { preference = '' }
  let mode = 'light'
  if (preference === 'dark') mode = 'dark'
  else if (preference === 'light') mode = 'light'
  else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) mode = 'dark'
  document.documentElement.setAttribute('data-mode', mode)
}

if (window.matchMedia) {
  const darkQuery = window.matchMedia('(prefers-color-scheme: dark)')
  const modeHandler = function () { applyColorMode() }
  if (darkQuery.addEventListener) darkQuery.addEventListener('change', modeHandler)
  else if (darkQuery.addListener) darkQuery.addListener(modeHandler)
}
window.addEventListener('storage', function (event) {
  if (!event || event.key === null || event.key === 'nuxt-color-mode') applyColorMode()
})

function normalizeHTTPURL(value) {
  const raw = String(value || '').trim()
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
      return parsed.toString().replace(/\/+$/, '')
    }
  } catch (error) {}
  return ''
}

// 品牌资源 URL：相对路径基于站点 URL 解析；javascript:/data: 一律拒绝。
function resolveBrandURL(value, baseURL) {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const lower = raw.toLowerCase()
  if (lower.indexOf('javascript:') === 0 || lower.indexOf('data:') === 0) return ''
  try {
    return new URL(raw, baseURL || location.origin).toString()
  } catch (error) {
    return ''
  }
}

function optionMapFrom(payload) {
  const values = {}
  if (!payload || typeof payload !== 'object') return values
  const items = Array.isArray(payload) ? payload : []
  for (const item of items) {
    if (item && typeof item.name === 'string') {
      values[item.name] = typeof item.value === 'string' ? item.value : ''
    }
  }
  return values
}

function firstRune(text) {
  const source = String(text || '').trim()
  for (const rune of source) return rune
  return 'S'
}

function setBrandIcon(url) {
  const img = document.getElementById('site-icon')
  const mark = document.getElementById('site-mark')
  const showMark = function () { img.hidden = true; mark.hidden = false }
  const showImage = function () { img.hidden = false; mark.hidden = true }
  if (!url) { showMark(); return }
  img.onload = showImage
  img.onerror = showMark
  if (img.getAttribute('src') !== url) img.setAttribute('src', url)
  if (img.complete && img.naturalWidth === 0) showMark()
  else if (img.complete && img.naturalWidth > 0) showImage()
}

function applySiteBrand(options) {
  const values = optionMapFrom(options)
  const siteName = firstNonEmpty(values['site.name']) || 'SForum'
  const siteURL = normalizeHTTPURL(values['site.url'])
  const iconURL = resolveBrandURL(
    firstNonEmpty(values['site.favicon_url'], values['site.logo_url'], '/brand/sforum-logo.svg'),
    siteURL
  )

  const brand = document.getElementById('site-brand')
  brand.setAttribute('href', siteURL || '/')
  brand.setAttribute('title', siteName)
  brand.setAttribute('aria-label', '打开 ' + siteName + ' 首页（新标签页）')
  document.getElementById('site-name').textContent = siteName
  document.getElementById('site-mark').textContent = firstRune(siteName)
  // 标签页标题跟随宿主 SEO 约定：解析 seo.page.title_template（仅 pageTitle /
  // seoSiteName 两个变量），seo.site.name 未配置或开启继承时退回 site.name。
  const declaredSeoName = firstNonEmpty(values['seo.site.name'])
  const inheritEnabled = values['seo.site.inherit_site_name'] !== 'disabled'
  const seoSiteName = (inheritEnabled || !declaredSeoName) ? siteName : declaredSeoName
  const titleTemplate = firstNonEmpty(values['seo.page.title_template']) || '{pageTitle} | {seoSiteName}'
  document.title = titleTemplate
    .replace(/\{pageTitle\}/g, '授权确认')
    .replace(/\{seoSiteName\}/g, seoSiteName)
  setBrandIcon(iconURL)
}

// 并行读取宿主公开配置与当前会话；任一失败都降级为 null，页面保持默认外观。
function loadSiteChrome() {
  const fetchJSON = function (url) {
    const controller = new AbortController()
    const timer = setTimeout(function () { controller.abort() }, 2500)
    return fetch(url, {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      signal: controller.signal
    }).then(function (response) {
      return response.json().catch(function () { return {} })
    }).finally(function () { clearTimeout(timer) })
  }
  const unwrap = function (parsed) {
    return parsed && parsed.data && typeof parsed.data === 'object' ? parsed.data : null
  }
  return Promise.all([
    fetchJSON('/api/v1/web-options').then(unwrap).catch(function () { return null }),
    fetchJSON('/api/v1/auth/session').then(unwrap).catch(function () { return null })
  ]).then(function (results) {
    return { options: results[0], session: results[1] }
  })
}

/* ---------- 账号头像：与站点 SFAvatar 完全一致 ----------
   色板与稳定哈希复刻 apps/web/app/utils/forum/forumListPresentation.ts，
   首字规则复刻 SFAvatar.vue（中文取首字，英文按空白拆词取词首）。 */
const AVATAR_TONES = ['#5c7cfa', '#20c997', '#fab005', '#cc5de8', '#339af0', '#ff6b6b', '#845ef7', '#51cf66']

function avatarTone(seed) {
  let hash = 0
  const source = String(seed || 'user')
  for (let index = 0; index < source.length; index += 1) {
    hash = (hash * 31 + source.charCodeAt(index)) >>> 0
  }
  return AVATAR_TONES[hash % AVATAR_TONES.length] || AVATAR_TONES[0]
}

function avatarInitials(name) {
  const source = String(name || '').trim()
  if (!source) return 'U'
  const first = source.charAt(0)
  if (/[\u3400-\u9fff\uf900-\ufaff]/.test(first)) return first
  return source.split(/\s+/).slice(0, 2).map(function (part) { return part.slice(0, 1) }).join('').toUpperCase()
}

function renderUserAvatar(element, text, tone, imageURL) {
  element.textContent = ''
  const fallback = document.createElement('span')
  fallback.className = 'avatar__fallback'
  fallback.textContent = text
  fallback.style.background = tone
  element.append(fallback)
  if (imageURL) {
    const img = document.createElement('img')
    img.className = 'avatar__img'
    img.src = imageURL
    img.alt = ''
    img.referrerPolicy = 'no-referrer'
    img.decoding = 'async'
    img.addEventListener('error', function () { img.remove() })
    element.append(img)
  }
}

function renderAccount(summaryUser, session) {
  const summaryId = summaryUser && typeof summaryUser.id === 'number' ? summaryUser.id : 0
  const displayName = firstNonEmpty(
    summaryUser && summaryUser.displayName, summaryUser && summaryUser.username,
    session && session.displayName, session && session.username
  ) || '当前账号'
  const handleName = firstNonEmpty(summaryUser && summaryUser.username, session && session.username)
  document.getElementById('user-name').textContent = displayName
  document.getElementById('user-handle').textContent = handleName ? '@' + handleName : ''

  // 会话头像只在确认页身份与会话一致时使用，避免展示错人的头像。
  const sameUser = Boolean(session && typeof session.id === 'number' && summaryId > 0
    && Number(session.id) === Number(summaryId))
  const avatarView = sameUser && session.avatar && typeof session.avatar === 'object' ? session.avatar : null
  const imageURL = avatarView && typeof avatarView.url === 'string' && avatarView.url
    ? resolveBrandURL(avatarView.url, '')
    : ''
  const seed = firstNonEmpty(summaryUser && summaryUser.displayName, summaryUser && summaryUser.username, handleName)
  renderUserAvatar(
    document.getElementById('user-avatar'),
    avatarInitials(seed),
    avatarTone(seed),
    imageURL
  )
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

function renderScopes() {
  const count = scopes.length
  document.getElementById('scope-count').textContent = count
  document.getElementById('perm-badge').textContent = count
  document.getElementById('perm-title').textContent = count === 1 ? '读取 1 项资料' : '读取 ' + count + ' 项资料'
  document.getElementById('perm-summary').textContent = scopes
    .map(function (scope) { return scope.label || scope.key })
    .join('、')

  listEl.textContent = ''
  for (const scope of scopes) {
    const row = document.createElement('div')
    row.className = 'perm'

    const icon = document.createElement('div')
    icon.className = 'perm-icon'
    icon.innerHTML = iconFor(scope.key)

    const text = document.createElement('div')
    text.className = 'perm-text'
    const label = document.createElement('b')
    label.textContent = scope.label || scope.key
    const code = document.createElement('code')
    code.textContent = scope.key
    text.appendChild(label)
    text.appendChild(code)

    row.appendChild(icon)
    row.appendChild(text)
    listEl.appendChild(row)
  }
}

// 统一 icon 渲染：后端 summary 下发 icon 描述符（与前台卡片、管理端列表同源）。
// 生成形态 = 方型浅灰底 + 5×5 GitHub 风格 identicon 像素块（单色，无文字）；
// logo <img> 叠在上面，加载失败移除 img 露出生成图标；描述符缺失时用本地镜像兜底。
// 本地镜像（stableHash / identiconPattern / identiconColor）与 backend/identicon.go 逐位一致。
const ICON_BACKGROUND = '#f0f0f0'

function stableHash(seed) {
  let hash = 0
  const source = String(seed == null ? '' : seed)
  for (let index = 0; index < source.length; index += 1) {
    hash = (Math.imul(hash, 31) + source.charCodeAt(index)) >>> 0
  }
  return hash
}

// 与 backend/identicon.go 的 identiconColor 逐字一致（S/L 取自 GitHub 实测区间）。
function identiconColor(seed) {
  const base = String(seed == null ? '' : seed).trim() || 'sforum'
  return 'hsl(' + (stableHash(base) % 360) + ' 60% 63%)'
}

function identiconPattern(seed) {
  const base = String(seed == null ? '' : seed).trim() || 'sforum'
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

function renderIdenticonSVG(pattern, color) {
  const namespace = 'http://www.w3.org/2000/svg'
  const svg = document.createElementNS(namespace, 'svg')
  svg.setAttribute('class', 'tile__ident')
  svg.setAttribute('viewBox', '0 0 5 5')
  svg.setAttribute('aria-hidden', 'true')
  const path = document.createElementNS(namespace, 'path')
  let data = ''
  for (let index = 0; index < 25; index += 1) {
    if (pattern.charAt(index) !== '1') continue
    data += 'M' + (index % 5) + ' ' + Math.floor(index / 5) + 'h1v1h-1z'
  }
  path.setAttribute('d', data)
  path.setAttribute('fill', color)
  path.setAttribute('shape-rendering', 'crispEdges')
  svg.append(path)
  return svg
}

function renderClientIcon(data) {
  const tile = document.getElementById('client-tile')
  tile.textContent = ''
  const icon = data.icon && data.icon.kind ? data.icon : null
  const name = data.clientName || data.clientId || ''
  const seed = firstNonEmpty(data.clientId, name)
  const mark = document.createElement('span')
  mark.className = 'tile__mark'
  const pattern = icon && typeof icon.pattern === 'string' && icon.pattern.length === 25
    ? icon.pattern
    : identiconPattern(seed)
  mark.style.background = (icon && icon.background) || ICON_BACKGROUND
  mark.append(renderIdenticonSVG(pattern, (icon && icon.color) || identiconColor(seed)))
  tile.append(mark)
  if (icon && icon.url) {
    const img = document.createElement('img')
    img.className = 'tile__img'
    img.src = icon.url
    img.alt = ''
    img.referrerPolicy = 'no-referrer'
    img.addEventListener('error', function () { img.remove() })
    tile.append(img)
  }
}

function renderIdentity(data, session) {
  const clientName = data.clientName || data.clientId || '第三方应用'
  document.getElementById('client-name').textContent = clientName
  renderClientIcon(data)
  document.getElementById('client-desc').textContent = data.clientDescription || '请求访问你的 SForum 账号'
  renderAccount(data.user || {}, session)
  if (data.clientName || data.clientId) {
    document.getElementById('sheet-title').setAttribute('aria-label', clientName + ' 请求访问你的 SForum 账号')
  }
}

const chromePromise = loadSiteChrome()
const initialSummary = fetchSummary().catch(function () { return null })

async function loadSummary() {
  setState('loading')
  setBusy(true)
  const chrome = await chromePromise
  applySiteBrand(chrome.options)
  applyThemeFromSources(chrome.session, optionMapFrom(chrome.options))

  // 页面正文由 stream 模式输出，紧随其后的首次数据请求偶尔会撞上运行时切换，
  // 因此失败时先重试一次，再显示真实原因。
  let result = await initialSummary
  if (!result || !result.ok || result.data.status !== 'ok') {
    await new Promise(function (resolve) { setTimeout(resolve, 600) })
    result = await fetchSummary()
  }
  const data = result.data
  if (!result.ok || data.status !== 'ok') {
    document.getElementById('expired-reason').textContent =
      result.reason ? 'HTTP ' + result.status + ' · ' + result.reason : ''
    setState('expired')
    return
  }
  scopes = Array.isArray(data.scopes) ? data.scopes : []
  renderIdentity(data, chrome.session)
  renderScopes()
  setState('ok')
  setBusy(false)
  allowButton.focus({ preventScroll: true })
}

async function decide(decision) {
  setBusy(true)
  setStatus(decision === 'allow' ? '正在授权…' : '正在返回应用…')
  const controller = new AbortController()
  const timer = setTimeout(function () { controller.abort() }, 10000)
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
        decision: decision,
        remember: document.getElementById('remember').checked
      }),
      signal: controller.signal
    })
    clearTimeout(timer)
    const parsed = await response.json().catch(function () { return {} })
    const data = unwrapDocument(parsed)
    const target = data && typeof data.location === 'string' ? data.location : ''
    if (data && data.status === 'redirect' && target) {
      window.location.replace(target)
      return
    }
    // 拒绝失败保持中性提示；只有允许失败才用红色错误样式。
    setStatus(firstNonEmpty(
      typeof (data && data.errorDescription) === 'string' ? data.errorDescription : '',
      typeof (data && data.error) === 'string' ? data.error : '',
      '处理失败，请重试。'
    ), decision === 'allow')
    setBusy(false)
  } catch (error) {
    clearTimeout(timer)
    setStatus(error && error.name === 'AbortError' ? '请求超时，请重试。' : '网络错误，请重试。', decision === 'allow')
    setBusy(false)
  }
}

toggleButton.addEventListener('click', function () {
  expanded = !expanded
  listEl.dataset.open = String(expanded)
  toggleButton.setAttribute('aria-expanded', String(expanded))
})

allowButton.addEventListener('click', function () { decide('allow') })
denyButton.addEventListener('click', function () { decide('deny') })
closeButton.addEventListener('click', function () { decide('deny') })

loadSummary().catch(function () {
  document.getElementById('expired-reason').textContent = ''
  setState('expired')
})

// 「切换账号」= 退出当前会话，再回登录页；登录成功后回到同一个授权请求。
// 只接受带查询串的本站授权端点：裸端点没有 client_id，回跳只会得到一个错误页。
function hasAuthorizeReturn(value) {
  return typeof value === 'string' && value.indexOf('/api/oauth/authorize?') === 0
}

// csrf_ cookie 可能尚未种下（例如用户直接打开确认页链接）：先打一次安全 GET 让宿主种。
function primeCsrf() {
  if (readCookie('csrf_')) return Promise.resolve()
  return fetch('/api/v1/health', {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' }
  }).catch(function () {})
}

async function switchAccount() {
  setBusy(true)
  setStatus('正在退出当前账号…')
  try {
    await primeCsrf()
    const response = await fetch('/api/v1/auth/logout', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Accept': 'application/json',
        'X-Csrf-Token': readCookie('csrf_')
      }
    })
    // 401 = 会话已经失效，照样去登录页；其余失败留在当前页重试。
    if (!response.ok && response.status !== 401) {
      throw new Error('logout failed')
    }
    window.location.assign('/login?redirect=' + encodeURIComponent(returnURL))
  } catch (error) {
    setStatus('切换账号失败，请重试。', true)
    setBusy(false)
  }
}

if (hasAuthorizeReturn(returnURL)) {
  // 保留回跳地址：用于「切换账号」后回到同一授权请求。
  const switcher = document.getElementById('switch-account')
  switcher.href = '/login?redirect=' + encodeURIComponent(returnURL)
  switcher.hidden = false
  switcher.addEventListener('click', function (event) {
    event.preventDefault()
    void switchAccount()
  })
}
</script>
</body>
</html>
`

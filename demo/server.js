#!/usr/bin/env node
/**
 * OAuth2 Demo 服务 — 测试 SForum OAuth2 Provider 的授权码 + PKCE 流程
 *
 * 零依赖（仅用 Node 内置模块）。流程：
 *   1. 打开 http://127.0.0.1:9000 → 点击「使用 SForum 登录」
 *   2. 跳到授权服务器 →（登录 + 确认页）→ 回调带 ?code=
 *   3. demo 后端做 CSRF 握手后 POST /api/oauth/token 换令牌
 *   4. GET /api/oauth/userinfo?access_token=… 拿用户资料并展示
 *
 * 环境变量（或写入同目录 .env，格式 KEY=VALUE）：
 *   SFO_BASE_URL   授权服务器地址，默认 http://127.0.0.1:3000
 *   CLIENT_ID      sfo_client_…（后台「OAuth2 应用」创建，回调地址填
 *                  http://127.0.0.1:9000/callback）
 *   CLIENT_SECRET  sfos_…（公开客户端则留空，强制 PKCE）
 *   SCOPES         申请的权限范围（空格分隔，如 "profile avatar"）。留空则不传
 *                  scope —— 由授权服务器回落到应用在后台登记的范围。
 *   PORT           demo 监听端口，默认 9000
 */

'use strict';

const http = require('http');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const { URL, URLSearchParams } = require('url');

/* ---------- 配置 ---------- */

function loadDotEnv() {
  const file = path.join(__dirname, '.env');
  if (!fs.existsSync(file)) return;
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const m = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$/);
    if (m && !(m[1] in process.env)) process.env[m[1]] = m[2];
  }
}
loadDotEnv();

const CONFIG = {
  baseUrl: (process.env.SFO_BASE_URL || 'http://127.0.0.1:3000').replace(/\/+$/, ''),
  clientId: process.env.CLIENT_ID || '',
  clientSecret: process.env.CLIENT_SECRET || '',
  // 留空表示不在 authorize 请求里带 scope，让 provider 用应用登记的范围作默认值。
  scopes: (process.env.SCOPES || '').trim(),
  port: Number(process.env.PORT || 9000),
  redirectUri: '', // 在 main() 里根据端口生成
};

/* ---------- 内存会话 ---------- */

/** @type {Map<string, {verifier:string, createdAt:number}>} */
const sessions = new Map(); // state -> { verifier }
/** 最近一次成功结果，刷新首页可见 */
let lastResult = null;

/* ---------- PKCE ---------- */

function base64url(buf) {
  return Buffer.from(buf).toString('base64url'); // Node ≥15.7；RFC 7636 无填充
}

function makePkce() {
  const verifier = base64url(crypto.randomBytes(32)); // 43 字符
  const challenge = base64url(crypto.createHash('sha256').update(verifier).digest());
  return { verifier, challenge };
}

/* ---------- HTTP 辅助 ---------- */

function send(res, status, body, headers = {}) {
  const data = typeof body === 'string' ? body : JSON.stringify(body);
  res.writeHead(status, {
    'Content-Type': typeof body === 'string' && body.trim().startsWith('<') ? 'text/html; charset=utf-8' : 'application/json; charset=utf-8',
    ...headers,
  });
  res.end(data);
}

/** 手写最小 form 编码（避免依赖 querystring 之外的假设；JSON body 用 JSON.stringify） */
function formEncode(obj) {
  return new URLSearchParams(obj).toString();
}

/** 服务端到服务端调用：先 CSRF 插手，再带 X-Csrf-Token + Origin 发 JSON POST */
async function oauthPost(jsonPath, bodyObj) {
  // 1) CSRF 握手：GET 会种下 csrf_ cookie（非 HttpOnly）
  const handshake = await fetch(`${CONFIG.baseUrl}/api/oauth/csrf`, { redirect: 'manual' });
  const setCookies = handshake.headers.getSetCookie ? handshake.headers.getSetCookie() : [];
  let csrfToken = '';
  const cookieParts = [];
  for (const raw of setCookies) {
    const [pair] = raw.split(';');
    const eq = pair.indexOf('=');
    cookieParts.push(pair);
    if (pair.slice(0, eq) === 'csrf_') csrfToken = decodeURIComponent(pair.slice(eq + 1));
  }
  if (!csrfToken) {
    throw new Error('CSRF 握手失败：未收到 csrf_ cookie（状态码 ' + handshake.status + '）');
  }

  // 2) JSON POST
  const resp = await fetch(`${CONFIG.baseUrl}${jsonPath}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
      Cookie: cookieParts.join('; '),
      'X-Csrf-Token': csrfToken,
      Origin: CONFIG.baseUrl,
    },
    body: JSON.stringify(bodyObj),
  });
  const text = await resp.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* 保持 null */ }
  return { status: resp.status, json, text };
}

/* ---------- 页面 ---------- */

const PAGE = (title, bodyHtml) => `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>${title}</title>
<style>
  body{font-family:system-ui,-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;
       max-width:640px;margin:48px auto;padding:0 16px;color:#1f2328;background:#f6f8fa}
  .card{background:#fff;border:1px solid #d0d7de;border-radius:10px;padding:24px;margin-bottom:16px}
  h1{font-size:20px} h2{font-size:15px;margin:0 0 8px;color:#57606a}
  a.btn{display:inline-block;background:#1f6feb;color:#fff;padding:10px 18px;border-radius:6px;
        text-decoration:none;font-weight:600}
  code,pre{background:#f6f8fa;border:1px solid #d0d7de;border-radius:6px;font-size:13px}
  pre{padding:12px;overflow-x:auto;white-space:pre-wrap;word-break:break-all}
  .kv{display:grid;grid-template-columns:120px 1fr;gap:4px 12px;font-size:14px}
  .kv dt{color:#57606a}.kv dd{margin:0;font-family:monospace;word-break:break-all}
  .ok{color:#1a7f37}.err{color:#cf222e}
  table{border-collapse:collapse;width:100%;font-size:14px}
  td,th{border:1px solid #d0d7de;padding:6px 10px;text-align:left}
  code.inline{padding:1px 6px}
</style></head><body>${bodyHtml}</body></html>`;

function pageHome() {
  const cfgOk = Boolean(CONFIG.clientId);
  return PAGE('OAuth2 Demo', `
  <h1>SForum OAuth2 Demo</h1>
  <div class="card">
    <h2>配置</h2>
    <dl class="kv">
      <dt>授权服务器</dt><dd>${CONFIG.baseUrl}</dd>
      <dt>Client ID</dt><dd>${CONFIG.clientId || '<span class="err">未配置 — 在 demo/.env 里填</span>'}</dd>
      <dt>Client Secret</dt><dd>${CONFIG.clientSecret ? '已配置（机密客户端）' : '未配置（公开客户端，强制 PKCE）'}</dd>
      <dt>回调地址</dt><dd>${CONFIG.redirectUri}</dd>
      <dt>申请权限</dt><dd>${CONFIG.scopes ? escapeHtml(CONFIG.scopes) : '未指定（由应用在后台登记的范围决定）'}</dd>
    </dl>
    ${cfgOk ? '' : '<p class="err">请先在后台创建应用：回调地址填 <code class="inline">' + CONFIG.redirectUri + '</code>，把 client_id（和 client_secret，如有）写入 demo/.env 后重启本服务。</p>'}
  </div>
  <div class="card">
    <h2>开始</h2>
    ${cfgOk ? `<a class="btn" href="/login">使用 SForum 登录</a>` : `<button class="btn" disabled style="opacity:.5">请先配置 Client ID</button>`}
    <p style="color:#57606a;font-size:13px">流程：/login 生成 PKCE → 跳授权服务器 → 登录并确认 → 回调 /callback 带 code → 后端换令牌 → 读 userinfo。</p>
  </div>
  ${renderLastResult()}`);
}

function renderLastResult() {
  if (!lastResult) return '';
  const r = lastResult;
  const ok = r.ok;
  return `
  <div class="card">
    <h2>最近一次结果 <span class="${ok ? 'ok' : 'err'}">${ok ? '✓ 成功' : '✗ 失败'}</span> <small style="color:#57606a">${r.at}</small></h2>
    ${ok ? `
    <h2>用户资料（userinfo）</h2>
    ${renderAvatarPreview(r.userinfo)}
    <pre>${escapeHtml(JSON.stringify(r.userinfo, null, 2))}</pre>
    <h2>令牌响应</h2>
    <dl class="kv">
      <dt>access_token</dt><dd>${escapeHtml(mask(r.access_token))}</dd>
      <dt>refresh_token</dt><dd>${escapeHtml(mask(r.refresh_token))}</dd>
      <dt>expires_in</dt><dd>${r.expires_in}s</dd>
      <dt>scope</dt><dd>${escapeHtml(r.scope || '')}</dd>
    </dl>
    <h2>操作</h2>
    <p>
      <a href="/refresh">刷新令牌</a> ·
      <a href="/userinfo">重新读 userinfo</a> ·
      <a href="/revoke">撤销 access_token</a> ·
      <a href="/introspect">内省 access_token</a>
    </p>
    <h2>令牌操作原始响应</h2>
    <pre>${escapeHtml(r.lastActionLog || '(尚未执行操作)')}</pre>
    ${cfgSecretNote()}
    ` : `
    <p class="err">授权失败</p>
    <pre>${escapeHtml(r.error || '')}</pre>`}
  </div>`;
}

function renderAvatarPreview(userinfo) {
  const avatar = userinfo && typeof userinfo.avatar === 'string' ? userinfo.avatar.trim() : '';
  if (!avatar) return '';
  // 授予 avatar 后一定会有值：宿主 URL（上传 / Gravatar / 静态图）或插件生成的 data URL。
  const source = avatar.startsWith('data:') ? '插件生成的默认头像（站点首字头像）' : '宿主头像地址';
  return `<p><img src="${escapeHtml(avatar)}" alt="avatar" width="64" height="64"
    style="border-radius:50%;border:1px solid #d0d7de;vertical-align:middle">
    <small style="color:#57606a">avatar · ${source}</small></p>`;
}

function cfgSecretNote() {
  return CONFIG.clientSecret
    ? '<p style="color:#57606a;font-size:13px">当前为机密客户端：token 请求带 client_secret + PKCE。</p>'
    : '<p style="color:#57606a;font-size:13px">当前为公开客户端：token 请求只带 code_verifier（PKCE）。</p>';
}

function mask(token) {
  if (!token) return '(无)';
  return token.length > 12 ? token.slice(0, 10) + '…' + token.slice(-4) : token;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

/* ---------- 路由 ---------- */

async function handle(req, res) {
  const url = new URL(req.url, `http://${req.headers.host || 'localhost'}`);
  const p = url.pathname;

  try {
    if (p === '/') return send(res, 200, pageHome());

    if (p === '/login') {
      if (!CONFIG.clientId) return send(res, 400, { error: 'missing CLIENT_ID' });
      const { verifier, challenge } = makePkce();
      const state = base64url(crypto.randomBytes(16));
      sessions.set(state, { verifier, createdAt: Date.now() });
      // 清理 10 分钟前的旧 state
      for (const [k, v] of sessions) if (Date.now() - v.createdAt > 600000) sessions.delete(k);

      const params = new URLSearchParams({
        client_id: CONFIG.clientId,
        redirect_uri: CONFIG.redirectUri,
        response_type: 'code',
        state,
        code_challenge: challenge,
        code_challenge_method: 'S256',
      });
      // 不传 scope 时由 provider 回落到应用登记的范围；配置了 SCOPES 才显式申请。
      if (CONFIG.scopes) params.set('scope', CONFIG.scopes);
      res.writeHead(302, { Location: `${CONFIG.baseUrl}/api/oauth/authorize?${params}` });
      return res.end();
    }

    if (p === '/callback') {
      const code = url.searchParams.get('code');
      const error = url.searchParams.get('error');
      const state = url.searchParams.get('state');
      if (error) {
        lastResult = { ok: false, error: `授权服务器返回错误：${error} ${url.searchParams.get('error_description') || ''}`, at: new Date().toLocaleString() };
        return send(res, 200, pageHome());
      }
      if (!code || !state || !sessions.has(state)) {
        lastResult = { ok: false, error: !code ? '回调缺少 code' : 'state 无效或已过期（可能重复使用了授权码链接）', at: new Date().toLocaleString() };
        return send(res, 200, pageHome());
      }
      const { verifier } = sessions.get(state);
      sessions.delete(state); // state 一次性

      const tokenResp = await oauthPost('/api/oauth/token', {
        grant_type: 'authorization_code',
        client_id: CONFIG.clientId,
        ...(CONFIG.clientSecret ? { client_secret: CONFIG.clientSecret } : {}),
        code,
        redirect_uri: CONFIG.redirectUri,
        code_verifier: verifier,
      });
      if (tokenResp.status !== 200 || !tokenResp.json || !tokenResp.json.access_token) {
        lastResult = { ok: false, error: `token 交换失败（HTTP ${tokenResp.status}）：\n${tokenResp.text}`, at: new Date().toLocaleString() };
        return send(res, 200, pageHome());
      }
      const tokens = tokenResp.json;

      const ui = await fetch(`${CONFIG.baseUrl}/api/oauth/userinfo?access_token=${encodeURIComponent(tokens.access_token)}`);
      const uiText = await ui.text();
      let userinfo = null;
      try { userinfo = JSON.parse(uiText); } catch { /* ignore */ }

      lastResult = {
        ok: true,
        at: new Date().toLocaleString(),
        ...tokens,
        userinfo: ui.status === 200 ? userinfo : { _error: `userinfo HTTP ${ui.status}`, body: uiText },
        lastActionLog: '(尚未执行操作)',
      };
      return send(res, 200, pageHome());
    }

    if (p === '/userinfo') return await tokenAction(res, async (at) => {
      const r = await fetch(`${CONFIG.baseUrl}/api/oauth/userinfo?access_token=${encodeURIComponent(at)}`);
      return `GET userinfo → HTTP ${r.status}\n${await r.text()}`;
    });

    if (p === '/refresh') return await tokenAction(res, async (at, rt) => {
      if (!rt) return '(没有 refresh_token 可用)';
      const r = await oauthPost('/api/oauth/token', {
        grant_type: 'refresh_token',
        client_id: CONFIG.clientId,
        ...(CONFIG.clientSecret ? { client_secret: CONFIG.clientSecret } : {}),
        refresh_token: rt,
      });
      if (r.status === 200 && r.json && r.json.access_token) {
        // 刷新令牌轮换：保存新令牌
        Object.assign(lastResult, {
          access_token: r.json.access_token,
          refresh_token: r.json.refresh_token || rt,
          expires_in: r.json.expires_in,
          scope: r.json.scope,
        });
      }
      return `POST token (refresh_token) → HTTP ${r.status}\n${r.text}`;
    });

    if (p === '/revoke') return await tokenAction(res, async (at) => {
      const r = await oauthPost('/api/oauth/revoke', { client_id: CONFIG.clientId, token: at });
      return `POST revoke → HTTP ${r.status}\n${r.text}`;
    });

    if (p === '/introspect') return await tokenAction(res, async (at) => {
      const r = await oauthPost('/api/oauth/introspect', { client_id: CONFIG.clientId, token: at });
      return `POST introspect → HTTP ${r.status}\n${r.text}`;
    });

    send(res, 404, { error: 'not found' });
  } catch (e) {
    send(res, 500, { error: String(e && e.stack || e) });
  }
}

/** 执行一个令牌相关操作，把原始响应写进 lastResult.lastActionLog 并回首页 */
async function tokenAction(res, fn) {
  if (!lastResult || !lastResult.ok || !lastResult.access_token) {
    return send(res, 400, { error: '请先完成一次授权登录' });
  }
  try {
    const log = await fn(lastResult.access_token, lastResult.refresh_token);
    lastResult.lastActionLog = log;
  } catch (e) {
    lastResult.lastActionLog = '操作异常：' + String(e);
  }
  return send(res, 200, pageHome());
}

/* ---------- main ---------- */

function main() {
  CONFIG.redirectUri = `http://127.0.0.1:${CONFIG.port}/callback`;
  const server = http.createServer(handle);
  server.listen(CONFIG.port, '127.0.0.1', () => {
    console.log('OAuth2 demo 已启动');
    console.log(`  首页:      http://127.0.0.1:${CONFIG.port}/`);
    console.log(`  授权服务器: ${CONFIG.baseUrl}`);
    console.log(`  回调地址:   ${CONFIG.redirectUri}  ← 后台创建应用时填这个`);
    if (!CONFIG.clientId) {
      console.log('  ⚠ 未配置 CLIENT_ID：请在后台「OAuth2 应用」创建应用，把密钥写入 demo/.env 后重启。');
    }
  });
}

main();

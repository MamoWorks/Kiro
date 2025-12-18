/**
 * Kiro Web OAuth - Deno 单文件应用
 * 登录用 Web OAuth，刷新用桌面端 API（只需 refreshToken）
 * 运行: deno run --allow-net --allow-env kiro-oauth.ts
 */

import { encode as cborEncode, decode as cborDecode } from "npm:cbor-x@1.6.0";

const KIRO_WEB_PORTAL = "https://app.kiro.dev";
const KIRO_DESKTOP_API = "https://prod.us-east-1.auth.desktop.kiro.dev";
const KIRO_REDIRECT_URI = "https://app.kiro.dev/signin/oauth";
const PORT = 8088;

// ============================================================
// PKCE 工具函数
// ============================================================

function base64UrlEncode(data: Uint8Array): string {
  const base64 = btoa(String.fromCharCode(...data));
  return base64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function generateCodeVerifier(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  return base64UrlEncode(bytes);
}

async function generateCodeChallenge(verifier: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const hash = await crypto.subtle.digest("SHA-256", data);
  return base64UrlEncode(new Uint8Array(hash));
}

// ============================================================
// Web OAuth API (登录用)
// ============================================================

async function initiateLogin(idp: string, codeChallenge: string, state: string) {
  const url = `${KIRO_WEB_PORTAL}/service/KiroWebPortalService/operation/InitiateLogin`;
  const body = cborEncode({
    idp,
    redirectUri: KIRO_REDIRECT_URI,
    codeChallenge,
    codeChallengeMethod: "S256",
    state,
  });

  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/cbor",
      "Accept": "application/cbor",
      "smithy-protocol": "rpc-v2-cbor",
    },
    body,
  });

  if (!response.ok) {
    throw new Error(`InitiateLogin failed: ${response.status}`);
  }

  const bytes = new Uint8Array(await response.arrayBuffer());
  return cborDecode(bytes) as { redirectUrl?: string };
}

async function exchangeToken(
  idp: string,
  code: string,
  codeVerifier: string,
  state: string
) {
  const url = `${KIRO_WEB_PORTAL}/service/KiroWebPortalService/operation/ExchangeToken`;
  const body = cborEncode({
    idp,
    code,
    codeVerifier,
    redirectUri: KIRO_REDIRECT_URI,
    state,
  });

  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/cbor",
      "Accept": "application/cbor",
      "smithy-protocol": "rpc-v2-cbor",
    },
    body,
  });

  // 解析 Set-Cookie
  const cookies: Record<string, string> = {};
  const setCookie = response.headers.get("set-cookie") || "";
  for (const part of setCookie.split(/,(?=[^;]+?=)/)) {
    const match = part.match(/^\s*([^=]+)=([^;]+)/);
    if (match) cookies[match[1]] = match[2];
  }

  const bytes = new Uint8Array(await response.arrayBuffer());

  if (!response.ok) {
    let errorMsg: string;
    try {
      errorMsg = JSON.stringify(cborDecode(bytes));
    } catch {
      errorMsg = new TextDecoder().decode(bytes);
    }
    throw new Error(`ExchangeToken failed (${response.status}): ${errorMsg}`);
  }

  const data = cborDecode(bytes) as {
    accessToken?: string;
    csrfToken?: string;
    expiresIn?: number;
    profileArn?: string;
  };

  return {
    accessToken: data.accessToken || cookies["AccessToken"],
    refreshToken: cookies["RefreshToken"],
    expiresIn: data.expiresIn,
    profileArn: data.profileArn,
  };
}

// ============================================================
// 桌面端 API (刷新用)
// ============================================================

async function refreshTokenDesktop(refreshToken: string) {
  const url = `${KIRO_DESKTOP_API}/refreshToken`;

  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Accept": "application/json",
    },
    body: JSON.stringify({ refreshToken }),
  });

  const data = await response.json();

  if (!response.ok) {
    throw new Error(`RefreshToken failed (${response.status}): ${JSON.stringify(data)}`);
  }

  return data as {
    accessToken: string;
    refreshToken: string;
    expiresIn: number;
    profileArn: string;
  };
}

// ============================================================
// 状态存储
// ============================================================

const pendingAuth: Map<string, { codeVerifier: string; idp: string }> = new Map();

// ============================================================
// HTML 页面
// ============================================================

const HTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Kiro OAuth</title>
  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      min-height: 100vh;
      font-family: -apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Helvetica Neue", Arial, sans-serif;
      background: #ffffff;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 20px;
    }

    .container {
      width: 100%;
      max-width: 520px;
    }

    .card {
      background: rgba(255, 255, 255, 0.72);
      backdrop-filter: blur(20px) saturate(180%);
      -webkit-backdrop-filter: blur(20px) saturate(180%);
      border-radius: 20px;
      border: 1px solid rgba(255, 255, 255, 0.4);
      box-shadow:
        0 8px 32px rgba(0, 0, 0, 0.08),
        0 2px 8px rgba(0, 0, 0, 0.04),
        inset 0 1px 0 rgba(255, 255, 255, 0.6);
      padding: 36px;
      margin-bottom: 16px;
    }

    .logo {
      text-align: center;
      margin-bottom: 28px;
    }

    .logo h1 {
      font-size: 28px;
      font-weight: 600;
      background: linear-gradient(135deg, #1a1a1a 0%, #4a4a4a 100%);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      background-clip: text;
      letter-spacing: -0.5px;
    }

    .logo p {
      color: rgba(0, 0, 0, 0.5);
      font-size: 14px;
      margin-top: 6px;
      font-weight: 400;
    }

    .section {
      margin-bottom: 24px;
    }

    .section:last-child {
      margin-bottom: 0;
    }

    .section-title {
      font-size: 13px;
      font-weight: 600;
      color: rgba(0, 0, 0, 0.5);
      text-transform: uppercase;
      letter-spacing: 0.5px;
      margin-bottom: 12px;
    }

    .btn-group {
      display: flex;
      gap: 10px;
    }

    .btn {
      flex: 1;
      padding: 14px 20px;
      border: none;
      border-radius: 12px;
      font-size: 15px;
      font-weight: 500;
      cursor: pointer;
      transition: all 0.2s ease;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
    }

    .btn-google {
      background: #ffffff;
      color: #1a1a1a;
      border: 1px solid rgba(0, 0, 0, 0.15);
      box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    }

    .btn-google:hover {
      transform: translateY(-1px);
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
      background: #f8f8f8;
    }

    .btn-google:active {
      transform: translateY(0);
    }

    .btn-secondary {
      background: rgba(0, 0, 0, 0.06);
      color: #1a1a1a;
    }

    .btn-secondary:hover {
      background: rgba(0, 0, 0, 0.1);
    }

    .btn-github {
      background: linear-gradient(135deg, #24292e 0%, #1a1a1a 100%);
      color: white;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
    }

    .btn-github:hover {
      transform: translateY(-1px);
      box-shadow: 0 6px 16px rgba(0, 0, 0, 0.3);
    }

    .btn svg {
      width: 18px;
      height: 18px;
    }

    .input-group {
      position: relative;
    }

    .input {
      width: 100%;
      padding: 14px 16px;
      border: 1px solid rgba(0, 0, 0, 0.1);
      border-radius: 12px;
      font-size: 14px;
      font-family: inherit;
      background: rgba(255, 255, 255, 0.6);
      transition: all 0.2s ease;
      color: #1a1a1a;
    }

    .input:focus {
      outline: none;
      border-color: #007AFF;
      box-shadow: 0 0 0 3px rgba(0, 122, 255, 0.15);
      background: rgba(255, 255, 255, 0.9);
    }

    .input::placeholder {
      color: rgba(0, 0, 0, 0.35);
    }

    textarea.input {
      min-height: 100px;
      resize: vertical;
      font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
      font-size: 12px;
      line-height: 1.5;
    }

    .result-box {
      background: rgba(0, 0, 0, 0.04);
      border-radius: 12px;
      padding: 16px;
      margin-top: 12px;
    }

    .result-item {
      margin-bottom: 14px;
    }

    .result-item:last-child {
      margin-bottom: 0;
    }

    .result-label {
      font-size: 11px;
      font-weight: 600;
      color: rgba(0, 0, 0, 0.45);
      text-transform: uppercase;
      letter-spacing: 0.5px;
      margin-bottom: 6px;
    }

    .result-value {
      font-family: "SF Mono", Monaco, "Cascadia Code", monospace;
      font-size: 12px;
      color: #1a1a1a;
      word-break: break-all;
      background: rgba(255, 255, 255, 0.7);
      padding: 10px 12px;
      border-radius: 8px;
      border: 1px solid rgba(0, 0, 0, 0.06);
      max-height: 120px;
      overflow-y: auto;
    }

    .status {
      padding: 10px 14px;
      border-radius: 10px;
      font-size: 13px;
      font-weight: 500;
      margin-top: 12px;
      display: none;
    }

    .status.show {
      display: block;
    }

    .status.success {
      background: rgba(52, 199, 89, 0.15);
      color: #248A3D;
      border: 1px solid rgba(52, 199, 89, 0.3);
    }

    .status.error {
      background: rgba(255, 59, 48, 0.15);
      color: #D70015;
      border: 1px solid rgba(255, 59, 48, 0.3);
    }

    .status.loading {
      background: rgba(0, 122, 255, 0.1);
      color: #007AFF;
      border: 1px solid rgba(0, 122, 255, 0.2);
    }

    .divider {
      height: 1px;
      background: rgba(0, 0, 0, 0.08);
      margin: 24px 0;
    }

    .copy-btn {
      position: absolute;
      right: 8px;
      top: 8px;
      padding: 6px 10px;
      background: rgba(0, 0, 0, 0.06);
      border: none;
      border-radius: 6px;
      font-size: 11px;
      font-weight: 500;
      color: rgba(0, 0, 0, 0.6);
      cursor: pointer;
      transition: all 0.15s ease;
    }

    .copy-btn:hover {
      background: rgba(0, 0, 0, 0.1);
      color: rgba(0, 0, 0, 0.8);
    }

    .footer {
      text-align: center;
      color: rgba(255, 255, 255, 0.7);
      font-size: 12px;
      font-weight: 400;
    }

    .info-box {
      background: rgba(0, 122, 255, 0.08);
      border: 1px solid rgba(0, 122, 255, 0.2);
      border-radius: 10px;
      padding: 12px 14px;
      font-size: 12px;
      color: rgba(0, 0, 0, 0.6);
      margin-top: 12px;
    }

    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.6; }
    }

    .loading-text {
      animation: pulse 1.5s ease-in-out infinite;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="card">
      <div class="logo">
        <h1>Kiro OAuth</h1>
        <p>获取 Refresh Token（可直接刷新 AT）</p>
      </div>

      <div class="section">
        <div class="section-title">选择登录方式</div>
        <div class="btn-group">
          <button class="btn btn-google" onclick="generateUrl('Google')">
            <svg viewBox="0 0 24 24"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/></svg>
            Google
          </button>
          <button class="btn btn-github" onclick="generateUrl('Github')">
            <svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
            Github
          </button>
        </div>
      </div>

      <div id="url-result" class="section" style="display: none;">
        <div class="section-title">登录链接</div>
        <div class="input-group">
          <textarea id="login-url" class="input" readonly placeholder="登录链接将显示在这里..."></textarea>
          <button class="copy-btn" onclick="copyText('login-url')">复制</button>
        </div>
        <button class="btn btn-secondary" style="width: 100%; margin-top: 12px;" onclick="window.open(document.getElementById('login-url').value, '_blank')">
          在浏览器中打开
        </button>
      </div>

      <div class="divider"></div>

      <div class="section">
        <div class="section-title">输入回调 URL</div>
        <div class="input-group">
          <input type="text" id="callback-url" class="input" placeholder="https://app.kiro.dev/signin/oauth?code=...&state=...">
        </div>
        <button class="btn btn-primary" style="width: 100%; margin-top: 12px;" onclick="exchangeToken()">
          获取 Token
        </button>
        <div id="exchange-status" class="status"></div>
      </div>

      <div id="token-result" class="section" style="display: none;">
        <div class="section-title">Token 结果</div>
        <div class="result-box">
          <div class="result-item">
            <div class="result-label">Refresh Token</div>
            <div class="result-value" id="result-rt"></div>
          </div>
        </div>
        <button class="btn btn-secondary" style="width: 100%; margin-top: 12px;" onclick="copyRT()">
          复制 Refresh Token
        </button>
      </div>
    </div>

  </div>

  <script>
    let currentState = null;

    function showStatus(id, type, message) {
      const el = document.getElementById(id);
      el.className = 'status show ' + type;
      el.innerHTML = message;
    }

    function hideStatus(id) {
      document.getElementById(id).className = 'status';
    }

    async function generateUrl(provider) {
      showStatus('exchange-status', 'loading', '<span class="loading-text">生成登录链接中...</span>');
      try {
        const res = await fetch('/api/initiate?provider=' + provider);
        const data = await res.json();
        if (data.error) throw new Error(data.error);

        currentState = data.state;
        document.getElementById('login-url').value = data.authorizeUrl;
        document.getElementById('url-result').style.display = 'block';
        hideStatus('exchange-status');
      } catch (e) {
        showStatus('exchange-status', 'error', '错误: ' + e.message);
      }
    }

    async function exchangeToken() {
      const callbackUrl = document.getElementById('callback-url').value.trim();
      if (!callbackUrl) {
        showStatus('exchange-status', 'error', '请输入回调 URL');
        return;
      }
      if (!currentState) {
        showStatus('exchange-status', 'error', '请先生成登录链接');
        return;
      }

      showStatus('exchange-status', 'loading', '<span class="loading-text">获取 Token 中...</span>');
      try {
        const res = await fetch('/api/exchange', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ callbackUrl, state: currentState })
        });
        const data = await res.json();
        if (data.error) throw new Error(data.error);

        document.getElementById('result-rt').textContent = data.refreshToken || '-';
        document.getElementById('token-result').style.display = 'block';
        showStatus('exchange-status', 'success', '获取成功!');
      } catch (e) {
        showStatus('exchange-status', 'error', '错误: ' + e.message);
      }
    }

    function copyText(id) {
      const text = document.getElementById(id).value || document.getElementById(id).textContent;
      navigator.clipboard.writeText(text);
    }

    function copyRT() {
      const rt = document.getElementById('result-rt').textContent;
      navigator.clipboard.writeText(rt);
    }
  </script>
</body>
</html>`;

// ============================================================
// HTTP 服务器
// ============================================================

async function handleRequest(req: Request): Promise<Response> {
  const url = new URL(req.url);

  // 静态页面
  if (url.pathname === "/" || url.pathname === "/index.html") {
    return new Response(HTML, {
      headers: { "Content-Type": "text/html; charset=utf-8" },
    });
  }

  // API: 生成登录 URL
  if (url.pathname === "/api/initiate") {
    try {
      const provider = url.searchParams.get("provider") || "Google";
      const state = crypto.randomUUID();
      const codeVerifier = generateCodeVerifier();
      const codeChallenge = await generateCodeChallenge(codeVerifier);

      const result = await initiateLogin(provider, codeChallenge, state);

      if (!result.redirectUrl) {
        throw new Error("No redirectUrl in response");
      }

      pendingAuth.set(state, { codeVerifier, idp: provider });

      return Response.json({
        authorizeUrl: result.redirectUrl,
        state,
      });
    } catch (e) {
      return Response.json({ error: (e as Error).message }, { status: 400 });
    }
  }

  // API: 交换 Token
  if (url.pathname === "/api/exchange" && req.method === "POST") {
    try {
      const body = await req.json();
      const { callbackUrl, state } = body;

      const pending = pendingAuth.get(state);
      if (!pending) {
        throw new Error("No pending authentication state found");
      }

      const cbUrl = new URL(callbackUrl);
      const code = cbUrl.searchParams.get("code");
      const returnedState = cbUrl.searchParams.get("state");

      if (!code) throw new Error("No 'code' in callback URL");
      if (!returnedState) throw new Error("No 'state' in callback URL");

      const result = await exchangeToken(
        pending.idp,
        code,
        pending.codeVerifier,
        returnedState
      );

      pendingAuth.delete(state);

      // 只返回 refreshToken
      return Response.json({
        refreshToken: result.refreshToken,
      });
    } catch (e) {
      return Response.json({ error: (e as Error).message }, { status: 400 });
    }
  }

  // API: 刷新 Token (桌面端 API)
  if (url.pathname === "/api/refresh" && req.method === "POST") {
    try {
      const body = await req.json();
      const { refreshToken } = body;

      if (!refreshToken) {
        throw new Error("refreshToken is required");
      }

      const result = await refreshTokenDesktop(refreshToken);

      return Response.json({
        accessToken: result.accessToken,
        refreshToken: result.refreshToken,
        expiresIn: result.expiresIn,
        profileArn: result.profileArn,
      });
    } catch (e) {
      return Response.json({ error: (e as Error).message }, { status: 400 });
    }
  }

  return new Response("Not Found", { status: 404 });
}

// ============================================================
// 启动服务器
// ============================================================

console.log(`\n🚀 Kiro OAuth Server running at http://localhost:${PORT}\n`);
Deno.serve({ port: PORT }, handleRequest);

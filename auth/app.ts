/**
 * Amazon Q 授权服务 - Deno 单文件应用
 * 使用 Express + Tailwind CSS 实现授权链接生成和 Token 获取
 */

import express from "npm:express@4.18.2"
import axios from "npm:axios@1.6.2"
import { createHash, randomBytes } from "node:crypto"

const app = express()
const PORT = 3000

/**
 * Base64 URL 安全编码
 */
function base64urlEncode(data: Uint8Array): string {
  return btoa(String.fromCharCode(...data))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '')
}

/**
 * 生成随机字符串
 */
function generateRandomString(length: number): string {
  return base64urlEncode(randomBytes(length))
}

/**
 * 生成 code_challenge
 */
function generateCodeChallenge(verifier: string): string {
  const hash = createHash('sha256')
  hash.update(verifier)
  return base64urlEncode(hash.digest())
}

/**
 * 生成随机回调端口
 */
function getRandomPort(): number {
  return Math.floor(Math.random() * (65000 - 10000 + 1)) + 10000
}

// 存储配置信息
let authConfig: any = null

app.use(express.json())
app.use(express.urlencoded({ extended: true }))

/**
 * 主页路由 - 返回 HTML 界面
 */
app.get("/", (_req, res) => {
  res.send(`
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Amazon Q 授权服务</title>
  <script src="https://cdn.tailwindcss.com"></script>
  <script>
    tailwind.config = {
      darkMode: 'class'
    }
  </script>
  <style>
    @import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'SF Pro Display', 'Segoe UI', sans-serif;
    }
    .glass {
      backdrop-filter: blur(40px) saturate(180%);
      -webkit-backdrop-filter: blur(40px) saturate(180%);
    }
    .glass-strong {
      backdrop-filter: blur(60px) saturate(200%);
      -webkit-backdrop-filter: blur(60px) saturate(200%);
    }
    .bg-pattern {
      background-image:
        radial-gradient(circle at 20% 30%, rgba(59, 130, 246, 0.1) 0%, transparent 50%),
        radial-gradient(circle at 80% 70%, rgba(147, 51, 234, 0.08) 0%, transparent 50%),
        radial-gradient(circle at 40% 80%, rgba(59, 130, 246, 0.06) 0%, transparent 40%);
    }
    .dark .bg-pattern {
      background-image:
        radial-gradient(circle at 20% 30%, rgba(59, 130, 246, 0.15) 0%, transparent 50%),
        radial-gradient(circle at 80% 70%, rgba(147, 51, 234, 0.12) 0%, transparent 50%),
        radial-gradient(circle at 40% 80%, rgba(59, 130, 246, 0.1) 0%, transparent 40%);
    }
  </style>
</head>
<body class="bg-gray-100 dark:bg-gray-900 min-h-screen py-12 px-4 transition-colors duration-300 bg-pattern relative overflow-x-hidden">
  <!-- 夜间模式切换按钮 -->
  <div class="fixed bottom-6 right-6 z-50">
    <button id="themeToggle" class="glass bg-white/80 dark:bg-gray-800/80 p-2.5 rounded-full shadow-lg hover:shadow-xl transition-all duration-200 border border-gray-200/50 dark:border-gray-700/50">
      <svg id="sunIcon" class="w-5 h-5 text-gray-700 dark:text-gray-300 hidden dark:block" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"></path>
      </svg>
      <svg id="moonIcon" class="w-5 h-5 text-gray-700 dark:text-gray-300 block dark:hidden" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"></path>
      </svg>
    </button>
  </div>

  <div class="max-w-4xl mx-auto">
    <div class="glass-strong bg-white/70 dark:bg-gray-800/70 rounded-2xl shadow-2xl p-10 mb-8 border border-white/20 dark:border-gray-700/30 transition-colors duration-300">
      <h1 class="text-3xl font-semibold text-gray-900 dark:text-white mb-3 text-center">Amazon Q 授权服务</h1>
      <div class="flex items-center justify-center mb-10">
        <a href="https://github.com/MamoCode/AmazonQ" target="_blank" class="flex items-center gap-2 text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 transition-colors duration-200">
          <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
            <path fill-rule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" clip-rule="evenodd"/>
          </svg>
          <span class="text-sm">github.com/MamoCode/AmazonQ</span>
        </a>
      </div>

      <!-- 步骤 1: 生成授权链接 -->
      <div id="step1" class="mb-10">
        <h2 class="text-lg font-medium text-gray-900 dark:text-gray-100 mb-4">1. 生成授权链接</h2>
        <button id="generateBtn" class="w-full bg-blue-500 hover:bg-blue-600 dark:bg-blue-600 dark:hover:bg-blue-700 text-white font-medium py-3 px-6 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-[0.98]">
          生成授权链接
        </button>
        <div id="authUrlResult" class="mt-4 hidden">
          <div class="glass bg-gray-50/80 dark:bg-gray-700/50 border border-gray-200/50 dark:border-gray-600/50 rounded-lg p-4">
            <p class="text-xs text-gray-500 dark:text-gray-400 mb-2 uppercase tracking-wide font-medium">授权链接</p>
            <div class="glass bg-white/80 dark:bg-gray-800/50 p-3 rounded border border-gray-200/30 dark:border-gray-600/30 break-all text-sm mb-3">
              <a id="authUrl" href="" target="_blank" class="text-blue-600 dark:text-blue-400 hover:underline"></a>
            </div>
            <button id="copyUrlBtn" class="w-full bg-gray-800 hover:bg-gray-900 dark:bg-gray-700 dark:hover:bg-gray-600 text-white text-sm font-medium py-2.5 px-5 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-[0.98]">
              复制链接
            </button>
          </div>
        </div>
      </div>

      <!-- 步骤 2: 提取 Token (初始隐藏) -->
      <div id="step2" class="hidden">
        <h2 class="text-lg font-medium text-gray-900 dark:text-gray-100 mb-4">2. 提取并交换 Token</h2>
        <div class="glass bg-gray-50/80 dark:bg-gray-700/50 border border-gray-200/50 dark:border-gray-600/50 rounded-lg p-4 mb-4">
          <p class="text-sm text-gray-600 dark:text-gray-300 leading-relaxed">
            <span class="font-medium text-gray-900 dark:text-white">操作步骤：</span><br>
            • 点击上方生成的授权链接<br>
            • 使用 Google / AWS 账号登录并授权<br>
            • 授权后会跳转到无法访问的页面（正常现象）<br>
            • 复制浏览器地址栏中的完整 URL 并粘贴到下方
          </p>
        </div>
        <textarea
          id="callbackUrl"
          placeholder="粘贴授权后跳转的完整 URL..."
          class="w-full h-24 p-3.5 glass bg-white/80 dark:bg-gray-700/50 text-gray-900 dark:text-gray-100 border border-gray-200/50 dark:border-gray-600/50 rounded-lg focus:ring-2 focus:ring-blue-500/50 dark:focus:ring-blue-500/50 focus:border-transparent mb-4 transition-all duration-200 placeholder-gray-400 dark:placeholder-gray-500 text-sm"
        ></textarea>
        <button id="extractBtn" class="w-full bg-blue-500 hover:bg-blue-600 dark:bg-blue-600 dark:hover:bg-blue-700 text-white font-medium py-3 px-6 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-[0.98]">
          提取 Token
        </button>
        <div id="tokenResult" class="mt-4 hidden">
          <div class="glass bg-gray-50/80 dark:bg-gray-700/50 border border-gray-200/50 dark:border-gray-600/50 rounded-lg p-4">
            <p class="text-xs text-gray-500 dark:text-gray-400 mb-3 uppercase tracking-wide font-medium">凭证信息</p>
            <div class="space-y-2.5">
              <div class="glass bg-white/80 dark:bg-gray-800/50 p-3 rounded border border-gray-200/30 dark:border-gray-600/30">
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5">Client ID</p>
                <p id="clientId" class="text-xs font-mono text-gray-900 dark:text-gray-100 whitespace-nowrap overflow-x-auto"></p>
              </div>
              <div class="glass bg-white/80 dark:bg-gray-800/50 p-3 rounded border border-gray-200/30 dark:border-gray-600/30">
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5">Client Secret</p>
                <p id="clientSecret" class="text-xs font-mono text-gray-900 dark:text-gray-100 whitespace-nowrap overflow-x-auto"></p>
              </div>
              <div class="glass bg-white/80 dark:bg-gray-800/50 p-3 rounded border border-gray-200/30 dark:border-gray-600/30">
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5">Refresh Token</p>
                <p id="refreshToken" class="text-xs font-mono text-gray-900 dark:text-gray-100 whitespace-nowrap overflow-x-auto"></p>
              </div>
              <div class="glass bg-white/80 dark:bg-gray-800/50 p-3 rounded border border-gray-200/30 dark:border-gray-600/30">
                <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5">完整凭证</p>
                <p id="fullCredentials" class="text-xs font-mono text-gray-900 dark:text-gray-100 whitespace-nowrap overflow-x-auto"></p>
              </div>
            </div>
            <button id="copyCredsBtn" class="w-full mt-3 bg-gray-800 hover:bg-gray-900 dark:bg-gray-700 dark:hover:bg-gray-600 text-white text-sm font-medium py-2.5 px-5 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-[0.98]">
              复制完整凭证
            </button>
            <p class="text-xs text-gray-500 dark:text-gray-400 text-center mt-2">完整凭证为本项目的身份校验KEY</p>
          </div>
        </div>
      </div>
    </div>

    <!-- 错误提示 -->
    <div id="errorMsg" class="hidden glass bg-white/70 dark:bg-gray-800/70 border border-gray-300/50 dark:border-gray-600/50 rounded-lg p-4 mb-4">
      <p class="text-gray-800 dark:text-gray-200"></p>
    </div>

    <!-- 加载提示 -->
    <div id="loading" class="hidden fixed inset-0 bg-black/10 dark:bg-black/40 backdrop-blur flex items-center justify-center z-50">
      <div class="glass-strong bg-white/90 dark:bg-gray-800/90 rounded-xl p-6 flex items-center space-x-3 border border-gray-200/30 dark:border-gray-700/30 shadow-2xl">
        <div class="animate-spin rounded-full h-7 w-7 border-2 border-gray-300 dark:border-gray-600 border-t-blue-500 dark:border-t-blue-500"></div>
        <p class="text-gray-900 dark:text-gray-100 font-medium">处理中...</p>
      </div>
    </div>
  </div>

  <script>
    // 夜间模式切换
    const themeToggle = document.getElementById('themeToggle')
    const htmlElement = document.documentElement

    // 检查本地存储中的主题设置
    const currentTheme = localStorage.getItem('theme') ||
                        (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')

    if (currentTheme === 'dark') {
      htmlElement.classList.add('dark')
    }

    themeToggle.addEventListener('click', () => {
      htmlElement.classList.toggle('dark')
      const theme = htmlElement.classList.contains('dark') ? 'dark' : 'light'
      localStorage.setItem('theme', theme)
    })
    const showError = (msg) => {
      const errorDiv = document.getElementById('errorMsg')
      errorDiv.querySelector('p').textContent = msg
      errorDiv.classList.remove('hidden')
      setTimeout(() => errorDiv.classList.add('hidden'), 5000)
    }

    const showLoading = (show) => {
      document.getElementById('loading').classList.toggle('hidden', !show)
    }

    // 生成授权链接
    document.getElementById('generateBtn').addEventListener('click', async () => {
      showLoading(true)
      try {
        const response = await fetch('/api/generate-auth', { method: 'POST' })
        const data = await response.json()

        if (!response.ok) {
          throw new Error(data.error || '生成失败')
        }

        document.getElementById('authUrl').href = data.authUrl
        document.getElementById('authUrl').textContent = data.authUrl
        document.getElementById('authUrlResult').classList.remove('hidden')

        // 显示步骤2
        setTimeout(() => {
          document.getElementById('step2').classList.remove('hidden')
        }, 300)
      } catch (error) {
        showError('生成授权链接失败: ' + error.message)
      } finally {
        showLoading(false)
      }
    })

    // 复制按钮通用函数
    const copyWithFeedback = (btn, text) => {
      navigator.clipboard.writeText(text).then(() => {
        const originalText = btn.textContent
        btn.textContent = '已复制 ✓'
        btn.classList.add('bg-green-600')
        btn.classList.remove('bg-gray-800', 'hover:bg-gray-900')
        setTimeout(() => {
          btn.textContent = originalText
          btn.classList.remove('bg-green-600')
          btn.classList.add('bg-gray-800', 'hover:bg-gray-900')
        }, 1500)
      })
    }

    // 复制授权链接
    document.getElementById('copyUrlBtn').addEventListener('click', function() {
      const url = document.getElementById('authUrl').textContent
      copyWithFeedback(this, url)
    })

    // 提取并交换 Token
    document.getElementById('extractBtn').addEventListener('click', async () => {
      const callbackUrl = document.getElementById('callbackUrl').value.trim()

      if (!callbackUrl) {
        showError('请输入回调 URL')
        return
      }

      showLoading(true)
      try {
        const response = await fetch('/api/exchange-token', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ callbackUrl })
        })
        const data = await response.json()

        if (!response.ok) {
          throw new Error(data.error || '交换失败')
        }

        document.getElementById('clientId').textContent = data.clientId
        document.getElementById('clientSecret').textContent = data.clientSecret || '(无)'
        document.getElementById('refreshToken').textContent = data.refreshToken
        document.getElementById('fullCredentials').textContent = data.credentials
        document.getElementById('tokenResult').classList.remove('hidden')
      } catch (error) {
        showError('Token 交换失败: ' + error.message)
      } finally {
        showLoading(false)
      }
    })

    // 复制完整凭证
    document.getElementById('copyCredsBtn').addEventListener('click', function() {
      const creds = document.getElementById('fullCredentials').textContent
      copyWithFeedback(this, creds)
    })
  </script>
</body>
</html>
  `)
})

/**
 * API: 生成授权链接
 */
app.post("/api/generate-auth", async (_req, res) => {
  try {
    const REGION = "us-east-1"
    const CLIENT_NAME = "AWS IDE Extensions for VSCode"
    const START_URL = "https://view.awsapps.com/start"
    const SCOPES = "codewhisperer:completions,codewhisperer:analysis,codewhisperer:conversations,codewhisperer:transformations,codewhisperer:taskassist"

    const callbackPort = getRandomPort()
    const redirectUri = `http://127.0.0.1:${callbackPort}/oauth/callback`

    const response = await axios.post(
      `https://oidc.${REGION}.amazonaws.com/client/register`,
      {
        clientName: CLIENT_NAME,
        clientType: "public",
        grantTypes: ["authorization_code", "refresh_token"],
        redirectUris: [redirectUri],
        scopes: SCOPES.split(','),
        issuerUrl: START_URL
      },
      {
        headers: { 'Content-Type': 'application/json' }
      }
    )

    const registration = response.data
    const clientId = registration.clientId
    const clientSecret = registration.clientSecret || ''

    const codeVerifier = generateRandomString(64)
    const state = crypto.randomUUID()
    const codeChallenge = generateCodeChallenge(codeVerifier)

    const params = new URLSearchParams({
      response_type: 'code',
      client_id: clientId,
      redirect_uri: redirectUri,
      scopes: SCOPES,
      state: state,
      code_challenge: codeChallenge,
      code_challenge_method: 'S256'
    })

    const authUrl = `https://oidc.${REGION}.amazonaws.com/authorize?${params.toString()}`

    authConfig = {
      clientId,
      clientSecret,
      redirectUri,
      codeVerifier,
      state,
      region: REGION
    }

    res.json({ authUrl })
  } catch (error: any) {
    console.error('生成授权链接失败:', error)
    res.status(500).json({ error: error.message || '生成授权链接失败' })
  }
})

/**
 * API: 交换 Token
 */
app.post("/api/exchange-token", async (req, res) => {
  try {
    if (!authConfig) {
      return res.status(400).json({ error: '请先生成授权链接' })
    }

    const { callbackUrl } = req.body

    if (!callbackUrl) {
      return res.status(400).json({ error: '缺少回调 URL' })
    }

    const url = new URL(callbackUrl)
    const code = url.searchParams.get('code')
    const returnedState = url.searchParams.get('state')

    if (!code) {
      return res.status(400).json({ error: '回调 URL 中未找到授权码' })
    }

    if (returnedState !== authConfig.state) {
      console.warn('State 验证失败，但继续尝试交换令牌...')
    }

    const payload: any = {
      grantType: "authorization_code",
      code: code,
      redirectUri: authConfig.redirectUri,
      clientId: authConfig.clientId,
      codeVerifier: authConfig.codeVerifier
    }

    if (authConfig.clientSecret) {
      payload.clientSecret = authConfig.clientSecret
    }

    const response = await axios.post(
      `https://oidc.${authConfig.region}.amazonaws.com/token`,
      payload,
      {
        headers: { 'Content-Type': 'application/json' }
      }
    )

    const tokens = response.data
    const refreshToken = tokens.refreshToken || tokens.refresh_token

    if (!refreshToken) {
      return res.status(500).json({ error: '未获取到 refresh_token' })
    }

    const credentials = `${authConfig.clientId}:${authConfig.clientSecret}:${refreshToken}`

    res.json({
      clientId: authConfig.clientId,
      clientSecret: authConfig.clientSecret,
      refreshToken: refreshToken,
      credentials: credentials
    })
  } catch (error: any) {
    console.error('交换 Token 失败:', error)
    res.status(500).json({ error: error.response?.data?.message || error.message || '交换 Token 失败' })
  }
})

/**
 * 启动服务器
 */
app.listen(PORT, () => {
  console.log(`\n服务已启动: http://localhost:${PORT}`)
  console.log(`\n请在浏览器中打开上述地址开始使用`)
})

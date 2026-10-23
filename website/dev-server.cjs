// 官网本地预览服务器：只服务 dist/ 静态产物，模拟线上站点根（https://xapptool.cn/）的托管行为。
// =============================================================
// 单一源约束（website 端口/base）
// 单一代码源：vite.config.js 的 SITE_BASE（'/'）与 server.port（8213）
// 本文件必须与 vite.config.js 字面一致：
//   - PORT = 8213        同步 vite.config.js server.port
//   - SITE_BASE = '/'    同步 vite.config.js base（线上站点根路径托管）
// 跨包对齐：platform-server 端口与本文件无关——纯静态站不代理任何后端。
// =============================================================
const http = require('http')
const fs = require('fs')
const path = require('path')

const PORT = 8213
const SITE_BASE = '/'
const DIST_DIR = path.join(__dirname, 'dist')

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.txt': 'text/plain; charset=utf-8',
  '.webmanifest': 'application/manifest+json',
}

function send(res, status, body, headers = {}) {
  res.writeHead(status, { 'Cache-Control': 'no-store', ...headers })
  res.end(body)
}

function readFileOr(res, filePath, status, onError) {
  fs.readFile(filePath, (err, data) => {
    if (err) return onError()
    const ext = path.extname(filePath).toLowerCase()
    send(res, status, data, { 'Content-Type': MIME[ext] || 'application/octet-stream' })
  })
}

function serveStatic(req, res) {
  const urlPath = decodeURIComponent(req.url.split('?')[0])

  if (!urlPath.startsWith(SITE_BASE)) {
    return send(res, 404, 'Not Found')
  }

  const relPath = urlPath.slice(SITE_BASE.length)
  const base = path.join(DIST_DIR, relPath)
  // 防止路径穿越：必须是 dist 或其内部路径（等值比较挡掉 dist-evil 这类同前缀兄弟目录）
  if (base !== DIST_DIR && !base.startsWith(DIST_DIR + path.sep)) {
    return send(res, 403, 'Forbidden')
  }
  const notFound = () => {
    // 未命中的路径回退到 404.html：它由 postbuild 从 index.html 复制而来，
    // 加载后由 vue-router 按真实 pathname 决定渲染页面还是 NotFoundPage。
    // 状态码保持 404，与线上 nginx 的 error_page 404 /404.html 语义一致。
    const notFoundPath = path.join(DIST_DIR, '404.html')
    readFileOr(res, notFoundPath, 404, () =>
      send(res, 404, 'Not Found: 缺少 dist/404.html，请先 npm run build（postbuild 会生成它）')
    )
  }
  fs.stat(base, (err, stat) => {
    if (!err && stat.isDirectory()) {
      // 目录（含站点根 relPath=''）→ index.html，与 nginx 目录索引行为一致
      return readFileOr(res, path.join(base, 'index.html'), 200, notFound)
    }
    if (!err && stat.isFile()) {
      return readFileOr(res, base, 200, notFound)
    }
    notFound()
  })
}

const server = http.createServer(serveStatic)

server.listen(PORT, '127.0.0.1', () => {
  console.log(`Website preview server: http://127.0.0.1:${PORT}${SITE_BASE}`)
  console.log(`Serving ${DIST_DIR} (静态站，不代理任何后端)`)
})

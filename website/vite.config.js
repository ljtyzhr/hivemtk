import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'


// =============================================================
// 单一源约束（website base / dev 端口）
// 单一代码源：本文件的 SITE_BASE 与 server.port。
//   - dev-server.cjs 的 SITE_BASE / PORT 必须与本文件字面一致
//   - README 的发布步骤跟着 SITE_BASE 走
// 发布面只有一个：本地构建产物 dist/ 整目录上传到线上服务器站点根
// /www/wwwroot/hivemtk/website（对应 https://xapptool.cn/ 根路径），
// 因此 base 必须是根路径 '/'，产物里的资源引用为 /assets/… 绝对路径。
// 纯静态站：运行期对任何后端零依赖（无 API 调用、无客服浮标、无登录态）。
// =============================================================
const SITE_BASE = '/'

export default defineConfig({
  base: SITE_BASE,
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  // 纯静态站：不代理任何后端。历史版本在此把 /public 与 /merchant-api 反代到 platform-server:8205
  // 供右下角客服浮标拉联系信息；浮标随线上域名下线一起删除后，官网运行期零后端依赖。
  // dev 端口 8213 与 dev-server.cjs 的 PORT 是一对（仅本机预览用，不与任何后端端口对齐）。
  server: {
    port: 8213,
    host: true,
  },
  build: {
    sourcemap: false,
    chunkSizeWarningLimit: 1000,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (
              id.includes('vue') ||
              id.includes('vue-router') ||
              id.includes('vue-i18n')
            ) {
              return 'vendor'
            }
          }
        },
      },
    },
  },
})


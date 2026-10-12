import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.js'

// Vitest 配置：单测 + 组件测试
// 复用 vite.config.js 的 alias 与插件配置
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      globals: true,
      environment: 'jsdom',
      include: ['tests/unit/**/*.{test,spec}.{js,mjs,ts}'],
      exclude: ['node_modules', 'dist', 'tests/e2e/**'],
      // 串行跑测试文件，不并行。
      //
      // 为什么：本仓的组件用例普遍用「刷新 N 次」等渲染稳定（`await flushPromises()`
      // 只排空微任务，对 setTimeout 驱动的防抖/轮询一个都不等）。N 在空闲时碰巧够用，
      // 一旦几个 worker 抢 CPU 就不够 —— 实测同一棵树连跑三次，失败集合分别是
      // 0 / 30 / 11 个，横跨十几个互不相干的页面，全是这个原因。
      //
      // 这类失败比真 bug 更坏：它让人怀疑刚改的代码，久了大家就不信这套测试了。
      // 56 个文件串行约一分多钟，换确定性值这个价。真要恢复并行，先把
      // tests/unit/settle-helper.js 那个 settle() 铺进所有用例，让断言不再依赖
      // 「刷几次够」，而不是把 fileParallelism 调回 true。
      fileParallelism: false,
      coverage: {
        provider: 'v8',
        reporter: ['text', 'html', 'json-summary'],
        reportsDirectory: './coverage',
        include: [
          'src/utils/**',
          'src/api/backup.js',
          'src/api/order.js',
          'src/api/securityAudit.js',
          'src/views/backup/**',
          'src/views/order/**',
          'src/views/securityAudit/**'
        ],
        exclude: ['**/*.test.*', '**/*.spec.*', '**/__tests__/**']
      }
    }
  })
)

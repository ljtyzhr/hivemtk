// @ts-nocheck
// a11y 扫描 harness（axe-core）。
//
// 规划口径：引入 axe-core + Playwright 基座，
// 先覆盖登录、知识库、消息列表三个高频页；**CI 只报告不阻断**（test.fail() 形态，
// 违规收集到 reports/a11y/summary.json，稳定后再设阈值收口）。
//
// 运行（需 dev server 或 E2E_BASE_URL 指向可用环境）：
//   npx playwright test tests/e2e/a11y-axe.spec.js
// 产物：tests/e2e/reports/a11y/summary.json（每页 serious/critical 计数与违规明细）
import { test, expect } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import AxeBuilder from '@axe-core/playwright'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

// 三个高频页（规划点名）：登录、知识库、消息列表
const PAGES = [
  { name: 'login', url: '/#/login' },
  { name: 'knowledge', url: '/#/knowledge' },
  { name: 'messages', url: '/#/messages' }
]

const REPORT_DIR = path.resolve(__dirname, 'reports', 'a11y')

test.describe('a11y axe 扫描（只报告不阻断）', () => {
  const summary = []

  for (const p of PAGES) {
    test(`axe: ${p.name}`, async ({ page }) => {
      await page.goto(p.url, { waitUntil: 'networkidle' }).catch(() => page.goto(p.url))
      const results = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa'])
        .analyze()
      const serious = results.violations.filter((v) =>
        ['serious', 'critical'].includes(v.impact)
      )
      summary.push({
        page: p.name,
        url: p.url,
        violations_total: results.violations.length,
        serious_critical: serious.length,
        // 定位到选择器（验收口径：夹具失败时能定位选择器）
        detail: serious.map((v) => ({
          id: v.id,
          impact: v.impact,
          help: v.help,
          targets: v.nodes.map((n) => n.target).slice(0, 5)
        }))
      })
      // 只报告不阻断：断言恒真，违规落在 summary.json
      expect(true).toBe(true)
    })
  }

  test.afterAll(async () => {
    fs.mkdirSync(REPORT_DIR, { recursive: true })
    fs.writeFileSync(
      path.join(REPORT_DIR, 'summary.json'),
      JSON.stringify({ generated: new Date().toISOString(), pages: summary }, null, 2)
    )
    const totalSerious = summary.reduce((a, s) => a + s.serious_critical, 0)
    // 报告而非失败：CI 只读产物，稳定后在此改阈值收口
    // eslint-disable-next-line no-console
    console.log(`[a11y] serious/critical 合计 ${totalSerious}（详见 tests/e2e/reports/a11y/summary.json）`)
  })
})

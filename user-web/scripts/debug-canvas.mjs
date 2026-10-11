import { chromium } from 'playwright';
const BASE = 'http://localhost:8211';
const b = await chromium.launch({ headless: true });
const ctx = await b.newContext({ viewport: { width: 1280, height: 800 } });
const page = await ctx.newPage();
await page.goto(BASE + '/#/login', { waitUntil: 'networkidle', timeout: 15000 });
await page.waitForTimeout(2000);
const u = await page.$('input[type="text"], input[placeholder*="用户名"], input[placeholder*="邮箱"]');
const p = await page.$('input[type="password"]');
await u.fill('admin'); await p.fill('Seed@123456');
const btn = await page.$('button[type="submit"], .el-button--primary');
if (btn) await btn.click();
await page.waitForTimeout(5000);
await page.goto(BASE + '/#/reachPipeline/editor', { waitUntil: 'networkidle', timeout: 15000 });
await page.waitForTimeout(3000);
const info = await page.evaluate(() => {
  const vf = document.querySelector('.vue-flow');
  const viewport = document.querySelector('.vue-flow__viewport');
  const edges = document.querySelectorAll('.vue-flow__edge').length;
  const nodes = document.querySelectorAll('.vue-flow__node').length;
  const panel = document.querySelector('.flow-node-panel');
  return {
    hasVueFlow: !!vf,
    hasViewport: !!viewport,
    edges, nodes,
    hasPanel: !!panel,
    panelHTML: panel ? panel.outerHTML.substring(0, 300) : null,
    vfHTML: vf ? vf.outerHTML.substring(0, 400) : null,
  };
});
console.log(JSON.stringify(info, null, 2));
await b.close();

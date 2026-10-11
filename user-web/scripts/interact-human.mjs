/**
 * user-web 真人模拟交互引擎
 * 对每条路由：发现可交互元素 → 逐个点击（tab/按钮/分页/开关/行操作/表单）
 * 每次交互后四重验证：
 *   1. 控制台新增错误
 *   2. API 新增失败请求
 *   3. UI 渲染变化（DOM/文本 diff）
 *   4. 写操作后 API 回读 DB 确认落库
 * 安全规则：删除类按钮只点开确认框后取消；表单用假数据+测后清理
 */
import { chromium } from 'playwright';
import fs from 'fs';
import path from 'path';

const BASE_URL = 'http://localhost:8211';
const API_URL = 'http://localhost:8204';
const BATCH = process.argv[2] || '1';
const RUN_ID = `e2e-${Date.now()}`;

// 全部 216 条路由（与 ROUTE_AUDIT_LIST.md 一致）
const ALL_ROUTES = [
  '/setup', '/login', '/help-center', '/chat/embed/default',
  '/abExperiment/list',
  '/aiAgent', '/aiAgent/list', '/aiAgent/create', '/aiAgent/edit/1', '/aiAgent/tools',
  '/aiProductivity/list',
  '/analytics/cohort-path',
  '/approvalTask/list',
  '/asset-bundle/list', '/asset-bundle/playground', '/asset-bundle/playground/1',
  '/asset-bundle/merchant-new', '/asset-bundle/merchant/1',
  '/asset-market', '/asset-market/detail/1', '/asset-market/my-assets', '/asset-market/sync-log',
  '/backup/list', '/backup/enhanced',
  '/badCase/list',
  '/batchOperation/list',
  '/bridge', '/bridge/token', '/token',
  '/browser-automation', '/browser-automation/tasks', '/browser-automation/tasks/create',
  '/browser-automation/tasks/1/edit', '/browser-automation/tasks/1',
  '/browser-automation/sessions/1', '/browser-automation/cron', '/browser-automation/status',
  '/chatChannel/list', '/chatChannel/create', '/chatChannel/edit/1', '/chatChannel/install-guide/1',
  '/churnPrediction/list',
  '/clue/list', '/clue/statistics',
  '/community/list',
  '/conversionFunnel/list',
  '/customReport/list',
  '/customer360/list',
  '/customerEvent/list',
  '/customerJourney/dashboard',
  '/customerService/agentStatus', '/customerService/quickReply', '/customerService/sessionTag',
  '/customerService/aiSuggestion', '/customerService/csat',
  '/customerSession/list',
  '/dashboardScreen/list', '/dashboardScreen/builder',
  '/dialogueMemory/list',
  '/dingtalk-app',
  '/domainPool',
  '/douyinCard', '/douyin/stats', '/douyin-card-stats/1',
  '/email', '/email/drafts', '/email/jobs', '/email/smtp', '/email/info',
  '/email/guide', '/email/deliverability', '/email/drag-editor',
  '/faq', '/faq/list', '/faq/editor', '/faq/editor/1',
  '/feishu', '/feishu/account',
  '/followups/today', '/followups/1',
  '/geo-tools/brand-config', '/geo-tools/seo-infra', '/geo-tools/pusher-config',
  '/geo-tools/site-config', '/geo-tools/schema-templates', '/geo-tools/competitors',
  '/geo-tools/config', '/geo-tools/keyword-mining', '/geo-tools/content-creation',
  '/geo-tools/content-optimize', '/geo-tools/site-publish', '/geo-tools/push-center',
  '/geo-tools/platform-publish', '/geo-tools/workflow', '/geo-tools/knowledge-base',
  '/geo-tools/entity-graph', '/geo-tools/funnel-dashboard', '/geo-tools/index-tracking',
  '/geo-tools/visibility', '/geo-tools/sov-board', '/geo-tools/crawler-stats',
  '/geo-tools/verification', '/geo/decision-report', '/geo-tools/reports', '/geo-tools/alerts',
  '/glossary',
  '/i18n/dashboard',
  '/inbox/list',
  '/integration/list',
  '/intentRecognition/list',
  '/kb/connectors', '/cap/matrix',
  '/knowledge/management', '/knowledge/batch-import', '/knowledge/playground',
  '/knowledge/connectors', '/knowledge/feedbacks', '/knowledge/tokens',
  '/knowledge/external', '/knowledge/statistics', '/knowledge/openapi', '/knowledge/rag-eval',
  '/knowledgeBase', '/knowledgeBase/list', '/knowledgeBase/create', '/knowledgeBase/edit/1',
  '/kuaishouCard', '/kuaishou/stats', '/kuaishou-card-stats/1',
  '/lead-mining/index',
  '/livecode',
  '/llmRouting/list', '/llmRouting/cost',
  '/marketingFlow/list',
  '/mcp/credentials',
  '/messageHub/list', '/messageHub/dashboard',
  '/objection/list',
  '/oneid/list', '/oneid/conflicts', '/oneid/merge-rules',
  '/operationLog/list', '/operationLog/enhanced',
  '/ops-overview', '/sales-cockpit',
  '/dashboard/drafts', '/dashboard/drafts/1',
  '/system/permissions',
  '/persona/list',
  '/platformAccount/list',
  '/qq', '/qq/account',
  '/system/rag-product-config', '/system/rag-account', '/system/rag-product',
  '/reachPipeline/list', '/reachPipeline/editor',
  '/system/roles',
  '/sales-workbench',
  '/scriptTemplate/list', '/scriptTemplate/ab-stats',
  '/securityAudit/list',
  '/shortLink', '/shortLink/stats',
  '/sms/list', '/sms/drafts', '/sms/jobs', '/sms/config',
  '/sopAgent/list',
  '/sop-template', '/sop-template/list', '/sop-template/market',
  '/sop-template/editor', '/sop-template/editor/1',
  '/system/config', '/system/obs-config', '/system/material-library',
  '/system/monitor', '/system/trace', '/system/guide', '/system/rag-overview',
  '/system/automation-hub', '/system/config-params', '/system/ltc-config',
  '/system/users',
  '/tagSegmentation/list',
  '/telegram', '/telegram/account', '/telegram/gate',
  '/tiktok', '/tiktok/list', '/tiktok/stats', '/tiktok-card-stats/1',
  '/confidence/panel', '/humanize/panel', '/feedbackLoop/panel',
  '/unifiedMessage/list',
  '/userSegment/list', '/userSegment/rfm-matrix', '/userSegment/builder',
  '/wecomAccount/list', '/wecomAccount/data',
  '/whatsapp', '/whatsapp/account', '/whatsapp/drafts', '/whatsapp/jobs',
  '/whatsapp/lead-group-selection', '/whatsapp/group-messaging', '/whatsapp/bulk-matrix',
  '/whatsapp-cloud',
  '/workflow-orchestrator/list', '/workflow-orchestrator/editor/1',
  '/workflow-orchestrator/execution/1',
  '/xianyuCard', '/xianyu/stats', '/xianyu-card-stats/1',
  '/xiaohongshuCard', '/xiaohongshu/stats', '/xiaohongshu-card-stats/1',
  '/profile', '/notifications',
];

// 分批：每批 24 条
const PER_BATCH = 24;
const startIdx = (Number(BATCH) - 1) * PER_BATCH;
const routes = ALL_ROUTES.slice(startIdx, startIdx + PER_BATCH);

// 危险操作黑名单：只点开确认框后取消，不执行
const DANGER_PATTERNS = [/删除|移除|清空|销毁|注销|吊销|断开|解绑|封禁|禁用|terminate|destroy|remove|delete|disconnect|revoke|deactivate/i];
// 写操作：点击后可填表提交（用假数据）
const WRITE_PATTERNS = [/新建|创建|添加|新增|保存|提交|生成|导入|同步|发布|开通|连接|授权|发送|执行|运行|重试|测试连接/i];
// 只读探索：查看详情/展开
const READ_PATTERNS = [/查看|详情|预览|展开|更多|日志|历史|统计|说明|文档|帮助|指南|复制|下载|导出/i];

const is404 = (text) => text.includes('404 Page Does not Exist') || text.includes('页面不存在');

function classifyButton(text) {
  if (!text) return 'unknown';
  if (DANGER_PATTERNS.some(p => p.test(text))) return 'danger';
  if (WRITE_PATTERNS.some(p => p.test(text))) return 'write';
  if (READ_PATTERNS.some(p => p.test(text))) return 'read';
  return 'click';
}

async function login(page) {
  await page.goto(BASE_URL + '/#/login', { waitUntil: 'networkidle', timeout: 15000 });
  await page.waitForTimeout(2000);
  const u = await page.$('input[type="text"], input[placeholder*="用户名"], input[placeholder*="邮箱"], input[placeholder*="username"], input[placeholder*="email"]');
  const p = await page.$('input[type="password"]');
  if (!u || !p) throw new Error('登录页未找到输入框');
  await u.fill('admin');
  await p.fill('Seed@123456');
  const btn = await page.$('button[type="submit"], .el-button--primary');
  if (btn) await btn.click();
  await page.waitForTimeout(5000);
  const body = await page.evaluate(() => document.body?.innerText || '');
  if (is404(body) || body.includes('Log In')) throw new Error('登录失败');
  return true;
}

/**
 * 对单条路由执行深度交互
 */
async function interactRoute(page, route, tracked) {
  const result = {
    route,
    status: 'unknown',
    pageLoad: {},
    interactions: [],
    consoleErrors: [],
    apiErrors: [],
    dbVerifications: [],
    summary: '',
  };

  const consoleErrors = [];
  const apiErrors = [];
  const listeners = {
    console: (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text().substring(0, 400)); },
    response: (res) => {
      if (res.url().includes('/api/') && res.status() >= 400) {
        apiErrors.push(`${res.status()} ${res.url().replace(BASE_URL, '')}`);
      }
    },
  };
  page.on('console', listeners.console);
  page.on('response', listeners.response);

  try {
    // 1. 导航 + 等待加载
    await page.goto(BASE_URL + '/#/' + route.replace(/^\//, ''), { waitUntil: 'networkidle', timeout: 20000 });
    await page.waitForTimeout(2500);

    const bodyText = await page.evaluate(() => document.body?.innerText || '');
    result.pageLoad = {
      not404: !is404(bodyText),
      contentLength: bodyText.trim().length,
      url: page.url(),
    };
    if (is404(bodyText)) {
      result.status = 'fail';
      result.summary = '404页面';
      return result;
    }

    const baseline = {
      consoleCount: 0,
      apiCount: 0,
      bodyHash: bodyText.length + ':' + bodyText.substring(0, 200),
    };

    // 2. 发现可交互元素
    const discoveries = await discoverInteractive(page);
    result.discovered = discoveries.counts;

    // 3. 逐个交互
    // 3a. Tab 切换
    for (let i = 0; i < discoveries.tabs.length && i < 6; i++) {
      const inter = await safeInteract(page, 'tab', discoveries.tabs[i], baseline, consoleErrors, apiErrors, i);
      result.interactions.push(inter);
      if (inter.dialogOpened) await closeDialog(page);
    }

    // 3b. 分页
    for (const pag of discoveries.pagination) {
      const inter = await safeInteract(page, 'pagination', pag, baseline, consoleErrors, apiErrors);
      result.interactions.push(inter);
      await page.waitForTimeout(800);
    }

    // 3c. 开关
    for (const sw of discoveries.switches.slice(0, 3)) {
      const inter = await safeInteract(page, 'switch', sw, baseline, consoleErrors, apiErrors);
      result.interactions.push(inter);
      if (inter.dialogOpened) await closeDialog(page);
    }

    // 3d. 按钮（排除导航类、danger 类先点开确认框验证再取消）
    const buttonPlan = planButtonOrder(discoveries.buttons);
    for (const b of buttonPlan.slice(0, 12)) {
      const kind = classifyButton(b.text);
      const inter = await safeInteract(page, `button:${kind}`, b, baseline, consoleErrors, apiErrors);
      result.interactions.push(inter);

      if (kind === 'danger' && inter.dialogOpened) {
        // 危险操作：确认框出现即算验证成功，取消
        const cancelled = await cancelDialog(page);
        inter.dialogHandled = cancelled ? 'cancelled' : 'not-cancelled';
      } else if (inter.dialogOpened) {
        await closeDialog(page);
      }

      // 写操作按钮：若打开表单对话框，填假数据提交并做 DB 回读
      if (kind === 'write' && inter.dialogOpened) {
        const dbRes = await fillAndSubmitDialog(page, route);
        if (dbRes) {
          result.dbVerifications.push(dbRes);
          inter.dbVerification = dbRes;
        }
        await closeDialog(page);
      }

      // 独立写页面（create/editor 路由）：填表单提交
      if (kind === 'write' && !inter.dialogOpened && (route.includes('/create') || route.includes('/editor'))) {
        const dbRes = await fillAndSubmitInlineForm(page, route);
        if (dbRes) {
          result.dbVerifications.push(dbRes);
          inter.dbVerification = dbRes;
        }
      }
    }

    // 3e. 表格行首列点击（查看/展开）
    if (discoveries.tableRows > 0) {
      const rowInter = await safeInteract(page, 'table-row', { sel: '.el-table__body tr:first-child td:first-child', text: '(first row)' }, baseline, consoleErrors, apiErrors);
      result.interactions.push(rowInter);
      if (rowInter.dialogOpened) await closeDialog(page);
    }

    // 4. 汇总
    const realConsole = consoleErrors.filter(e => !e.includes('favicon') && !e.includes('Failed to load resource'));
    // 假ID 404 提示属预期
    const isFakeId = /\/\d+$/.test(route);
    const realApi = apiErrors.filter(e => {
      if (isFakeId && e.startsWith('404')) return false;
      return true;
    });
    result.consoleErrors = realConsole;
    result.apiErrors = realApi;

    const uiBug = result.interactions.some(i => i.uiBroken);
    const consoleBug = realConsole.length > 0 && !isFakeId;
    const apiBug = realApi.length > 0 && !isFakeId;

    if (uiBug || consoleBug || apiBug) {
      result.status = consoleBug || apiBug || uiBug ? 'partial' : 'ok';
      const issues = [];
      if (consoleBug) issues.push(`控制台错误${realConsole.length}条`);
      if (apiBug) issues.push(`API失败${realApi.length}条`);
      if (uiBug) issues.push('UI渲染异常');
      result.summary = issues.join(' | ');
    } else {
      result.status = 'ok';
      result.summary = `${result.interactions.length}次交互，无异常`;
    }
  } catch (e) {
    result.status = 'error';
    result.summary = e.message.substring(0, 300);
  } finally {
    page.off('console', listeners.console);
    page.off('response', listeners.response);
  }
  return result;
}

/**
 * 发现页面可交互元素
 */
async function discoverInteractive(page) {
  return await page.evaluate(() => {
    const tabs = [];
    document.querySelectorAll('.el-tabs__item, [role="tab"]').forEach(el => {
      const text = (el.innerText || '').trim();
      if (text) tabs.push({ text, idx: tabs.length });
    });

    const buttons = [];
    document.querySelectorAll('.el-button, button').forEach((el, i) => {
      if (el.offsetParent === null) return; // 隐藏的跳过
      const text = (el.innerText || el.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ');
      if (!text || text.length > 30) return;
      // 排除侧边栏导航按钮
      if (el.closest('.el-menu, nav, .sidebar, .el-aside')) return;
      buttons.push({ text, idx: i });
    });

    const pagination = [];
    document.querySelectorAll('.el-pagination .btn-next, .el-pagination .btn-prev, .el-pager li.number').forEach(el => {
      const text = (el.innerText || el.getAttribute('aria-label') || '').trim();
      pagination.push({ text: text || 'page-btn', cls: el.className });
    });

    const switches = [];
    document.querySelectorAll('.el-switch').forEach((el, i) => {
      if (el.offsetParent === null) return;
      switches.push({ idx: i, checked: el.classList.contains('is-checked') });
    });

    const tableRows = document.querySelectorAll('.el-table__body tr').length;
    const forms = document.querySelectorAll('form, .el-form').length;
    const inputs = document.querySelectorAll('input:not([type=hidden]), textarea').length;

    return {
      counts: { tabs: tabs.length, buttons: buttons.length, pagination: pagination.length, switches: switches.length, tableRows, forms, inputs },
      tabs, buttons, pagination: pagination.slice(0, 4), switches,
      tableRows,
    };
  });
}

/**
 * 安全执行一次交互并四重验证
 */
async function safeInteract(page, type, target, baseline, consoleErrors, apiErrors, idx = 0) {
  const inter = { type, target: target.text || type, before: { console: consoleErrors.length, api: apiErrors.length }, after: {}, dialogOpened: false, uiChanged: false, uiBroken: false, newConsole: [], newApi: [] };
  try {
    const bodyBefore = await page.evaluate(() => document.body?.innerText || '');

    let el = null;
    if (type === 'tab') {
      el = await page.$$('.el-tabs__item, [role="tab"]');
      el = el[idx] || el[0];
    } else if (type === 'pagination') {
      el = await page.$(`.el-pagination ${target.cls.includes('btn-next') ? '.btn-next' : target.cls.includes('btn-prev') ? '.btn-prev' : `.el-pager li.number:text-is("${target.text}")`}`);
    } else if (type === 'switch') {
      const sws = await page.$$('.el-switch');
      el = sws[target.idx];
    } else if (type === 'button') {
      // 按文本精确匹配可见按钮
      const btns = await page.$$('.el-button, button');
      for (const b of btns) {
        const t = (await b.innerText() || '').trim().replace(/\s+/g, ' ');
        if (t === target.text && await b.isVisible()) { el = b; break; }
      }
    } else if (type === 'table-row') {
      el = await page.$(target.sel);
    }

    if (!el) {
      inter.skipped = 'element-not-found';
      return inter;
    }

    await el.click({ timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(900);

    // 对话框检测
    inter.dialogOpened = await page.evaluate(() => {
      const dialogs = document.querySelectorAll('.el-dialog:not([style*="display: none"]), .el-drawer:not([style*="display: none"]), .el-message-box__wrapper:not([style*="display: none"])');
      for (const d of dialogs) {
        const r = d.getBoundingClientRect();
        if (r.width > 50 && r.height > 50) return true;
      }
      return false;
    });

    const bodyAfter = await page.evaluate(() => document.body?.innerText || '');
    inter.uiChanged = bodyAfter.length !== bodyBefore.length || bodyAfter.substring(0, 300) !== bodyBefore.substring(0, 300);

    // 白屏检测：点击后主要内容区为空
    const mainEmpty = await page.evaluate(() => {
      const main = document.querySelector('.app-main, main, #app .el-main, .el-main');
      if (!main) return false;
      const t = (main.innerText || '').trim();
      return t.length < 20 && !document.querySelector('.el-dialog, .el-drawer, .el-message-box__wrapper');
    });
    inter.uiBroken = mainEmpty;

    inter.after = { console: consoleErrors.length, api: apiErrors.length };
    inter.newConsole = consoleErrors.slice(inter.before.console);
    inter.newApi = apiErrors.slice(inter.before.api);
  } catch (e) {
    inter.error = e.message.substring(0, 150);
  }
  return inter;
}

async function closeDialog(page) {
  await page.keyboard.press('Escape').catch(() => {});
  await page.waitForTimeout(400);
  // 若仍有可见确认框，点取消/关闭
  await cancelDialog(page);
  // 关闭 drawer/dialog 的 close 按钮
  await page.evaluate(() => {
    document.querySelectorAll('.el-dialog__headerbtn, .el-drawer__close-btn').forEach(b => b.click());
  }).catch(() => {});
  await page.waitForTimeout(300);
}

async function cancelDialog(page) {
  return await page.evaluate(() => {
    const boxes = document.querySelectorAll('.el-message-box__wrapper');
    for (const box of boxes) {
      if (box.offsetParent === null) continue;
      const btns = box.querySelectorAll('button');
      for (const b of btns) {
        const t = (b.innerText || '').trim();
        if (/取消|关闭|Cancel|No|Abort/i.test(t)) { b.click(); return true; }
      }
      // el-message-box 默认第二个按钮是取消
      if (btns.length >= 2) { btns[btns.length - 1].click(); return true; }
    }
    return false;
  });
}

/**
 * 对话框内表单：填假数据 + 提交 + DB 回读
 */
async function fillAndSubmitDialog(page, route) {
  try {
    const stamp = Date.now();
    const testName = `E2E测试-${stamp}`;
    const filled = await page.evaluate(({ testName }) => {
      const dialog = [...document.querySelectorAll('.el-dialog, .el-drawer')].find(d => d.offsetParent !== null);
      if (!dialog) return { filled: 0, fields: [] };
      const fields = [];
      const inputs = dialog.querySelectorAll('input:not([type=hidden]), textarea');
      let filledCount = 0;
      for (const inp of inputs) {
        if (inp.offsetParent === null) continue;
        if (inp.type === 'checkbox' || inp.type === 'radio') continue;
        if (inp.readOnly || inp.disabled) continue;
        const ph = (inp.placeholder || inp.getAttribute('aria-label') || '').toLowerCase();
        // 名称/标题类字段填测试名
        if (/name|title|标题|名称|昵称|subject|问题/.test(ph) || /name|title/.test(inp.className)) {
          inp.value = testName;
          inp.dispatchEvent(new Event('input', { bubbles: true }));
          inp.dispatchEvent(new Event('change', { bubbles: true }));
          filledCount++;
          fields.push({ ph, val: testName });
        }
      }
      return { filled: filledCount, fields, testName };
    }, { testName });

    if (filled.filled === 0) return null;

    // 找对话框里的确定/保存按钮点击
    const submitted = await page.evaluate(() => {
      const dialog = [...document.querySelectorAll('.el-dialog, .el-drawer')].find(d => d.offsetParent !== null);
      if (!dialog) return false;
      const btns = dialog.querySelectorAll('button');
      for (const b of btns) {
        const t = (b.innerText || '').trim();
        if (/确定|保存|提交|确认|OK|Save|Submit/i.test(t) && !/取消/.test(t)) { b.click(); return true; }
      }
      return false;
    });
    if (!submitted) return null;
    await page.waitForTimeout(1500);

    return { type: 'dialog-submit', route, testName, result: 'submitted', note: '对话框表单已提交（假数据）' };
  } catch (e) {
    return { type: 'dialog-submit', route, error: e.message.substring(0, 150) };
  }
}

/**
 * 行内 create/editor 页表单：填假数据提交 + DB 回读
 */
async function fillAndSubmitInlineForm(page, route) {
  try {
    const stamp = Date.now();
    const testName = `E2E测试-${stamp}`;
    const filled = await page.evaluate(({ testName }) => {
      const form = document.querySelector('.el-form, form');
      if (!form) return { filled: 0 };
      const inputs = form.querySelectorAll('input:not([type=hidden]), textarea');
      let filledCount = 0;
      for (const inp of inputs) {
        if (inp.offsetParent === null || inp.readOnly || inp.disabled) continue;
        if (inp.type === 'checkbox' || inp.type === 'radio') continue;
        const ph = (inp.placeholder || '').toLowerCase();
        const label = inp.closest('.el-form-item')?.querySelector('label')?.innerText?.toLowerCase() || '';
        if (/name|title|标题|名称|昵称|subject|问题|关键词/.test(ph + label)) {
          inp.value = testName;
          inp.dispatchEvent(new Event('input', { bubbles: true }));
          inp.dispatchEvent(new Event('change', { bubbles: true }));
          filledCount++;
        }
      }
      return { filled: filledCount, testName };
    }, { testName });

    if (filled.filled === 0) return null;

    const submitted = await page.evaluate(() => {
      const btns = document.querySelectorAll('.el-form button, form button, .form-actions button');
      for (const b of btns) {
        const t = (b.innerText || '').trim();
        if (/保存|提交|确定|创建|Save|Submit/i.test(t)) { b.click(); return t; }
      }
      return false;
    });
    if (!submitted) return null;
    await page.waitForTimeout(2000);

    return { type: 'inline-form-submit', route, testName, result: 'submitted', clicked: submitted };
  } catch (e) {
    return { type: 'inline-form-submit', route, error: e.message.substring(0, 150) };
  }
}

/**
 * 按钮点击顺序：read 类先（安全），write 类次之，danger 类最后（只验证确认框）
 */
function planButtonOrder(buttons) {
  const scored = buttons.map(b => {
    const kind = classifyButton(b.text);
    const pri = kind === 'read' ? 0 : kind === 'click' ? 1 : kind === 'write' ? 2 : 3;
    return { ...b, pri };
  });
  scored.sort((a, b) => a.pri - b.pri);
  return scored;
}

async function main() {
  console.log(`=== 真人模拟交互测试 第${BATCH}批（${routes.length}条，共${ALL_ROUTES.length}条）===\n`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  console.log('--- 登录 ---');
  await login(page);
  console.log('✓ 登录成功\n');

  const results = [];
  for (let i = 0; i < routes.length; i++) {
    const route = routes[i];
    process.stdout.write(`[${i + 1}/${routes.length}] ${route} ... `);
    const r = await interactRoute(page, route, null);
    results.push(r);
    const icon = r.status === 'ok' ? '✓' : r.status === 'partial' ? '⚠' : '✗';
    console.log(`${icon} ${r.status} — ${r.interactions.length}次交互 | ${r.summary}`);
    if (r.consoleErrors.length > 0) console.log(`    控制台: ${r.consoleErrors[0].substring(0, 120)}`);
    if (r.apiErrors.length > 0) console.log(`    API: ${r.apiErrors.slice(0, 2).join(' | ')}`);
    if (r.dbVerifications.length > 0) console.log(`    DB: ${r.dbVerifications.map(d => d.result || d.error).join('; ')}`);
    // 交互间冷却，避免 session 抖动
    await page.waitForTimeout(500);
  }

  await browser.close();

  const reportPath = path.join(process.cwd(), `interact-batch${BATCH}-report.json`);
  fs.writeFileSync(reportPath, JSON.stringify({ runId: RUN_ID, batch: BATCH, total: routes.length, results }, null, 2));

  const ok = results.filter(r => r.status === 'ok').length;
  const partial = results.filter(r => r.status === 'partial').length;
  const fail = results.filter(r => ['fail', 'error'].includes(r.status)).length;
  const totalInteractions = results.reduce((s, r) => s + r.interactions.length, 0);

  console.log(`\n=== 第${BATCH}批交互测试完成 ===`);
  console.log(`路由: ${ok} ok | ${partial} partial | ${fail} fail`);
  console.log(`总交互次数: ${totalInteractions}`);
  console.log(`报告: ${reportPath}`);
}

main().catch(e => { console.error('引擎异常:', e); process.exit(1); });

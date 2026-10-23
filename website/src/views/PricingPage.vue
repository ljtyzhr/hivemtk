<script setup>
import PageHeader from '../components/PageHeader.vue'
import { pricingPage as page, moduleCatalog } from '../config/content.js'

const paidModules = moduleCatalog.filter((m) => m.category === 'paid')

function priceText(m) {
  if (m.billing === 'metered') return '¥0'
  if (m.priceCents === 0) return '免费'
  return '¥' + (m.priceCents / 100).toLocaleString('en-US')
}
function cycleKey(m) {
  if (m.billing === 'metered') return page.cycleMetered
  if (m.billing === 'once') return page.cycleOnce
  return m.priceCents === 0 ? '' : page.cycleYearly
}
</script>

<template>
  <div>
    <PageHeader
      :kicker="page.kicker"
      :title="page.title"
      :gradient-index="page.gradientIndex"
      :subtitle="page.subtitle"
      :breadcrumbs="[{ label: '首页', href: '/' }, { label: '按需订阅' }]"
    />

    <!-- 云端版引导条（T024 四入口之一，复用 page-cta-band 现有样式） -->
    <div class="container">
      <div class="page-cta-band">
        <p class="sec-desc">{{ $t(page.cloudBand.desc) }}</p>
        <a
          :href="page.cloudBand.href"
          target="_blank"
          rel="noopener noreferrer"
          class="btn btn-primary"
        >
          <span>{{ $t(page.cloudBand.label) }}</span>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
        </a>
      </div>
    </div>

    <!-- 三层模型 -->
    <section class="price-section">
      <div class="container">
        <div class="models-grid">
          <article v-for="(m, i) in page.models" :key="m.title" class="model-card" :class="{ 'model-card-featured': i === 2 }">
            <div class="model-index">{{ String(i + 1).padStart(2, '0') }}</div>
            <h3 class="model-title">{{ $t(m.title) }}</h3>
            <div class="model-price">{{ $t(m.price) }}</div>
            <div class="model-note">{{ $t(m.note) }}</div>
            <p class="model-desc">{{ $t(m.desc) }}</p>
          </article>
        </div>
      </div>
    </section>

    <!-- 价目表 -->
    <section class="price-section price-section-alt">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.tableTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.tableDesc) }}</p>
        </header>
        <div class="price-table-wrap">
          <table class="price-table">
            <thead>
              <tr>
                <th>{{ $t(page.colModule) }}</th>
                <th class="col-summary">{{ $t(page.colSummary) }}</th>
                <th class="col-price">{{ $t(page.colPrice) }}</th>
                <th class="col-cycle">{{ $t(page.colCycle) }}</th>
                <th class="col-menus">{{ $t(page.colMenus) }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="m in paidModules" :key="m.key">
                <td class="cell-name">{{ $t(m.name) }}</td>
                <td class="cell-summary">{{ $t(m.summary) }}</td>
                <td class="cell-price">{{ priceText(m) }}</td>
                <td class="cell-cycle"><span v-if="cycleKey(m)" class="cycle-chip">{{ $t(cycleKey(m)) }}</span></td>
                <td class="cell-menus">{{ m.menus }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <!-- 计费口径（三条花费路线） -->
    <section class="price-section">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.budgetTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.budgetDesc) }}</p>
        </header>
        <div class="budget-grid">
          <article
            v-for="(path, i) in page.budgetPaths"
            :key="path.name"
            class="budget-card"
            :class="{ 'budget-card-mid': i === 1 }"
          >
            <div class="budget-head">
              <span class="budget-tier">{{ String(i + 1).padStart(2, '0') }}</span>
              <h3 class="budget-name">{{ $t(path.name) }}</h3>
              <span class="budget-sub">{{ $t(path.subtitle) }}</span>
            </div>
            <div class="budget-price">{{ $t(path.price) }}</div>
            <div class="budget-price-note">{{ $t(path.priceNote) }}</div>
            <ul class="budget-includes">
              <li v-for="(item, j) in path.includes" :key="j">{{ $t(item) }}</li>
            </ul>
            <p class="budget-scene">{{ $t(path.scene) }}</p>
          </article>
        </div>
        <div class="budget-points">
          <article v-for="pt in page.budgetPoints" :key="pt.title" class="budget-point">
            <h4 class="point-title">{{ $t(pt.title) }}</h4>
            <p class="point-desc">{{ $t(pt.desc) }}</p>
          </article>
        </div>
        <p class="budget-note">{{ $t(page.budgetNote) }}</p>
      </div>
    </section>

    <!-- 订阅流程 -->
    <section class="price-section">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.flowTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.flowDesc) }}</p>
        </header>
        <ol class="flow-list">
          <li v-for="(step, i) in page.flow" :key="step.title" class="flow-step">
            <span class="flow-num">{{ i + 1 }}</span>
            <div class="flow-body">
              <h3 class="flow-title">{{ $t(step.title) }}</h3>
              <p class="flow-desc">{{ $t(step.desc) }}</p>
            </div>
          </li>
        </ol>
      </div>
    </section>

    <!-- 模型算力双轨 -->
    <section class="price-section price-section-alt">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.llmTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.llmDesc) }}</p>
        </header>
        <div class="tracks-grid">
          <article v-for="track in page.llmTracks" :key="track.title" class="track-card glass-card">
            <h3 class="track-title">{{ $t(track.title) }}</h3>
            <div class="track-price">{{ $t(track.price) }}</div>
            <p class="track-desc">{{ $t(track.desc) }}</p>
          </article>
        </div>
        <div class="offline-card">
          <h3 class="offline-title">{{ $t(page.offlineTitle) }}</h3>
          <p class="offline-desc">{{ $t(page.offlineDesc) }}</p>
        </div>
      </div>
    </section>

    <!-- SaaS 功能 -->
    <section class="price-section">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.saasTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.saasDesc) }}</p>
        </header>
        <div class="saas-grid">
          <article v-for="p in page.saasPoints" :key="p.title" class="saas-card">
            <h3 class="saas-title">{{ $t(p.title) }}</h3>
            <p class="saas-desc">{{ $t(p.desc) }}</p>
          </article>
        </div>
        <!-- 云端版开箱试用 CTA -->
        <div class="saas-trial">
          <p class="saas-trial-desc">{{ $t(page.saasTrial.desc) }}</p>
          <a
            :href="page.saasTrial.href"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-primary"
          >
            <span>{{ $t(page.saasTrial.label) }}</span>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
          </a>
        </div>
      </div>
    </section>

    <!-- FAQ -->
    <section class="price-section price-section-alt">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">{{ $t(page.faqsTitle[0]) }}</h2>
        </header>
        <div class="faq-list">
          <details v-for="faq in page.faqs" :key="faq.q" class="faq-item">
            <summary class="faq-q">{{ $t(faq.q) }}</summary>
            <p class="faq-a">{{ $t(faq.a) }}</p>
          </details>
        </div>
      </div>
    </section>

    <!-- CTA -->
    <section class="price-section">
      <div class="container">
        <div class="page-cta-band">
          <router-link :to="page.ctaModules.href" class="btn btn-ghost">{{ $t(page.ctaModules.label) }}</router-link>
          <router-link :to="page.ctaDeploy.href" class="btn btn-primary">{{ $t(page.ctaDeploy.label) }}</router-link>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.price-section {
  padding: 56px 0;
}
.price-section-alt {
  background: var(--bg-subtle);
}
.sec-head {
  max-width: 860px;
  margin-bottom: 32px;
}
.sec-title {
  font-family: var(--font-display);
  font-size: clamp(1.7rem, 3.6vw, 2.4rem);
  font-weight: 900;
  line-height: 1.14;
  letter-spacing: -0.026em;
  color: var(--text);
  margin-bottom: 14px;
}
.sec-desc {
  font-size: 1.02rem;
  color: var(--text-muted);
  line-height: 1.75;
}

/* —— 三层模型卡 —— */
.models-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px;
}
.model-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 28px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.model-card-featured {
  border-color: var(--primary);
  box-shadow: 0 6px 24px rgba(200, 57, 47, 0.1);
}
.model-index {
  font-family: var(--font-display);
  font-size: 1.4rem;
  font-weight: 900;
  color: var(--primary);
  letter-spacing: -0.03em;
}
.model-title {
  font-family: var(--font-display);
  font-size: 1.25rem;
  font-weight: 800;
  color: var(--text);
}
.model-price {
  font-family: var(--font-display);
  font-size: 1.5rem;
  font-weight: 900;
  color: var(--primary);
  letter-spacing: -0.02em;
}
.model-note {
  font-size: 0.76rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.04em;
  margin-bottom: 6px;
}
.model-desc {
  font-size: 0.92rem;
  color: var(--text-soft);
  line-height: 1.72;
}

/* —— 价目表 —— */
.price-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  background: var(--bg-surface);
}
.price-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
  min-width: 760px;
}
.price-table th {
  text-align: left;
  font-family: var(--font-display);
  font-size: 0.78rem;
  font-weight: 800;
  letter-spacing: 0.06em;
  color: var(--text-muted);
  padding: 14px 16px;
  border-bottom: 2px solid var(--text);
  white-space: nowrap;
}
.price-table td {
  padding: 13px 16px;
  border-bottom: 1px solid var(--border);
  color: var(--text-soft);
  vertical-align: top;
}
.price-table tbody tr:hover {
  background: var(--bg-subtle);
}
.cell-name {
  font-weight: 700;
  color: var(--text);
  white-space: nowrap;
}
.cell-summary {
  line-height: 1.6;
  min-width: 260px;
}
.cell-price {
  font-family: var(--font-display);
  font-weight: 900;
  color: var(--primary);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.cycle-chip {
  display: inline-block;
  font-size: 0.72rem;
  font-weight: 700;
  padding: 3px 8px;
  border-radius: 4px;
  background: var(--primary-soft);
  color: var(--primary);
  white-space: nowrap;
}
.col-price,
.col-cycle,
.col-menus,
.cell-price,
.cell-cycle,
.cell-menus {
  text-align: right;
}

/* —— 计费口径 —— */
.budget-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px;
}
.budget-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 26px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.budget-card-mid {
  border-color: var(--primary);
  box-shadow: 0 6px 24px rgba(200, 57, 47, 0.1);
}
.budget-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
}
.budget-tier {
  font-family: var(--font-display);
  font-size: 1.2rem;
  font-weight: 900;
  color: var(--primary);
  letter-spacing: -0.03em;
}
.budget-name {
  font-family: var(--font-display);
  font-size: 1.1rem;
  font-weight: 800;
  color: var(--text);
}
.budget-sub {
  font-size: 0.76rem;
  color: var(--text-muted);
  width: 100%;
}
.budget-price {
  font-family: var(--font-display);
  font-size: 1.6rem;
  font-weight: 900;
  color: var(--primary);
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
}
.budget-price-note {
  font-size: 0.76rem;
  font-weight: 600;
  color: var(--text-muted);
  margin-bottom: 6px;
}
.budget-includes {
  list-style: none;
  margin: 0;
  padding: 12px 0 0;
  border-top: 1px dashed var(--border);
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
}
.budget-includes li {
  position: relative;
  padding-left: 16px;
  font-size: 0.86rem;
  color: var(--text-soft);
  line-height: 1.62;
}
.budget-includes li::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
}
.budget-scene {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-muted);
  padding-top: 10px;
  border-top: 1px solid var(--border);
}
.budget-points {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 14px;
  margin-top: 20px;
}
.budget-point {
  background: var(--bg-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 18px 20px;
}
.point-title {
  font-family: var(--font-display);
  font-size: 0.98rem;
  font-weight: 800;
  color: var(--text);
  margin-bottom: 6px;
}
.point-desc {
  font-size: 0.86rem;
  color: var(--text-soft);
  line-height: 1.68;
}
.budget-note {
  margin-top: 18px;
  font-family: var(--font-mono);
  font-size: 0.74rem;
  color: var(--text-dim);
  line-height: 1.7;
  border-left: 3px solid var(--border-strong);
  padding-left: 14px;
}

/* —— 订阅流程 —— */
.flow-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 14px;
  counter-reset: step;
}
.flow-step {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 22px 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.flow-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background: var(--primary);
  color: #fff;
  font-family: var(--font-display);
  font-weight: 900;
  font-size: 0.9rem;
}
.flow-title {
  font-family: var(--font-display);
  font-size: 1rem;
  font-weight: 800;
  color: var(--text);
}
.flow-desc {
  font-size: 0.84rem;
  color: var(--text-soft);
  line-height: 1.68;
}

/* —— 双轨 —— */
.tracks-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
}
.track-card {
  padding: 26px;
}
.track-title {
  font-family: var(--font-display);
  font-size: 1.15rem;
  font-weight: 800;
  color: var(--text);
  margin-bottom: 8px;
}
.track-price {
  font-family: var(--font-display);
  font-weight: 900;
  font-size: 1.05rem;
  color: var(--primary);
  margin-bottom: 12px;
}
.track-desc {
  font-size: 0.92rem;
  color: var(--text-soft);
  line-height: 1.72;
}
.offline-card {
  margin-top: 20px;
  padding: 24px 26px;
  background: var(--bg-surface);
  border: 1px dashed var(--border-strong);
  border-radius: var(--radius-lg);
}
.offline-title {
  font-family: var(--font-display);
  font-size: 1.02rem;
  font-weight: 800;
  color: var(--text);
  margin-bottom: 8px;
}
.offline-desc {
  font-size: 0.9rem;
  color: var(--text-soft);
  line-height: 1.72;
}

/* —— SaaS —— */
.saas-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}
.saas-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 24px;
}
.saas-title {
  font-family: var(--font-display);
  font-size: 1.05rem;
  font-weight: 800;
  color: var(--text);
  margin-bottom: 10px;
}
.saas-desc {
  font-size: 0.9rem;
  color: var(--text-soft);
  line-height: 1.72;
}
.saas-trial {
  margin-top: 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 24px;
}
.saas-trial-desc {
  flex: 1;
  min-width: 260px;
  font-size: 0.95rem;
  color: var(--text-soft);
  line-height: 1.72;
}

/* —— FAQ —— */
.faq-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 900px;
}
.faq-item {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 18px 22px;
}
.faq-q {
  font-family: var(--font-display);
  font-size: 1rem;
  font-weight: 800;
  color: var(--text);
  cursor: pointer;
}
.faq-a {
  margin-top: 12px;
  font-size: 0.92rem;
  color: var(--text-soft);
  line-height: 1.75;
}

/* —— CTA —— */
.page-cta-band {
  display: flex;
  justify-content: center;
  gap: 14px;
  flex-wrap: wrap;
  padding: 30px;
  background: var(--bg-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
}

@media (max-width: 992px) {
  .models-grid,
  .saas-grid,
  .budget-grid {
    grid-template-columns: 1fr;
  }
  .budget-points {
    grid-template-columns: 1fr;
  }
  .flow-list {
    grid-template-columns: repeat(2, 1fr);
  }
  .tracks-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 560px) {
  .flow-list {
    grid-template-columns: 1fr;
  }
}
</style>

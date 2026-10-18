<script setup>
import PageHeader from '../components/PageHeader.vue'
import { modulesPage as page, moduleCatalog } from '../config/content.js'

const baseModules = moduleCatalog.filter((m) => m.category === 'base')
const paidModules = moduleCatalog.filter((m) => m.category === 'paid')

function priceText(m) {
  if (m.billing === 'base') return ''
  if (m.billing === 'metered') return '¥0'
  return '¥' + (m.priceCents / 100).toLocaleString('en-US')
}
function cycleText(m) {
  if (m.billing === 'base') return ''
  if (m.billing === 'metered') return page.cycleMetered
  return page.cycleYearly
}
</script>

<template>
  <div>
    <PageHeader
      :kicker="page.kicker"
      :title="page.title"
      :gradient-index="page.gradientIndex"
      :subtitle="page.subtitle"
      :breadcrumbs="[{ label: '首页', href: '/' }, { label: '功能架构' }]"
    />

    <!-- 数据指标 -->
    <section class="mod-stats">
      <div class="container stats-grid">
        <div v-for="s in page.stats" :key="s.label" class="stat-card">
          <div class="stat-value">{{ s.value }}</div>
          <div class="stat-label">{{ $t(s.label) }}</div>
          <div class="stat-sub">{{ $t(s.sub) }}</div>
        </div>
      </div>
    </section>

    <!-- 双形态 -->
    <section class="mod-section">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.modesTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.modesDesc) }}</p>
        </header>
        <div class="modes-grid">
          <article v-for="mode in page.modes" :key="mode.title" class="mode-card glass-card">
            <h3 class="mode-title">{{ $t(mode.title) }}</h3>
            <div class="mode-tags">
              <span v-for="tag in mode.tags" :key="tag" class="mode-tag">{{ tag }}</span>
            </div>
            <p class="mode-desc">{{ $t(mode.desc) }}</p>
          </article>
        </div>
      </div>
    </section>

    <!-- 基础模块 -->
    <section class="mod-section mod-section-alt">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.baseTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.baseDesc) }}</p>
        </header>
        <div class="mod-grid">
          <article v-for="m in baseModules" :key="m.key" class="mod-card">
            <div class="mod-head">
              <h3 class="mod-name">{{ $t(m.name) }}</h3>
              <span class="mod-badge badge-base">{{ $t('基础模块') }}</span>
            </div>
            <p class="mod-summary">{{ $t(m.summary) }}</p>
            <div class="mod-meta">
              <span class="meta-chip">{{ m.menus }} {{ $t('个页面') }}</span>
              <span class="meta-chip meta-free">{{ $t('免费常开') }}</span>
            </div>
            <div v-if="m.depends.length" class="mod-depends">
              <span class="depends-label">{{ $t('依赖') }}</span>
              <span v-for="d in m.depends" :key="d" class="depends-chip">{{ $t(d) }}</span>
            </div>
          </article>
        </div>
      </div>
    </section>

    <!-- 增值模块 -->
    <section class="mod-section">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.paidTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="sec-desc">{{ $t(page.paidDesc) }}</p>
        </header>
        <div class="mod-grid">
          <article v-for="m in paidModules" :key="m.key" class="mod-card mod-card-paid">
            <div class="mod-head">
              <h3 class="mod-name">{{ $t(m.name) }}</h3>
              <span class="mod-badge badge-paid">{{ $t('增值模块') }}</span>
            </div>
            <p class="mod-summary">{{ $t(m.summary) }}</p>
            <div class="mod-meta">
              <span class="meta-price">{{ priceText(m) }}</span>
              <span v-if="cycleText(m)" class="meta-chip">{{ $t(cycleText(m)) }}</span>
              <span class="meta-chip">{{ m.menus }} {{ $t('个页面') }}</span>
            </div>
            <div v-if="m.depends.length" class="mod-depends">
              <span class="depends-label">{{ $t('依赖') }}</span>
              <span v-for="d in m.depends" :key="d" class="depends-chip">{{ $t(d) }}</span>
            </div>
          </article>
        </div>
      </div>
    </section>

    <!-- 运行机制 -->
    <section class="mod-section mod-section-alt">
      <div class="container">
        <header class="sec-head">
          <h2 class="sec-title">
            <template v-for="(line, i) in page.mechTitle" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
        </header>
        <div class="mech-grid">
          <article v-for="(mech, i) in page.mechanics" :key="mech.title" class="mech-card">
            <span class="mech-num">{{ String(i + 1).padStart(2, '0') }}</span>
            <h3 class="mech-title">{{ $t(mech.title) }}</h3>
            <p class="mech-desc">{{ $t(mech.desc) }}</p>
          </article>
        </div>
        <p class="source-note">{{ $t(page.sourceNote) }}</p>
      </div>
    </section>

    <!-- 页尾 CTA -->
    <section class="mod-section">
      <div class="container">
        <div class="page-cta-band">
          <router-link :to="page.ctaModules.href" class="btn btn-ghost">{{ $t(page.ctaModules.label) }}</router-link>
          <router-link :to="page.ctaPricing.href" class="btn btn-primary">{{ $t(page.ctaPricing.label) }}</router-link>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* —— 指标条 —— */
.mod-stats {
  padding: 40px 0 8px;
}
.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}
.stat-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 22px 20px;
}
.stat-value {
  font-family: var(--font-display);
  font-size: 2.2rem;
  font-weight: 900;
  letter-spacing: -0.03em;
  color: var(--primary);
  line-height: 1;
  font-variant-numeric: tabular-nums;
}
.stat-label {
  font-family: var(--font-display);
  font-weight: 800;
  font-size: 0.98rem;
  color: var(--text);
  margin-top: 10px;
}
.stat-sub {
  font-size: 0.78rem;
  color: var(--text-muted);
  margin-top: 4px;
  letter-spacing: 0.02em;
}

/* —— 通用区块 —— */
.mod-section {
  padding: 56px 0;
}
.mod-section-alt {
  background: var(--bg-subtle);
}
.sec-head {
  max-width: 860px;
  margin-bottom: 36px;
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

/* —— 双形态 —— */
.modes-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
}
.mode-card {
  padding: 26px;
}
.mode-title {
  font-family: var(--font-display);
  font-size: 1.2rem;
  font-weight: 800;
  color: var(--text);
  margin-bottom: 12px;
}
.mode-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 14px;
}
.mode-tag {
  font-family: var(--font-mono);
  font-size: 0.72rem;
  padding: 3px 9px;
  border-radius: 4px;
  background: var(--primary-soft);
  color: var(--primary);
  border: 1px solid var(--primary-border);
}
.mode-desc {
  font-size: 0.93rem;
  color: var(--text-soft);
  line-height: 1.72;
}

/* —— 模块卡 —— */
.mod-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}
.mod-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 22px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}
.mod-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
  border-color: var(--primary);
}
.mod-card-paid {
  border-left: 3px solid var(--primary);
}
.mod-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.mod-name {
  font-family: var(--font-display);
  font-size: 1.08rem;
  font-weight: 800;
  color: var(--text);
  letter-spacing: -0.01em;
}
.mod-badge {
  font-size: 0.68rem;
  font-weight: 700;
  padding: 3px 8px;
  border-radius: 4px;
  white-space: nowrap;
}
.badge-base {
  background: var(--accent-soft);
  color: var(--accent);
}
.badge-paid {
  background: var(--primary);
  color: #fff;
}
.mod-summary {
  font-size: 0.88rem;
  color: var(--text-soft);
  line-height: 1.68;
  flex: 1;
}
.mod-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding-top: 10px;
  border-top: 1px dashed var(--border);
}
.meta-price {
  font-family: var(--font-display);
  font-weight: 900;
  font-size: 1.02rem;
  color: var(--primary);
  font-variant-numeric: tabular-nums;
}
.meta-chip {
  font-size: 0.72rem;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 4px;
  background: var(--bg-subtle);
  color: var(--text-muted);
  border: 1px solid var(--border);
}
.meta-free {
  background: var(--accent-soft);
  color: var(--accent);
  border-color: transparent;
}
.mod-depends {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.depends-label {
  font-size: 0.72rem;
  color: var(--text-dim);
}
.depends-chip {
  font-size: 0.7rem;
  padding: 2px 7px;
  border-radius: 4px;
  background: var(--bg-subtle);
  color: var(--text-muted);
}

/* —— 机制 —— */
.mech-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}
.mech-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 24px;
}
.mech-num {
  font-family: var(--font-display);
  font-size: 1.3rem;
  font-weight: 900;
  color: var(--primary);
  letter-spacing: -0.03em;
}
.mech-title {
  font-family: var(--font-display);
  font-size: 1.05rem;
  font-weight: 800;
  color: var(--text);
  margin: 8px 0 10px;
}
.mech-desc {
  font-size: 0.9rem;
  color: var(--text-soft);
  line-height: 1.72;
}
.source-note {
  margin-top: 24px;
  font-family: var(--font-mono);
  font-size: 0.76rem;
  color: var(--text-dim);
  line-height: 1.7;
  border-left: 3px solid var(--border-strong);
  padding-left: 14px;
}

/* —— 页尾 —— */
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
  .mod-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
@media (max-width: 860px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  .modes-grid,
  .mech-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 560px) {
  .mod-grid {
    grid-template-columns: 1fr;
  }
}
</style>

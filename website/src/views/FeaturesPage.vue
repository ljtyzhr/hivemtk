<script setup>
import PageHeader from '../components/PageHeader.vue'
import FeaturesSection from '../components/FeaturesSection.vue'
import WhyHiveMTKSection from '../components/WhyHiveMTKSection.vue'
import { featuresBand } from '../config/content.js'
</script>

<template>
  <div>
    <PageHeader
      :kicker="'13 项核心能力'"
      :title="['不是又一个 SCRM，', '是真正会卖货的 AI']"
      :gradient-index="1"
      :subtitle="'HiveMTK 覆盖从七端多账号聚合、AI 谈单、销冠 SOP、客户 CDP、全渠道触达、GEO 智能优化到数据驾驶舱的完整链路，94 个业务模块 + 45 个智能体工具，让系统真正能替代销冠的重复劳动。'"
      :breadcrumbs="[{ label: '首页', href: '/' }, { label: '核心功能' }]"
    />
    <FeaturesSection hide-header />
    <WhyHiveMTKSection />

    <!-- 功能架构与按需订阅引流带 -->
    <section class="features-band">
      <div class="container band-inner">
        <div class="band-text">
          <h2 class="band-title">
            <template v-for="(line, i) in featuresBand.title" :key="i">
              <span v-if="i === 1" class="ink-mark">{{ $t(line) }}</span>
              <template v-else>{{ $t(line) }}</template>
            </template>
          </h2>
          <p class="band-desc">{{ $t(featuresBand.desc) }}</p>
        </div>
        <div class="band-actions">
          <router-link :to="featuresBand.ctaArchitecture.href" class="btn btn-ghost">{{ $t(featuresBand.ctaArchitecture.label) }}</router-link>
          <router-link :to="featuresBand.ctaPricing.href" class="btn btn-primary">{{ $t(featuresBand.ctaPricing.label) }}</router-link>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.features-band {
  padding: 56px 0 var(--space-section);
  background: var(--bg-base);
}
.band-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 32px;
  flex-wrap: wrap;
  padding: 40px;
  background: var(--bg-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
}
.band-text {
  flex: 1;
  min-width: 280px;
  max-width: 720px;
}
.band-title {
  font-family: var(--font-display);
  font-size: clamp(1.5rem, 3vw, 2rem);
  font-weight: 900;
  line-height: 1.16;
  letter-spacing: -0.024em;
  color: var(--text);
  margin-bottom: 12px;
}
.band-desc {
  font-size: 0.98rem;
  color: var(--text-muted);
  line-height: 1.75;
}
.band-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
@media (max-width: 768px) {
  .band-inner {
    padding: 28px;
    flex-direction: column;
    align-items: stretch;
    text-align: center;
  }
  .band-actions {
    justify-content: center;
  }
}
</style>

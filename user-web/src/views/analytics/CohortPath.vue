<template>
  <div class="cohort-path">
    <el-tabs v-model="activeTab">
      <el-tab-pane label="Cohort 留存" name="cohort">
        <el-card>
          <el-empty v-if="cohortEmpty" description="暂无留存数据 — 有客户访问后将按周聚合展示" :image-size="80" />
          <div v-show="!cohortEmpty" ref="cohortChartRef" class="chart" />
        </el-card>
      </el-tab-pane>
      <el-tab-pane label="行为路径" name="path">
        <el-card>
          <el-empty v-if="pathEmpty" description="暂无行为路径数据 — 需要更多会话行为积累" :image-size="80" />
          <div v-show="!pathEmpty" ref="pathChartRef" class="chart" />
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, nextTick, watch } from 'vue';
import { safeInit } from '@/utils/echarts'
import { http } from '@/utils/request'

const activeTab = ref('cohort')
const cohortChartRef = ref()
const pathChartRef = ref()
const cohortEmpty = ref(false)
const pathEmpty = ref(false)
const options = reactive({ cohort: null, path: null })

// 未激活的页签是 v-show 藏起来的，容器宽高为 0。此时 init，echarts 会把画布宽度回退成
// 100px，切过来就是一张挤在左侧、标签互相压着读的图。所以数据先存着，只画当前页签，
// 换页签时再补画。
async function paint(key) {
  await nextTick()
  if (activeTab.value !== key) return
  const el = key === 'cohort' ? cohortChartRef.value : pathChartRef.value
  const option = options[key]
  if (!el || !option) return
  const chart = safeInit(el)
  chart.setOption(option)
  chart.resize()
}

async function loadCohort() {
  const data = await http.get('/api/analytics/cohort', { period: 'weekly' })
  const days = data.periods || []
  const cohorts = data.cohorts || []
  cohortEmpty.value = cohorts.every((c) => !c.size)
  if (cohortEmpty.value) return
  options.cohort = {
    tooltip: { trigger: 'item' },
    xAxis: { type: 'category', data: days },
    yAxis: { type: 'category', data: cohorts.map((c) => c.label) },
    visualMap: { min: 0, max: 100, calculable: true, orient: 'vertical', left: 'right' },
    series: [{
      type: 'heatmap',
      data: cohorts.flatMap((c) => c.retention.map((r, i) => [i, c.label, r])),
      label: { show: true, formatter: (p) => p.value[2] + '%' }
    }]
  }
  await paint('cohort')
}

async function loadPath() {
  const data = await http.get('/api/analytics/path', { limit: 5 })
  pathEmpty.value = !(data.nodes || []).length
  if (pathEmpty.value) return
  options.path = {
    tooltip: {},
    series: [{
      type: 'sankey',
      data: data.nodes || [],
      links: data.links || []
    }]
  }
  await paint('path')
}

onMounted(() => {
  loadCohort()
  loadPath()
})
watch(activeTab, paint)
</script>

<style scoped>
.cohort-path { padding: 16px; }
.chart { width: 100%; height: 480px; }
</style>

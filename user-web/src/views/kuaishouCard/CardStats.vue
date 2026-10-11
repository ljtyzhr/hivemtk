<template>
  <div class="kuaishou-card-stats-container">
    <el-card class="box-card">
      <template #header>
        <div class="card-header">
          <span>快手卡片统计 - {{ cardInfo.title ?? (cardLoadFailed ? '加载失败' : '加载中...') }}</span>
          <div class="header-actions">
            <el-button @click="goBack">{{ $t('返回') }}</el-button>
            <el-date-picker
              v-model="dateRange"
              type="daterange"
              range-separator="至"
              start-placeholder="开始日期"
              end-placeholder="结束日期"
              @change="handleDateChange"
            />
            <el-select v-model="groupBy" :placeholder="$t('分组方式')" @change="handleGroupByChange">
              <el-option :label="$t('按天')" value="day" />
              <el-option :label="$t('按周')" value="week" />
              <el-option :label="$t('按月')" value="month" />
            </el-select>
            <el-button type="primary" @click="refreshData">{{ $t('刷新') }}</el-button>
          </div>
        </div>
      </template>

      <el-alert
        v-if="cardLoadFailed || statsLoadFailed"
        class="stats-alert"
        type="warning"
        show-icon
        :closable="false"
        :title="cardLoadFailed ? '这张卡片读不到了' : '统计数据读取失败'"
        :description="cardLoadFailed ? '卡片已被删除，或链接里的 id 不属于当前账号。下方的 0 与空图是「没读到」，不是「真的没有」。' : '卡片本身还在，但这一轮的统计没读到。下方的 0 与空图是「没读到」，不是「真的没有」——请点「刷新」重试。'"
      />

      
      <div class="card-info-section" v-if="cardInfo.id">
        <el-row :gutter="20">
          <el-col :span="6">
            <el-image
              v-if="cardInfo.image_url"
              style="width: 100%; height: 200px"
              :src="cardInfo.image_url"
              :preview-src-list="[cardInfo.image_url]"
              fit="cover"
            >
              <template #error>
                <div class="img-error-fallback">{{ $t('图片加载失败') }}</div>
              </template>
            </el-image>
          </el-col>
          <el-col :span="18">
            <el-descriptions :column="2" border>
              <el-descriptions-item label="卡片ID">{{ cardInfo.id }}</el-descriptions-item>
              <el-descriptions-item label="标题">{{ cardInfo.title }}</el-descriptions-item>
              <el-descriptions-item label="状态">
                <el-tag :type="cardInfo.is_active ? 'success' : 'danger'">
                  {{ cardInfo.is_active ? '激活' : '禁用' }}
                </el-tag>
              </el-descriptions-item>
              <el-descriptions-item label="创建时间">{{ cardInfo.created_at }}</el-descriptions-item>
              <el-descriptions-item label="描述" :span="2">{{ cardInfo.description }}</el-descriptions-item>
            </el-descriptions>
          </el-col>
        </el-row>
      </div>

      
      <div class="stats-overview">
        <el-row :gutter="20">
          <el-col :span="24">
            <el-card class="stats-card">
              <div class="stats-item">
                <div class="stats-value">{{ formatNumber(cardStats.totalViews) }}</div>
                <div class="stats-label">总浏览量</div>
              </div>
            </el-card>
          </el-col>
        </el-row>
      </div>

      
      <div class="charts-section">
        <el-row :gutter="20">
          <el-col :span="24">
            <el-card>
              <template #header>
                <div class="card-header">
                  <span>访问趋势</span>
                </div>
              </template>
              <div ref="visitTrendChart" class="chart-container"></div>
            </el-card>
          </el-col>
        </el-row>
      </div>

      
      <div class="bottom-section">
        <el-row :gutter="20">
          <el-col :span="24">
            <el-card>
              <template #header>
                <div class="card-header">
                  <span>最近活动</span>
                </div>
              </template>
              <el-table :data="cardStats.recentActivity.filter(item => item.action === 'view')" style="width: 100%">
                <el-table-column prop="username" label="用户" />
                <el-table-column prop="action" label="操作">
                  <template #default="scope">
                    <el-tag :type="getActionType(scope.row.action)">
                      {{ getActionText(scope.row.action) }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="ipAddress" label="IP地址" />
                <el-table-column prop="createdAt" label="时间">
                  <template #default="scope">
                    {{ formatDate(scope.row.createdAt) }}
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </el-col>
        </el-row>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { safeInit } from '@/utils/echarts'
import { getKuaishouCard, getKuaishouCardStats } from '@/api/kuaishouCard'

const route = useRoute();
const router = useRouter()

const loading = ref(false);
const cardId = ref(route.params.id)
const cardInfo = ref({})
// 模板头部的「加载失败」与告警条都要有真值可依：这两个 ref 先前只在模板里出现、
// script 里从未声明，于是 v-if 恒假、头部永远停在「加载中...」。
const cardLoadFailed = ref(false)
const statsLoadFailed = ref(false)
const dateRange = ref([])
const groupBy = ref('day')

const cardStats = reactive({
  cardId: 0,
  cardTitle: '',
  // 键名跟响应走：dto/kuaishoucardstats.go 的单卡响应是 totalViews（不是抖音的 viewCount，
  // 也不是本文件原来写的 view_count），原先三个名字互不认账 ⇒ 「总浏览量」这一格恒 0。
  totalViews: 0,
  dailyStats: [],
  recentActivity: []
});

const visitTrendChart = ref(null);
let visitChartInstance = null

const fetchCardInfo = async () => {
  try {
    const res = await getKuaishouCard(cardId.value)
    cardInfo.value = res || {}
    cardLoadFailed.value = !res?.id
  } catch (error) {
    // 拦截器已经按状态码弹过一次，这里再弹一条是两句同义 toast；
    // 页面用常驻的 cardLoadFailed 说结论——toast 三秒就没了，头部的「加载中...」不会。
    cardInfo.value = {}
    cardLoadFailed.value = true
    console.error(error)
  }
};

const fetchCardStats = async () => {
  try {
    const params = {
      groupBy: groupBy.value
    }
    
    if (dateRange.value && dateRange.value.length === 2) {
      // 快手这条接口和抖音/小红书不一样：dto.KuaishouCardStatsRequest 的 StartDate/EndDate 是
      // time.Time 且没挂 time_format，gin 只按 RFC3339 解析。原来这里发的是 start_date=2026-09-11
      // 这种 snake + 纯日期：键名不绑定（日期区间控件是个哑控件），改成 camel 的纯日期又会 400。
      // 带毫秒的 Z 形 ISO 串两端都成立，实测 http=200 且区间真生效。
      params.startDate = new Date(dateRange.value[0]).toISOString()
      // 选择器回的是当天 00:00 的 Date：直接发出去会把结束日那天的浏览全 cut 掉，
      // 所以右端要抬到当天末尾，让区间是闭区间。
      const rangeEnd = new Date(dateRange.value[1])
      rangeEnd.setHours(23, 59, 59, 999)
      params.endDate = rangeEnd.toISOString()
    }
    
    const res = await getKuaishouCardStats(cardId.value, params)
    Object.assign(cardStats, res);
    statsLoadFailed.value = false
    if (!Array.isArray(cardStats.dailyStats))
      cardStats.dailyStats = [];
    if (!Array.isArray(cardStats.recentActivity)) cardStats.recentActivity = []

    nextTick(() => {
      initCharts()
    });
  } catch (error) {
    // 同上：统计读失败时拦截器已经出声，页面把「这一屏的 0 不是真值」常驻说出口。
    statsLoadFailed.value = true
    console.error(error)
  }
};

const initCharts = () => {
  if (visitChartInstance) {
    visitChartInstance.dispose()
  }

  if (visitTrendChart.value) {
    visitChartInstance = safeInit(visitTrendChart.value)
    const visitOption = {
      tooltip: {
        trigger: 'axis'
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '3%',
        containLabel: true
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        // dto.DailyStat.date 在快手这条链路上是完整时间戳（"2026-09-30T00:00:00Z"），
        // 抖音那条是 YYYY-MM-DD。直接喂给类目轴会让横轴印一长串 ISO，这里统一截成日期。
        data: cardStats.dailyStats.map(item => formatDate(item.date))
      },
      yAxis: {
        type: 'value'
      },
      series: [
        {
          name: '浏览量',
          type: 'line',
          smooth: true,
          data: cardStats.dailyStats.map(item => item.view),
          itemStyle: {
            color: '#2684FF'
          },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(38, 132, 255, 0.3)' },
              { offset: 1, color: 'rgba(38, 132, 255, 0.1)' }
            ])
          }
        }
      ]
    }
    visitChartInstance.setOption(visitOption)
  }
};

// 监听器只在 onMounted 注册一次（原先挂在 initCharts 里，每次刷新都调一遍）：
// handleResize 引用稳定时浏览器会去重，但卸载时也必须能按同一个引用摘掉。
const handleResize = () => {
  if (visitChartInstance) visitChartInstance.resize()
};

const handleDateChange = () => {
  fetchCardStats()
};

const handleGroupByChange = () => {
  fetchCardStats()
};

const refreshData = () => {
  fetchCardInfo()
  fetchCardStats()
};

const goBack = () => {
  router.push('/kuaishouCard')
};

const formatNumber = (num) => {
  if (!num) return '0'
  return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
};

const formatDate = (date) => {
  if (!date) return ''
  const d = new Date(date)
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
};

// 'default' 不在 el-tag 的合法 type 里（primary/success/info/warning/danger），会触发 Vue prop
// 校验告警并回落到 default='primary'。未识别动作用中性的 'info'。
const getActionType = (action) => {
  const typeMap = {
    'view': 'info'
  }
  return typeMap[action] || 'info'
};

const getActionText = (action) => {
  const typeMap = {
    'view': '浏览'
  }
  return typeMap[action] || '未知'
};

onMounted(async () => {
  const endDate = new Date();
  const startDate = new Date()
  startDate.setDate(startDate.getDate() - 30)
  dateRange.value = [startDate, endDate]

  window.addEventListener('resize', handleResize)
  
  await fetchCardInfo();
  await fetchCardStats()
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  if (visitChartInstance) {
    visitChartInstance.dispose()
    visitChartInstance = null
  }
})
</script>

<style scoped>
.kuaishou-card-stats-container {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}

.card-info-section {
  margin-bottom: 30px;
}

.stats-alert {
  margin-bottom: 16px;
}

.stats-overview {
  margin-bottom: 30px;
}

.stats-card {
  text-align: center;
}

.stats-item {
  padding: 20px;
}

.stats-value {
  font-size: 24px;
  font-weight: 600;
  color: #4F46E5;
  margin-bottom: 8px;
}

.stats-label {
  font-size: 14px;
  color: #909399;
}

.charts-section {
  margin-bottom: 30px;
}

.chart-container {
  height: 400px;
}

.bottom-section {
  margin-bottom: 30px;
}

.pagination-container {
  display: flex;
  justify-content: center;
  margin-top: 20px;
}

.el-tag {
  margin-right: 5px;
}

.el-tag:last-child {
  margin-right: 0;
}

/* 响应式设计 */
@media (max-width: 768px) {
  .card-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
  }
  
  .header-actions {
    width: 100%;
    flex-wrap: wrap;
  }
}
</style>
import { driver as Driver } from 'driver.js'
import 'driver.js/dist/driver.css'
import { useUserStore } from '@/stores/user'
import { useI18n } from 'vue-i18n'

const ONBOARDING_COMPLETED_KEY = 'hivemtk_onboarding_completed'
const ONBOARDING_STEP_KEY = 'hivemtk_onboarding_step'
const ONBOARDING_TOUR_TYPE_KEY = 'hivemtk_onboarding_tour_type'

type TourType = 'global' | 'module' | 'page'

interface OnboardingStep {
  element: string
  popover: {
    title: string
    description: string
    side?: 'top' | 'bottom' | 'left' | 'right'
    align?: 'start' | 'center' | 'end'
  }
  onHighlightStarted?: () => void
  onHighlighted?: () => void
  onDeselected?: () => void
}

interface ModuleTourConfig {
  key: string
  title: string
  route: string
  steps: Array<{
    element: string
    titleKey: string
    descriptionKey: string
    side?: 'top' | 'bottom' | 'left' | 'right'
    align?: 'start' | 'center' | 'end'
  }>
}

let driverInstance: ReturnType<typeof Driver> | null = null
let currentTourType: TourType = 'global'

function getGlobalNavSteps(t: (key: string) => string): OnboardingStep[] {
  return [
    {
      element: '.brand-mark',
      popover: {
        title: t('onboarding.global.step1.title'),
        description: t('onboarding.global.step1.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.top-menu',
      popover: {
        title: t('onboarding.global.step2.title'),
        description: t('onboarding.global.step2.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.app-aside',
      popover: {
        title: t('onboarding.global.step3.title'),
        description: t('onboarding.global.step3.description'),
        side: 'right',
        align: 'center',
      },
    },
    {
      element: '.notif-bell',
      popover: {
        title: t('onboarding.global.step4.title'),
        description: t('onboarding.global.step4.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.user-area',
      popover: {
        title: t('onboarding.global.step5.title'),
        description: t('onboarding.global.step5.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.app-main',
      popover: {
        title: t('onboarding.global.step6.title'),
        description: t('onboarding.global.step6.description'),
        side: 'top',
        align: 'center',
      },
    },
  ]
}

function getModuleTourSteps(t: (key: string) => string, moduleKey: string): OnboardingStep[] {
  const moduleTours: Record<string, ModuleTourConfig> = {
    messageHub: {
      key: 'messageHub',
      title: t('menu.messageHub'),
      route: '/messageHub/list',
      steps: [
        { element: '.mh-stats-cards', titleKey: 'onboarding.module.messageHub.step1.title', descriptionKey: 'onboarding.module.messageHub.step1.description', side: 'bottom' },
        { element: '.mh-channel-tabs', titleKey: 'onboarding.module.messageHub.step2.title', descriptionKey: 'onboarding.module.messageHub.step2.description', side: 'bottom' },
        { element: '.mh-message-table', titleKey: 'onboarding.module.messageHub.step3.title', descriptionKey: 'onboarding.module.messageHub.step3.description', side: 'top' },
      ],
    },
    inbox: {
      key: 'inbox',
      title: t('menu.inbox'),
      route: '/inbox/list',
      steps: [
        { element: '.inbox-filter-bar', titleKey: 'onboarding.module.inbox.step1.title', descriptionKey: 'onboarding.module.inbox.step1.description', side: 'bottom' },
        { element: '.inbox-session-list', titleKey: 'onboarding.module.inbox.step2.title', descriptionKey: 'onboarding.module.inbox.step2.description', side: 'right' },
        { element: '.inbox-chat-panel', titleKey: 'onboarding.module.inbox.step3.title', descriptionKey: 'onboarding.module.inbox.step3.description', side: 'left' },
        { element: '.inbox-agent-assign', titleKey: 'onboarding.module.inbox.step4.title', descriptionKey: 'onboarding.module.inbox.step4.description', side: 'bottom' },
      ],
    },
    customer: {
      key: 'customer',
      title: t('menu.customer'),
      route: '/customer360/list',
      steps: [
        { element: '.c360-search', titleKey: 'onboarding.module.customer.step1.title', descriptionKey: 'onboarding.module.customer.step1.description', side: 'bottom' },
        { element: '.c360-profile-card', titleKey: 'onboarding.module.customer.step2.title', descriptionKey: 'onboarding.module.customer.step2.description', side: 'right' },
        { element: '.c360-tabs', titleKey: 'onboarding.module.customer.step3.title', descriptionKey: 'onboarding.module.customer.step3.description', side: 'bottom' },
        { element: '.c360-event-timeline', titleKey: 'onboarding.module.customer.step4.title', descriptionKey: 'onboarding.module.customer.step4.description', side: 'top' },
      ],
    },
    aiAgent: {
      key: 'aiAgent',
      title: t('menu.aiAgent'),
      route: '/aiAgent/list',
      steps: [
        { element: '.agent-list-table', titleKey: 'onboarding.module.aiAgent.step1.title', descriptionKey: 'onboarding.module.aiAgent.step1.description', side: 'bottom' },
        { element: '.agent-create-btn', titleKey: 'onboarding.module.aiAgent.step2.title', descriptionKey: 'onboarding.module.aiAgent.step2.description', side: 'bottom' },
        { element: '.agent-type-tabs', titleKey: 'onboarding.module.aiAgent.step3.title', descriptionKey: 'onboarding.module.aiAgent.step3.description', side: 'top' },
      ],
    },
    agentActive: {
      key: 'agentActive',
      title: t('menu.agentActive'),
      route: '/reachPipeline/list',
      steps: [
        { element: '.pipeline-canvas', titleKey: 'onboarding.module.agentActive.step1.title', descriptionKey: 'onboarding.module.agentActive.step1.description', side: 'bottom' },
        { element: '.pipeline-node-palette', titleKey: 'onboarding.module.agentActive.step2.title', descriptionKey: 'onboarding.module.agentActive.step2.description', side: 'left' },
        { element: '.pipeline-toolbar', titleKey: 'onboarding.module.agentActive.step3.title', descriptionKey: 'onboarding.module.agentActive.step3.description', side: 'bottom' },
      ],
    },
    agentSupport: {
      key: 'agentSupport',
      title: t('menu.agentSupport'),
      route: '/sopAgent/list',
      steps: [
        { element: '.sop-list', titleKey: 'onboarding.module.agentSupport.step1.title', descriptionKey: 'onboarding.module.agentSupport.step1.description', side: 'bottom' },
        { element: '.script-template-list', titleKey: 'onboarding.module.agentSupport.step2.title', descriptionKey: 'onboarding.module.agentSupport.step2.description', side: 'bottom' },
        { element: '.faq-kb-list', titleKey: 'onboarding.module.agentSupport.step3.title', descriptionKey: 'onboarding.module.agentSupport.step3.description', side: 'top' },
      ],
    },
    agentWorkbench: {
      key: 'agentWorkbench',
      title: t('menu.agentWorkbench'),
      route: '/customerService/agentStatus',
      steps: [
        { element: '.agent-status-board', titleKey: 'onboarding.module.agentWorkbench.step1.title', descriptionKey: 'onboarding.module.agentWorkbench.step1.description', side: 'bottom' },
        { element: '.quick-reply-panel', titleKey: 'onboarding.module.agentWorkbench.step2.title', descriptionKey: 'onboarding.module.agentWorkbench.step2.description', side: 'right' },
        { element: '.ai-suggestion-panel', titleKey: 'onboarding.module.agentWorkbench.step3.title', descriptionKey: 'onboarding.module.agentWorkbench.step3.description', side: 'left' },
      ],
    },
    reach: {
      key: 'reach',
      title: t('menu.reach'),
      route: '/email/list',
      steps: [
        { element: '.reach-channel-tabs', titleKey: 'onboarding.module.reach.step1.title', descriptionKey: 'onboarding.module.reach.step1.description', side: 'bottom' },
        { element: '.reach-draft-list', titleKey: 'onboarding.module.reach.step2.title', descriptionKey: 'onboarding.module.reach.step2.description', side: 'bottom' },
        { element: '.reach-job-config', titleKey: 'onboarding.module.reach.step3.title', descriptionKey: 'onboarding.module.reach.step3.description', side: 'top' },
      ],
    },
    knowledge: {
      key: 'knowledge',
      title: t('menu.knowledge'),
      route: '/knowledge/management',
      steps: [
        { element: '.kb-doc-tree', titleKey: 'onboarding.module.knowledge.step1.title', descriptionKey: 'onboarding.module.knowledge.step1.description', side: 'right' },
        { element: '.kb-editor', titleKey: 'onboarding.module.knowledge.step2.title', descriptionKey: 'onboarding.module.knowledge.step2.description', side: 'left' },
        { element: '.kb-playground', titleKey: 'onboarding.module.knowledge.step3.title', descriptionKey: 'onboarding.module.knowledge.step3.description', side: 'bottom' },
      ],
    },
    analytics: {
      key: 'analytics',
      title: t('menu.analytics'),
      route: '/dashboardScreen/list',
      steps: [
        { element: '.dashboard-canvas', titleKey: 'onboarding.module.analytics.step1.title', descriptionKey: 'onboarding.module.analytics.step1.description', side: 'bottom' },
        { element: '.dashboard-widget-panel', titleKey: 'onboarding.module.analytics.step2.title', descriptionKey: 'onboarding.module.analytics.step2.description', side: 'right' },
        { element: '.funnel-config', titleKey: 'onboarding.module.analytics.step3.title', descriptionKey: 'onboarding.module.analytics.step3.description', side: 'bottom' },
      ],
    },
    geoTools: {
      key: 'geoTools',
      title: t('menu.geoTools'),
      route: '/geo/keywordMining',
      steps: [
        { element: '.geo-module-tabs', titleKey: 'onboarding.module.geoTools.step1.title', descriptionKey: 'onboarding.module.geoTools.step1.description', side: 'bottom' },
        { element: '.geo-keyword-table', titleKey: 'onboarding.module.geoTools.step2.title', descriptionKey: 'onboarding.module.geoTools.step2.description', side: 'bottom' },
        { element: '.geo-content-editor', titleKey: 'onboarding.module.geoTools.step3.title', descriptionKey: 'onboarding.module.geoTools.step3.description', side: 'top' },
      ],
    },
    system: {
      key: 'system',
      title: t('menu.system'),
      route: '/system/config',
      steps: [
        { element: '.sys-nav-tabs', titleKey: 'onboarding.module.system.step1.title', descriptionKey: 'onboarding.module.system.step1.description', side: 'bottom' },
        { element: '.sys-user-table', titleKey: 'onboarding.module.system.step2.title', descriptionKey: 'onboarding.module.system.step2.description', side: 'bottom' },
        { element: '.sys-role-permission', titleKey: 'onboarding.module.system.step3.title', descriptionKey: 'onboarding.module.system.step3.description', side: 'top' },
      ],
    },
    assetBundle: {
      key: 'assetBundle',
      title: t('menu.assetBundle'),
      route: '/assetBundle/list',
      steps: [
        { element: '.bundle-list', titleKey: 'onboarding.module.assetBundle.step1.title', descriptionKey: 'onboarding.module.assetBundle.step1.description', side: 'bottom' },
        { element: '.bundle-playground', titleKey: 'onboarding.module.assetBundle.step2.title', descriptionKey: 'onboarding.module.assetBundle.step2.description', side: 'bottom' },
        { element: '.bundle-merchant', titleKey: 'onboarding.module.assetBundle.step3.title', descriptionKey: 'onboarding.module.assetBundle.step3.description', side: 'top' },
      ],
    },
    assetMarket: {
      key: 'assetMarket',
      title: t('menu.assetMarket'),
      route: '/assetMarket/market',
      steps: [
        { element: '.market-filter', titleKey: 'onboarding.module.assetMarket.step1.title', descriptionKey: 'onboarding.module.assetMarket.step1.description', side: 'bottom' },
        { element: '.market-grid', titleKey: 'onboarding.module.assetMarket.step2.title', descriptionKey: 'onboarding.module.assetMarket.step2.description', side: 'bottom' },
        { element: '.my-assets-tab', titleKey: 'onboarding.module.assetMarket.step3.title', descriptionKey: 'onboarding.module.assetMarket.step3.description', side: 'top' },
      ],
    },
  }

  const tour = moduleTours[moduleKey]
  if (!tour) return []

  return tour.steps.map(s => ({
    element: s.element,
    popover: {
      title: t(s.titleKey),
      description: t(s.descriptionKey),
      side: s.side || 'bottom',
      align: s.align || 'center',
    },
  }))
}

function getPageTourSteps(t: (key: string) => string, pageKey: string): OnboardingStep[] {
  const pageTours: Record<string, OnboardingStep[]> = {
    'messageHub-list': [
      { element: '.mh-overview-cards', popover: { title: t('onboarding.page.messageHub.step1.title'), description: t('onboarding.page.messageHub.step1.description'), side: 'bottom' } },
      { element: '.mh-channel-filter', popover: { title: t('onboarding.page.messageHub.step2.title'), description: t('onboarding.page.messageHub.step2.description'), side: 'bottom' } },
      { element: '.mh-message-table', popover: { title: t('onboarding.page.messageHub.step3.title'), description: t('onboarding.page.messageHub.step3.description'), side: 'top' } },
      { element: '.mh-pagination', popover: { title: t('onboarding.page.messageHub.step4.title'), description: t('onboarding.page.messageHub.step4.description'), side: 'bottom' } },
    ],
    'inbox-list': [
      { element: '.inbox-filter-toolbar', popover: { title: t('onboarding.page.inbox.step1.title'), description: t('onboarding.page.inbox.step1.description'), side: 'bottom' } },
      { element: '.inbox-session-list', popover: { title: t('onboarding.page.inbox.step2.title'), description: t('onboarding.page.inbox.step2.description'), side: 'right' } },
      { element: '.inbox-chat-area', popover: { title: t('onboarding.page.inbox.step3.title'), description: t('onboarding.page.inbox.step3.description'), side: 'left' } },
      { element: '.inbox-assign-dropdown', popover: { title: t('onboarding.page.inbox.step4.title'), description: t('onboarding.page.inbox.step4.description'), side: 'bottom' } },
    ],
    'customer360-list': [
      { element: '.c360-search-box', popover: { title: t('onboarding.page.customer.step1.title'), description: t('onboarding.page.customer.step1.description'), side: 'bottom' } },
      { element: '.c360-customer-card', popover: { title: t('onboarding.page.customer.step2.title'), description: t('onboarding.page.customer.step2.description'), side: 'right' } },
      { element: '.c360-detail-tabs', popover: { title: t('onboarding.page.customer.step3.title'), description: t('onboarding.page.customer.step3.description'), side: 'bottom' } },
      { element: '.c360-tag-editor', popover: { title: t('onboarding.page.customer.step4.title'), description: t('onboarding.page.customer.step4.description'), side: 'left' } },
    ],
    'aiAgent-list': [
      { element: '.agent-list-toolbar', popover: { title: t('onboarding.page.aiAgent.step1.title'), description: t('onboarding.page.aiAgent.step1.description'), side: 'bottom' } },
      { element: '.agent-table', popover: { title: t('onboarding.page.aiAgent.step2.title'), description: t('onboarding.page.aiAgent.step2.description'), side: 'bottom' } },
      { element: '.agent-create-dialog', popover: { title: t('onboarding.page.aiAgent.step3.title'), description: t('onboarding.page.aiAgent.step3.description'), side: 'left' } },
    ],
    'reachPipeline-list': [
      { element: '.pipeline-canvas', popover: { title: t('onboarding.page.agentActive.step1.title'), description: t('onboarding.page.agentActive.step1.description'), side: 'bottom' } },
      { element: '.pipeline-node-panel', popover: { title: t('onboarding.page.agentActive.step2.title'), description: t('onboarding.page.agentActive.step2.description'), side: 'left' } },
      { element: '.pipeline-toolbar', popover: { title: t('onboarding.page.agentActive.step3.title'), description: t('onboarding.page.agentActive.step3.description'), side: 'bottom' } },
      { element: '.pipeline-minimap', popover: { title: t('onboarding.page.agentActive.step4.title'), description: t('onboarding.page.agentActive.step4.description'), side: 'right' } },
    ],
    'dashboardScreen-list': [
      { element: '.ds-builder-canvas', popover: { title: t('onboarding.page.analytics.step1.title'), description: t('onboarding.page.analytics.step1.description'), side: 'bottom' } },
      { element: '.ds-widget-library', popover: { title: t('onboarding.page.analytics.step2.title'), description: t('onboarding.page.analytics.step2.description'), side: 'right' } },
      { element: '.ds-data-source', popover: { title: t('onboarding.page.analytics.step3.title'), description: t('onboarding.page.analytics.step3.description'), side: 'left' } },
    ],
  }

  return pageTours[pageKey] || []
}

export function useOnboarding() {
  const userStore = useUserStore()
  const { t } = useI18n()

  function isCompleted(tourType: TourType = 'global'): boolean {
    return localStorage.getItem(`${ONBOARDING_COMPLETED_KEY}_${tourType}`) === 'true'
  }

  function getCurrentStep(): number {
    return Number(localStorage.getItem(ONBOARDING_STEP_KEY) || '0')
  }

  function setCurrentStep(step: number): void {
    localStorage.setItem(ONBOARDING_STEP_KEY, String(step))
  }

  function markCompleted(tourType: TourType = 'global'): void {
    localStorage.setItem(`${ONBOARDING_COMPLETED_KEY}_${tourType}`, 'true')
    localStorage.removeItem(ONBOARDING_STEP_KEY)
  }

  function reset(tourType: TourType = 'global'): void {
    localStorage.removeItem(`${ONBOARDING_COMPLETED_KEY}_${tourType}`)
    localStorage.removeItem(ONBOARDING_STEP_KEY)
    localStorage.removeItem(ONBOARDING_TOUR_TYPE_KEY)
  }

  function createDriver(steps: OnboardingStep[], onComplete?: () => void): ReturnType<typeof Driver> {
    if (driverInstance) {
      driverInstance.destroy()
    }

    driverInstance = Driver({
      animate: true,
      allowClose: true,
      overlayOpacity: 0.75,
      overlayColor: '#0f172a',
      stageRadius: 8,
      stagePadding: 8,
      nextBtnText: t('onboarding.next'),
      prevBtnText: t('onboarding.prev'),
      doneBtnText: t('onboarding.done'),
      closeBtnText: t('onboarding.skip'),
      keyboardControl: true,
      steps,
      onHighlightStarted: (step) => {
        setCurrentStep(steps.indexOf(step) + 1)
      },
      onHighlighted: (step) => {
        step.onHighlighted?.()
      },
      onDeselected: (step) => {
        step.onDeselected?.()
      },
      onDestroyed: () => {
        driverInstance = null
      },
    })

    // 监听完成事件（最后一步点击"完成"按钮）
    if (onComplete) {
      const originalDoneClick = steps[steps.length - 1]?.popover?.onNextClick
      steps[steps.length - 1].popover.onNextClick = () => {
        onComplete()
        originalDoneClick?.()
      }
    }

    return driverInstance
  }

  function startGlobalTour(): void {
    if (!userStore.isLoggedIn) return
    currentTourType = 'global'
    const steps = getGlobalNavSteps(t)
    const driver = createDriver(steps, () => markCompleted('global'))
    driver.drive()
  }

  function startModuleTour(moduleKey: string): void {
    if (!userStore.isLoggedIn) return
    currentTourType = 'module'
    const steps = getModuleTourSteps(t, moduleKey)
    if (steps.length === 0) {
      console.warn(`[Onboarding] No tour config for module: ${moduleKey}`)
      return
    }
    const driver = createDriver(steps, () => markCompleted('module'))
    driver.drive()
  }

  function startPageTour(pageKey: string): void {
    if (!userStore.isLoggedIn) return
    currentTourType = 'page'
    const steps = getPageTourSteps(t, pageKey)
    if (steps.length === 0) {
      console.warn(`[Onboarding] No tour config for page: ${pageKey}`)
      return
    }
    const driver = createDriver(steps, () => markCompleted('page'))
    driver.drive()
  }

  function restart(tourType: TourType = 'global'): void {
    reset(tourType)
    if (tourType === 'global') startGlobalTour()
  }

  function destroy(): void {
    if (driverInstance) {
      driverInstance.destroy()
      driverInstance = null
    }
  }

  function shouldAutoStart(tourType: TourType = 'global'): boolean {
    return userStore.isLoggedIn && !isCompleted(tourType)
  }

  function getAvailableModules(): string[] {
    return [
      'messageHub', 'inbox', 'customer', 'aiAgent',
      'agentActive', 'agentSupport', 'agentWorkbench',
      'reach', 'knowledge', 'analytics', 'geoTools',
      'system', 'assetBundle', 'assetMarket'
    ]
  }

  return {
    startGlobalTour,
    startModuleTour,
    startPageTour,
    restart,
    destroy,
    isCompleted,
    getCurrentStep,
    shouldAutoStart,
    markCompleted,
    getAvailableModules,
  }
}

export function useOnboardingAutoStart() {
  const { shouldAutoStart, startGlobalTour, markCompleted } = useOnboarding()

  function tryAutoStart(): void {
    if (shouldAutoStart('global')) {
      setTimeout(() => {
        startGlobalTour()
      }, 1000)
    }
  }

  function onTourComplete(tourType: TourType = 'global'): void {
    markCompleted(tourType)
  }

  return {
    tryAutoStart,
    onTourComplete,
  }
}

export function useModuleOnboarding(moduleKey: string) {
  const { startModuleTour, isCompleted, markCompleted } = useOnboarding()

  function tryStart(): void {
    if (!isCompleted('module')) {
      startModuleTour(moduleKey)
    }
  }

  function onComplete(): void {
    markCompleted('module')
  }

  return { tryStart, onComplete }
}

export function usePageOnboarding(pageKey: string) {
  const { startPageTour, isCompleted, markCompleted } = useOnboarding()

  function tryStart(): void {
    if (!isCompleted('page')) {
      startPageTour(pageKey)
    }
  }

  function onComplete(): void {
    markCompleted('page')
  }

  return { tryStart, onComplete }
}
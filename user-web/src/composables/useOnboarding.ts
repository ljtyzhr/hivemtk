import { driver as Driver } from 'driver.js'
import 'driver.js/dist/driver.css'
import { useUserStore } from '@/stores/user'
import { useI18n } from 'vue-i18n'

const ONBOARDING_COMPLETED_KEY = 'hivemtk_onboarding_completed'
const ONBOARDING_STEP_KEY = 'hivemtk_onboarding_step'

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

let driverInstance: ReturnType<typeof Driver> | null = null

function getSteps(t: (key: string) => string): OnboardingStep[] {
  return [
    {
      element: '.brand-mark',
      popover: {
        title: t('onboarding.step1.title'),
        description: t('onboarding.step1.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.top-menu',
      popover: {
        title: t('onboarding.step2.title'),
        description: t('onboarding.step2.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.app-aside',
      popover: {
        title: t('onboarding.step3.title'),
        description: t('onboarding.step3.description'),
        side: 'right',
        align: 'center',
      },
    },
    {
      element: '.notif-bell',
      popover: {
        title: t('onboarding.step4.title'),
        description: t('onboarding.step4.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.user-area',
      popover: {
        title: t('onboarding.step5.title'),
        description: t('onboarding.step5.description'),
        side: 'bottom',
        align: 'center',
      },
    },
    {
      element: '.app-main',
      popover: {
        title: t('onboarding.step6.title'),
        description: t('onboarding.step6.description'),
        side: 'top',
        align: 'center',
      },
    },
  ]
}

export function useOnboarding() {
  const userStore = useUserStore()
  const { t } = useI18n()

  function isCompleted(): boolean {
    return localStorage.getItem(ONBOARDING_COMPLETED_KEY) === 'true'
  }

  function getCurrentStep(): number {
    return Number(localStorage.getItem(ONBOARDING_STEP_KEY) || '0')
  }

  function setCurrentStep(step: number): void {
    localStorage.setItem(ONBOARDING_STEP_KEY, String(step))
  }

  function markCompleted(): void {
    localStorage.setItem(ONBOARDING_COMPLETED_KEY, 'true')
    localStorage.removeItem(ONBOARDING_STEP_KEY)
  }

  function reset(): void {
    localStorage.removeItem(ONBOARDING_COMPLETED_KEY)
    localStorage.removeItem(ONBOARDING_STEP_KEY)
  }

  function createDriver(): ReturnType<typeof Driver> {
    if (driverInstance) {
      driverInstance.destroy()
    }

    const steps = getSteps(t)

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
      onReset: () => {
        reset()
      },
      onDestroyed: () => {
        driverInstance = null
      },
    })

    return driverInstance
  }

  function start(): void {
    if (!userStore.isLoggedIn) return

    const driver = createDriver()
    driver.drive()
  }

  function restart(): void {
    reset()
    start()
  }

  function destroy(): void {
    if (driverInstance) {
      driverInstance.destroy()
      driverInstance = null
    }
  }

  function shouldAutoStart(): boolean {
    return userStore.isLoggedIn && !isCompleted()
  }

  return {
    start,
    restart,
    destroy,
    isCompleted,
    getCurrentStep,
    shouldAutoStart,
    markCompleted,
  }
}

export function useOnboardingAutoStart() {
  const { shouldAutoStart, start, markCompleted } = useOnboarding()

  function tryAutoStart(): void {
    if (shouldAutoStart()) {
      setTimeout(() => {
        start()
      }, 1000)
    }
  }

  function onTourComplete(): void {
    markCompleted()
  }

  return {
    tryAutoStart,
    onTourComplete,
  }
}
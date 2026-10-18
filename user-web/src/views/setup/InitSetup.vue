<template>
  <div class="init-setup-container">
    <div class="setup-header">
      <h1>{{ $t('setup.wizardTitle') }}</h1>
      <p>{{ $t('setup.wizardIntro') }}</p>
    </div>

    <div class="progress-container">
      <el-steps
        :active="currentStep"
        finish-status="success"
        align-center
      >
        <el-step
          :title="$t('setup.readAgreement')"
          description="阅读使用声明"
        />
        <el-step
          :title="$t('setup.stepAdmin')"
          description="配置管理员账号"
        />
        <el-step
          :title="$t('setup.finish')"
          description="进入登录"
        />
      </el-steps>
    </div>

    <div class="step-content">
      <div
        v-if="currentStep === 0"
        class="step-item"
      >
        <h2>{{ $t('setup.legalTitle') }}</h2>
        <div class="agreement-content">
          <p>{{ $t('setup.legalOpenSource') }}</p>
          <ul>
            <li>{{ $t('setup.legalLawfulUse') }}</li>
            <li>{{ $t('setup.legalLiability') }}</li>
            <li>{{ $t('setup.legalLocalDefault') }}</li>
          </ul>
          <el-checkbox
            v-model="agreed"
            style="margin-top: 20px"
          >
            {{ $t('setup.legalAgree') }}
          </el-checkbox>
        </div>
        <div class="step-actions">
          <el-button
            type="primary"
            :disabled="!agreed"
            @click="nextStep"
          >
            {{ $t('setup.next') }}
          </el-button>
        </div>
      </div>

      
      <div
        v-if="currentStep === 1"
        class="step-item"
      >
        <h2>{{ $t('setup.stepAdminFull') }}</h2>
        <p class="step-tip">
          {{ $t('setup.adminIntro') }}
        </p>
        <el-form
          ref="adminFormRef"
          :model="adminForm"
          :rules="adminRules"
          label-width="110px"
        >
          <el-form-item
            :label="$t('setup.adminAccount')"
            prop="username"
          >
            <el-input
              v-model="adminForm.username"
              :placeholder="$t('setup.adminUsernameHint')"
              size="large"
              clearable
            />
          </el-form-item>
          <el-form-item
            :label="$t('setup.adminPassword')"
            prop="password"
          >
            <el-input
              v-model="adminForm.password"
              type="password"
              show-password
              :placeholder="$t('setup.adminPasswordHint2')"
              size="large"
            />
            <div class="field-hint">
              {{ $t('setup.adminPasswordHint') }}
            </div>
          </el-form-item>
          <el-form-item
            :label="$t('setup.confirmPassword')"
            prop="confirmPassword"
          >
            <el-input
              v-model="adminForm.confirmPassword"
              type="password"
              show-password
              :placeholder="$t('setup.errPasswordConfirm')"
              size="large"
            />
          </el-form-item>

          <!-- 开户登记（监管必需项）。
               这一步原先把公司/负责人/电话/邮箱全部标为「选填」——
               而离线部署的用户**只走这一个入口**（他们不会去注册页），
               于是需求「注意离线独立部署情况，也要收集信息」对整个离线人群失效。
               现在四项必填：平台端持有的是一个联系得到的经营主体，而不是空白。 -->
          <el-divider content-position="left">
            {{ $t('setup.regTitle') }}
          </el-divider>

          <el-form-item
            :label="$t('setup.regCompany')"
            prop="company_name"
          >
            <el-input
              v-model="adminForm.company_name"
              :placeholder="$t('setup.regCompanyHint')"
              size="large"
            />
          </el-form-item>
          <el-form-item
            :label="$t('setup.regOwner')"
            prop="real_name"
          >
            <el-input
              v-model="adminForm.real_name"
              :placeholder="$t('setup.regOwnerName')"
              size="large"
            />
          </el-form-item>
          <el-form-item
            :label="$t('setup.phone')"
            prop="contact_phone"
          >
            <el-input
              v-model="adminForm.contact_phone"
              :placeholder="$t('setup.regContactHint')"
              size="large"
            />
          </el-form-item>
          <el-form-item
            :label="$t('setup.email')"
            prop="email"
          >
            <el-input
              v-model="adminForm.email"
              :placeholder="$t('setup.regNoticeHint')"
              size="large"
            />
          </el-form-item>
          <el-form-item
            :label="$t('setup.regLicenseCode')"
            prop="license_no"
          >
            <el-input
              v-model="adminForm.license_no"
              :placeholder="$t('setup.regLicenseHint')"
              size="large"
              clearable
            />
          </el-form-item>
          <el-form-item>
            <el-checkbox v-model="adminForm.regulatory_ack">
              {{ $t('setup.regAgree') }}
            </el-checkbox>
          </el-form-item>
        </el-form>
        <div class="step-actions">
          <el-button @click="prevStep">
            {{ $t('setup.prev') }}
          </el-button>
          <el-button
            type="primary"
            :loading="creatingAdmin"
            @click="handleCreateAdmin"
          >
            {{ $t('setup.createAdminCta') }}
          </el-button>
        </div>
      </div>

      
      <div
        v-if="currentStep === 2"
        class="step-item"
      >
        <el-result
          icon="success"
          :title="$t('setup.initDone')"
          :sub-title="$t('setup.adminCreated')"
        >
          <template #extra>
            <el-button
              type="primary"
              size="large"
              @click="goToLogin"
            >
              {{ $t('setup.goLogin') }}
            </el-button>
          </template>
        </el-result>
        <el-descriptions
          :column="1"
          border
          style="margin-top: 20px"
        >
          <el-descriptions-item :label="$t('setup.regCompany')">
            {{ adminForm.company_name }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('setup.regOwner')">
            {{ adminForm.real_name }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('setup.regMethod')">
            {{ $t('setup.regOfflineMode') }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('setup.adminAccount')">
            {{ adminForm.username }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('setup.notice')">
            <el-text type="info">
              {{ $t('setup.resetHint') }}
            </el-text>
          </el-descriptions-item>
        </el-descriptions>
      </div>
    </div>
  </div>
</template>

<script setup>
import i18n from '@/i18n'
// 脚本级翻译函数：裸 t( 形态对 i18n 门可见（点分键计数），运行时语义不变
const t = i18n.global.t.bind(i18n.global)

import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { http } from '@/utils/request'
import { markInitializationComplete } from '@/utils/initHelper'

const router = useRouter()

const currentStep = ref(0)
const agreed = ref(false)

const adminFormRef = ref(null);
const adminForm = reactive({
  username: '',
  password: '',
  confirmPassword: '',
  // 开户登记四项：离线部署唯一的合规留痕入口，必填
  company_name: '',
  real_name: '',
  contact_phone: '',
  email: '',
  license_no: '',
  regulatory_ack: false
})

// 协议版本与后端 serviceCurrentAgreementVersion / 平台端 CurrentAgreementVersion 同值。
// 它是「用户同意的是哪一版」的唯一凭据，改告知文本时三处一起升版本。
const REGISTRATION_AGREEMENT_VERSION = '2026-10-17.v1'
const adminRules = {
  username: [
    { required: true, message: t('setup.errUsernameRequired'), trigger: 'blur' },
    {
      pattern: /^[A-Za-z0-9_]{3,20}$/,
      message: t('setup.errUsernamePattern'),
      trigger: 'blur'
    }
  ],
  password: [
    { required: true, message: t('setup.errPasswordRequired'), trigger: 'blur' },
    { min: 8, max: 64, message: t('setup.errPasswordLen'), trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (!value) return callback()
        if (!/[a-z]/.test(value)) return callback(new Error('密码必须包含小写字母'))
        if (!/[A-Z]/.test(value)) return callback(new Error('密码必须包含大写字母'))
        if (!/\d/.test(value)) return callback(new Error('密码必须包含数字'))
        callback()
      },
      trigger: 'blur'
    }
  ],
  confirmPassword: [
    { required: true, message: t('setup.errPasswordConfirm'), trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (value !== adminForm.password) {
          return callback(new Error('两次输入的密码不一致'))
        }
        callback()
      },
      trigger: 'blur'
    }
  ],
  email: [
    { required: true, message: t('setup.errEmailRequired'), trigger: 'blur' },
    { type: 'email', message: t('setup.errEmailInvalid'), trigger: 'blur' }
  ],
  company_name: [
    { required: true, message: t('setup.errCompanyRequired'), trigger: 'blur' }
  ],
  real_name: [
    { required: true, message: t('setup.errOwnerRequired'), trigger: 'blur' }
  ],
  contact_phone: [
    { required: true, message: t('setup.errPhoneRequired'), trigger: 'blur' },
    { pattern: /^[0-9+\-() ]{6,32}$/, message: t('setup.errPhoneFormat'), trigger: 'blur' }
  ]
}

const creatingAdmin = ref(false)

const nextStep = () => {
  if (currentStep.value < 2) currentStep.value++
}
const prevStep = () => {
  if (currentStep.value > 0) currentStep.value--
}

const handleCreateAdmin = async () => {
  if (!adminFormRef.value) return
  try {
    await adminFormRef.value.validate()
    creatingAdmin.value = true
    if (!adminForm.regulatory_ack) {
      ElMessage.warning(t('setup.errAgreeRequired'))
      return
    }
    const resp = await http.post('/api/system/init-admin', {
      username: adminForm.username,
      password: adminForm.password,
      company_name: adminForm.company_name,
      // 后端 system_init.go 的 InitAdminInput 绑定的是 real_name（binding:"required"），
      // 键名写成 legal_person 会让每次提交都被 gin 参数校验打回 400「无效的请求参数」。
      real_name: adminForm.real_name,
      contact_phone: adminForm.contact_phone,
      email: adminForm.email,
      license_no: adminForm.license_no,
      regulatory_ack: true,
      agreement_version: REGISTRATION_AGREEMENT_VERSION
    }, { _silent: true });
    if (resp) {
      ElMessage.success(resp.message || '超管创建成功')
      await markInitializationComplete();
      nextStep()
    }
  } catch (error) {
    console.error('创建超管失败:', error)
    ElMessage.error(error?.response?.data?.message || error?.message || '超管创建失败，请检查后重试')
  } finally {
    creatingAdmin.value = false
  }
}

const goToLogin = () => {
  router.push('/login')
}

onMounted(async () => {
  try {
    const status = await http.get('/api/system/init-status')
    if (status && status.state) {
      switch (status.state) {
        case 'HAS_ADMIN':
        case 'INITIALIZED':
          await markInitializationComplete()
          ElMessage.info(t('setup.errAlreadyInit'))
          router.push('/login')
          break
        default:
          currentStep.value = 0
      }
    }
  } catch (e) {
    console.warn('查询初始化状态失败:', e)
  }
})
</script>

<style scoped>
.init-setup-container {
  max-width: 900px;
  margin: 0 auto;
  padding: 40px 20px;
}

.setup-header {
  text-align: center;
  margin-bottom: 40px;
}

.setup-header h1 {
  font-size: 28px;
  color: #303133;
  margin-bottom: 10px;
}

.setup-header p {
  font-size: 16px;
  color: #909399;
}

.progress-container {
  margin-bottom: 40px;
}

.step-content {
  min-height: 400px;
}

.step-item {
  background-color: #fff;
  padding: 30px;
  border-radius: 8px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
}

.step-item h2 {
  font-size: 20px;
  color: #303133;
  margin-bottom: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid #ebeef5;
}

.step-tip {
  color: #909399;
  font-size: 14px;
  margin-bottom: 24px;
}

.agreement-content ul {
  padding-left: 20px;
  margin-bottom: 20px;
}

.agreement-content li {
  margin-bottom: 8px;
  line-height: 1.6;
  color: #606266;
}

.step-actions {
  margin-top: 30px;
  display: flex;
  gap: 10px;
  justify-content: flex-end;
}

.field-hint {
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: #909399;
}
</style>

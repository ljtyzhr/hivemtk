<template>
  <div class="register-page">
    <el-card
      class="card"
      shadow="always"
    >
      <template #header>
        <div class="head">
          <h2>商户注册 / 开户</h2>
          <p class="sub">
            注册成功后即可开始使用；如需订阅功能模块，请在「功能模块市场」开通
          </p>
        </div>
      </template>

      <el-steps
        :active="step"
        align-center
        finish-status="success"
        class="mb-16"
      >
        <el-step title="账户信息" />
        <el-step title="开户登记" />
        <el-step title="确认告知" />
      </el-steps>

      <!-- 第 1 步：账户信息 -->
      <el-form
        v-show="step === 0"
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="110px"
      >
        <el-form-item
          label="用户名"
          prop="username"
        >
          <el-input
            v-model="form.username"
            placeholder="3-32 位字母数字下划线"
          />
        </el-form-item>
        <el-form-item
          label="密码"
          prop="password"
        >
          <el-input
            v-model="form.password"
            type="password"
            show-password
            placeholder="至少 8 位，含大小写字母与数字"
          />
        </el-form-item>
        <el-form-item
          label="确认密码"
          prop="confirmPassword"
        >
          <el-input
            v-model="form.confirmPassword"
            type="password"
            show-password
          />
        </el-form-item>
        <el-form-item
          label="登录邮箱"
          prop="email"
        >
          <el-input
            v-model="form.email"
            placeholder="用于登录与接收系统通知"
          />
        </el-form-item>
      </el-form>

      <!-- 第 2 步：开户登记（监管必需项）-->
      <el-form
        v-show="step === 1"
        :model="form"
        :rules="regRules"
        label-width="110px"
      >
        <el-alert
          type="info"
          :closable="false"
          class="mb-12"
          title="为什么需要这些信息"
          description="公司名称、负责人、联系电话、邮箱是平台履行监管责任时能够联系到经营主体的最小信息集。这些信息只用于合规登记与必要联系，不用于任何其它用途。"
        />
        <el-form-item
          label="公司名称"
          prop="company_name"
        >
          <el-input
            v-model="form.company_name"
            placeholder="营业执照上的名称（个人使用可填本人姓名）"
          />
        </el-form-item>
        <el-form-item
          label="负责人/法人"
          prop="real_name"
        >
          <el-input
            v-model="form.real_name"
            placeholder="负责人/法人姓名"
          />
        </el-form-item>
        <el-form-item
          label="联系电话"
          prop="phone"
        >
          <el-input
            v-model="form.phone"
            placeholder="手机号或固定电话"
          />
        </el-form-item>
        <el-form-item label="统一社会信用代码">
          <el-input
            v-model="form.license_no"
            placeholder="选填，建议填写以便后续快速核验"
          />
        </el-form-item>
        <el-form-item label="通信地址">
          <el-input
            v-model="form.address"
            placeholder="选填"
          />
        </el-form-item>
      </el-form>

      <!-- 第 3 步：监管告知确认 -->
      <div v-show="step === 2">
        <el-alert
          type="warning"
          show-icon
          :closable="false"
          title="平台监管责任告知"
          class="mb-12"
        />
        <div class="notice">
          {{ notice.notice || '加载中…' }}
        </div>
        <div class="notice-meta">
          协议版本 <span class="mono">{{ notice.agreement_version || '—' }}</span>
        </div>
        <el-checkbox
          v-model="form.regulatory_ack"
          class="mt-12"
        >
          我已阅读并同意上述告知，确认所填信息真实有效
        </el-checkbox>
      </div>

      <div class="actions">
        <el-button
          v-if="step > 0"
          @click="step--"
        >
          上一步
        </el-button>
        <el-button
          v-if="step < 2"
          type="primary"
          :loading="submitting"
          @click="next"
        >
          下一步
        </el-button>
        <el-button
          v-else
          type="primary"
          :loading="submitting"
          @click="submit"
        >
          完成注册
        </el-button>
        <el-button
          link
          type="primary"
          @click="$router.push('/login')"
        >
          已有账号，去登录
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElNotification } from 'element-plus'
import { http } from '@/utils/request'
import { regulatoryNotice } from '@/api/moduleMarket'

const router = useRouter()
const formRef = ref()
const step = ref(0)
const submitting = ref(false)
const notice = ref({ notice: '', agreement_version: '', required_fields: [] })

const form = reactive({
  username: '', password: '', confirmPassword: '', email: '',
  company_name: '', real_name: '', phone: '', license_no: '', address: '',
  regulatory_ack: false
})

const rules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 32, message: '长度 3-32 位', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, message: '至少 8 位', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    {
      validator: (r, v, cb) => (v === form.password ? cb() : cb(new Error('两次输入不一致'))),
      trigger: 'blur'
    }
  ],
  email: [
    { required: true, message: '请输入邮箱', trigger: 'blur' },
    { type: 'email', message: '邮箱格式不正确', trigger: 'blur' }
  ]
}

// 开户登记的必填口径与后端一致（后端是最终把关，前端只是为了少一次往返）
const regRules = {
  company_name: [{ required: true, message: '请输入公司名称', trigger: 'blur' }],
  real_name: [{ required: true, message: '请输入负责人/法人姓名', trigger: 'blur' }],
  phone: [
    { required: true, message: '请输入联系电话', trigger: 'blur' },
    { pattern: /^[0-9+\-() ]{6,32}$/, message: '电话格式不正确', trigger: 'blur' }
  ]
}

async function next () {
  if (step.value === 0) {
    await formRef.value?.validateField(['username', 'password', 'confirmPassword', 'email'])
      .catch(() => {})
    // 逐项校验：validateField 失败时字段会红，但流程仍会往下走 —— 这里显式拦一次
    const ok = await validateGroup(formRef, ['username', 'password', 'confirmPassword', 'email'])
    if (!ok) return
  }
  if (step.value === 1) {
    const ok = await validateGroup(formRef, ['company_name', 'real_name', 'phone'])
    if (!ok) return
  }
  step.value++
}

async function validateGroup (ref, fields) {
  try {
    await ref?.value?.validateField(fields)
    return true
  } catch {
    return false
  }
}

async function submit () {
  if (!form.regulatory_ack) {
    ElMessage.warning('请先阅读并确认平台监管责任告知')
    return
  }
  const ok = await validateGroup(formRef, [
    'username', 'password', 'confirmPassword', 'email',
    'company_name', 'real_name', 'phone'
  ])
  if (!ok) {
    // 出错时把第一个错的步骤翻出来：用户点「完成注册」却什么都没发生是最差的体验
    step.value = 0
    await formRef.value?.validate().catch(() => {})
    return
  }

  submitting.value = true
  try {
    // SaaS 开户：POST /api/tenant/register —— 一并创建租户（不再落到默认租户 1）。
    // 字段按后端 RegisterTenantRequest 口径：name=租户展示名（复用公司名称）、
    // legal_person=负责人（表单字段仍叫 real_name）。响应是 {tenant_id, tenant_code, token}，
    // 没有 user 对象，所以注册后不自动登录，跳回登录页让用户用刚设的凭据登录。
    const res = await http.post('/api/tenant/register', {
      name: form.company_name,
      username: form.username,
      email: form.email,
      password: form.password,
      company_name: form.company_name,
      legal_person: form.real_name,
      phone: form.phone,
      license_no: form.license_no,
      address: form.address,
      regulatory_ack: form.regulatory_ack,
      agreement_version: notice.value.agreement_version
    })
    // 租户代码是后续登录/开通子账号的凭据，漏看就麻烦了——用常驻通知而不是 3 秒 toast
    const tenantCode = res?.tenant_code || res?.data?.tenant_code || ''
    ElNotification({
      type: 'success',
      title: '注册成功！',
      message: `您的租户代码：${tenantCode || '—'}，请妥善保存（登录与开通子账号时需要）`,
      duration: 0
    })
    router.push('/login')
  } catch (e) {
    // 后端 bizerr 码：4103 邮箱已注册 / 4104 注册已关闭 / 4105 限流。
    // 文案用机器读数字段分流（bizCode/response.data.code），不靠 message 措辞判分支。
    const bizCode = String(e?.bizCode ?? e?.response?.data?.code ?? '')
    const codeMsg = {
      4103: '该邮箱已注册，请直接登录或更换邮箱',
      4104: '平台当前已关闭注册，请联系管理员',
      4105: '注册过于频繁，请稍后再试'
    }
    ElMessage.error(codeMsg[bizCode] || e?.response?.data?.message || e?.message || '注册失败')
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  try {
    const res = await regulatoryNotice()
    notice.value = res || notice.value
  } catch {
    notice.value = {
      notice: '本平台为软件产品服务方，不参与商户与最终客户之间的具体交易。商户应自行确保其经营活动合法合规，并对使用本平台产生的行为承担全部责任。',
      agreement_version: '',
      required_fields: []
    }
  }
})
</script>

<style scoped>
.register-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-bg-color-page);
  padding: 32px 16px;
}
.card { width: 100%; max-width: 620px; }
.head h2 { margin: 0 0 4px; font-size: 20px; }
.sub { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; }
.mb-16 { margin-bottom: 24px; }
.mb-12 { margin-bottom: 12px; }
.mt-12 { margin-top: 12px; }
.notice {
  background: var(--el-fill-color-light);
  border-radius: 6px; padding: 14px 16px;
  line-height: 1.9; font-size: 14px;
  max-height: 260px; overflow: auto;
}
.notice-meta { margin-top: 8px; color: var(--el-text-color-secondary); font-size: 12px; }
.mono { font-family: monospace; }
.actions { margin-top: 20px; display: flex; gap: 10px; align-items: center; }
</style>
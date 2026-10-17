package errors

// 业务错误码
const (
	CodeSuccess         = 0
	CodeParamInvalid    = 4000
	CodeUnauthorized    = 4001
	CodeForbidden       = 4003
	CodeNotFound        = 4004
	CodeConflict        = 4009
	CodeInternal        = 5000
	CodePlatformUnavail = 5001
	CodeAssetNotFound   = 6001
	CodeAssetDup        = 6002
	CodeAssetInvalid    = 6003
	CodeSyncFailed      = 6004
	CodeLoaderFallback  = 6005
)

// 功能模块化业务码（.specify/002-modular-subscription）
//
// 为什么单独开一段而不是塞进 6000 段：6000 段是「资产域」（资产包的
// 购买/同步/校验），功能模块是**授权域**。两者的处置方式完全不同 ——
// 资产错误多半是"重试或联系平台"，模块错误是"去市场开通"。
// 混在一段里，前端无法按段做差异化引导。
const (
	// CodeModuleNotEnabled 4301 该功能模块未开通（模块闸拦截）
	//
	// 用 4xxx 段（鉴权/权限）而不是 5xxx：本码的处置是"引导购买"，
	// 与权限不足同属「你的账号当前做不到」这一类。
	CodeModuleNotEnabled = 4301
	// CodeModuleNotFound 4302 模块不存在（目录里查无此 key）
	CodeModuleNotFound = 4302
	// CodeModulePlatformUnavail 4303 平台未连接/不可达，无法办理模块开通
	//
	// 与 CodePlatformUnavail(5001) 分开的原因：后者是"资产同步失败"，
	// 用户能重试；本码是"你离线部署，收费模块开不了"，重试一万次也一样，
	// 引导话术必须不同。
	CodeModulePlatformUnavail = 4303
	// CodeModuleRegistrationIncomplete 4304 开户登记信息不完整（缺公司名/负责人/电话/邮箱）
	CodeModuleRegistrationIncomplete = 4304
)

// 租户/开户业务码（specs/001-saas-multi-tenant T029/T030 contracts/api.md）
const (
	// CodeTenantParamInvalid 4101 开户参数错误（企业名/用户名/邮箱/密码不合规）
	CodeTenantParamInvalid = 4101
	// CodeTenantUsernameTaken 4102 用户名在租户内重复（跨租户可并存）
	CodeTenantUsernameTaken = 4102
	// CodeTenantEmailTaken 4103 邮箱全局重复（登录通道唯一，跨租户亦不可并存）
	CodeTenantEmailTaken = 4103
	// CodeTenantRegisterClosed 4104 注册未开放（离线 TENANT_MODE=off 恒返回）
	CodeTenantRegisterClosed = 4104
	// CodeTenantRegisterRateLimited 4105 注册限流（同源 IP/指纹频率超限）
	CodeTenantRegisterRateLimited = 4105
	// CodeTenantNotFound 4106 租户不存在
	CodeTenantNotFound = 4106
	// CodeTenantSuspended 4107 租户已停用：登录受限、后台任务跳过（数据保留）
	CodeTenantSuspended = 4107
	// CodeTenantAmbiguousUsername 4108 用户名跨租户歧义，提示改用邮箱登录
	CodeTenantAmbiguousUsername = 4108
)

// BizError 业务错误
type BizError struct {
	Code    int
	Message string
	Cause   error
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *BizError) Unwrap() error { return e.Cause }

func New(code int, msg string) *BizError {
	return &BizError{Code: code, Message: msg}
}

func Wrap(code int, msg string, cause error) *BizError {
	return &BizError{Code: code, Message: msg, Cause: cause}
}

// LLM 计费业务码（模块化改造 · 任务五）
//
// 为什么单开一段而不塞进 4xxx：4xxx 是"你这个账号当前做不到"（鉴权/权限），
// 而余额不足是"你这个账号**可以**用，但账户没钱" —— 前者引导找管理员，
// 后者引导去充值，混在一起前端无法分流。
const (
	// CodeLLMBalanceInsufficient 4401 平台模型余额不足
	//
	// 判据与 dispatcher 侧的 ErrInsufficientLLMBalance 一一对应：
	// 平台代理返回 402 → llm.ErrInsufficientLLMBalance → 本码。
	CodeLLMBalanceInsufficient = 4401
	// CodeLLMPlatformUnavail 4402 平台模型不可用（未连接平台 / provider 未配置）
	CodeLLMPlatformUnavail = 4402
)

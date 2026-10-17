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

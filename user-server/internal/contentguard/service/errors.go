// errors.go 服务层 sentinel 错误（控制器按它们映射状态码）。
package service

import "errors"

var (
	// ErrSanctionStoreUnavailable 处置仓储未装配。
	ErrSanctionStoreUnavailable = errors.New("content guard sanction store unavailable")
	// ErrSanctionSubjectRequired 处置主体缺失。
	ErrSanctionSubjectRequired = errors.New("content guard sanction subject required")
	// ErrSanctionKindInvalid 处置类型非法。
	ErrSanctionKindInvalid = errors.New("content guard sanction kind invalid")
	// ErrSanctionLevelInvalid 处置时长档非法。
	ErrSanctionLevelInvalid = errors.New("content guard sanction level invalid")
	// ErrSanctionNotFound 处置不存在。
	ErrSanctionNotFound = errors.New("content guard sanction not found")
	// ErrRecordNotFound 审查记录不存在。
	ErrRecordNotFound = errors.New("content guard record not found")
	// ErrWordNotFound 词条不存在。
	ErrWordNotFound = errors.New("content guard word not found")
	// ErrRuntimeUnavailable 运行时未装配（路由照挂，端点回 503）。
	ErrRuntimeUnavailable = errors.New("content guard runtime unavailable")
	// ErrReviewStatusInvalid 人工复核状态非法。
	ErrReviewStatusInvalid = errors.New("content guard review status invalid")
)

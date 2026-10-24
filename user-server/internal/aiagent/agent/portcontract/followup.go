package portcontract

import (
	"context"
	"time"
)

// FollowUpScheduleOptions 跟进任务选项投影。
type FollowUpScheduleOptions struct {
	Priority string
	Note     string
	Title    string
}

// FollowUpPort 跟进任务端口（实现方 service.FollowUpService，消费方 tooluse 跟进相关工具）。
//
// ResultInfo 的 result 取值须与 service.FollowUpResult 枚举对齐，
// 工具层不应硬编码 stage 名，而是通过 ResultInfo 反查目标 SOP 阶段。
type FollowUpPort interface {
	Schedule(ctx context.Context, customerID, ownerID, reminderType string, dueIn time.Duration, opts *FollowUpScheduleOptions) (any, error)
	CompleteWithResult(reminderID string, result, note string) error
	Cancel(reminderID string) error
	ResultInfo(result string) (stage string, ok bool)
}

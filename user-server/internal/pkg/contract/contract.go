// Package contract 是**租户端声明的线上契约版本**与"平台要求本机升级"这一状态的唯一事实源。
//
// # 它解决什么
//
// 平台与 user-server 是两个仓、各自发布。独立部署的实例生命周期远长于云端，所以
// "老实例连新平台"是常态而不是意外。平台侧（`internal/middleware/contract_handshake.go`）
// 对每个请求校验本包声明的版本，不兼容就回 **HTTP 426 Upgrade Required**。
//
// 本包负责另一半：**把这个 426 变成管理员看得见的东西**。
// 只写进日志是不够的——A4 的原始症状恰恰是"失败被 logger.Warn 吞掉，表现为同步一直没生效"。
// 所以这里存一份"平台已经拒绝我们了"的状态，由 `/api/system/info`（管理员可见）带出去。
package contract

import (
	"strconv"
	"sync"
	"time"
)

// HeaderName 租户端在每个发往平台的请求上声明的契约版本头。
// 与平台端 `internal/contract.HeaderName` **逐字一致**（两侧各有一条 pin 住它的用例）。
const HeaderName = "X-Contract-Version"

// Version 本机实现的契约版本。
//
// 改它等于宣布"线上协议不兼容地变了"，必须与平台端 `internal/contract.Version` 同批升级；
// 两侧各有一条 `TestContractVersionPinned` 钉住这个字面量，任一侧漂移都会红。
const Version = 1

// HeaderValue 返回要发出去的版本头值。
func HeaderValue() string { return strconv.Itoa(Version) }

// lastBlocked 记录"平台最近一次以 426 拒绝我们"的信息。
//
// 用互斥量而不是 atomic：这里存的是一个 struct（msg + 时间 + 计数），
// 原子替换一个指针会让读侧拿到半旧半新的组合，而这份状态的用途正是"给运维看的一行字"。
var (
	mu         sync.Mutex
	blockedMsg string
	blockedAt  time.Time
	blockedN   int64
)

// RecordUpgradeRequired 记下平台要求本机升级（426）。
//
// 幂等：同一条原因重复上报只更新时间与次数，不重复堆文案 —— 否则管理端会显示一屏相同告警。
func RecordUpgradeRequired(msg string) {
	mu.Lock()
	defer mu.Unlock()
	blockedMsg = msg
	blockedAt = time.Now()
	blockedN++
}

// ClearUpgradeRequired 清掉告警。平台重新接受本机的请求时调用。
func ClearUpgradeRequired() {
	mu.Lock()
	defer mu.Unlock()
	blockedMsg = ""
	blockedAt = time.Time{}
	blockedN = 0
}

// Status 平台契约兼容状态（管理端展示用）。
type Status struct {
	// Message 平台给出的原因（可为空 = 没被拒过）
	Message string `json:"message"`
	// Since 最近一次被拒的时间（RFC3339，空 = 没被拒过）
	Since string `json:"since,omitempty"`
	// Declared / Required 两侧版本，排障时第一眼要看的两格
	Declared int `json:"declared"`
	Required int `json:"required,omitempty"`
	// Count 累计被拒次数（同一原因会累加；用于判断"是不是一直在打"）
	Count int64 `json:"count,omitempty"`
}

// UpgradeRequired 返回当前状态。ok=false 表示平台没有拒绝过本机。
func UpgradeRequired() (st Status, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	if blockedMsg == "" {
		return Status{}, false
	}
	return Status{
		Message:  blockedMsg,
		Since:    blockedAt.Format(time.RFC3339),
		Declared: Version,
		Count:    blockedN,
	}, true
}

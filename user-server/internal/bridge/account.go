package bridge

import "context"

// BridgeAccountUpsert 账号注册/上线时持久化的字段。
// UserID=0 是常态：登记入口（HTTP ingest）只认实例级桥接凭证，没有用户身份，
// 账号归属落在租户上；非零只在带用户身份的写入路径出现。
type BridgeAccountUpsert struct {
	UserID      uint
	Channel     string
	AccountID   string
	AgentID     uint
	AccountName string
	Status      string
}

// BridgeAccountView 账号视图（含来自 hub 的实时在线状态）
type BridgeAccountView struct {
	ID          uint        `json:"id"`
	UserID      uint        `json:"user_id"`
	Channel     string      `json:"channel"`
	AccountID   string      `json:"account_id"`
	AccountName string      `json:"account_name"`
	AgentID     uint        `json:"agent_id"`
	Status      string      `json:"status"`
	Online      bool        `json:"online"`
	LastSyncAt  interface{} `json:"last_sync_at"`
}

// BridgeAccountRepo 桥接账号持久化接口（由 account_repo.go 实现，router 启动时注入）。
// 设计为接口 + 包级全局，避免改动桥接入站 handler 的构造签名、不影响既有测试。
type BridgeAccountRepo interface {
	Upsert(ctx context.Context, u BridgeAccountUpsert) error
	SetOffline(ctx context.Context, channel, accountID string) error
	TouchLastSync(ctx context.Context, channel, accountID string) error
	ListForUser(ctx context.Context, userID uint) ([]BridgeAccountView, error)
	IsOnline(ctx context.Context, channel, accountID string) (bool, error)
}

// GlobalBridgeAccountRepo 包级注入点（router.Setup 中调用 RegisterBridgeAccountRepo）
var GlobalBridgeAccountRepo BridgeAccountRepo

// RegisterBridgeAccountRepo 注入桥接账号持久化实现
func RegisterBridgeAccountRepo(r BridgeAccountRepo) {
	GlobalBridgeAccountRepo = r
}

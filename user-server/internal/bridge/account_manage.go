package bridge

import (
	"context"
	"errors"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/service"
)

// 桥接账号管理面的服务出口。
//
// controller 只做参数绑定与状态码映射；归属判定、渠道归一、租户钉死、
// 在线门与出站编排都在这里，避免 controller 直接持有仓储（五层唯一的
// 例外是装配期的 app 层适配器，管理面不属于例外）。

var (
	// ErrBridgeAccountRepoNotReady 账号仓储未装配（router.Setup 完成之前）
	ErrBridgeAccountRepoNotReady = errors.New("bridge account repo not initialized")
	// ErrBridgeAccountNotOwned 账号不在当前用户/租户的可见范围内
	ErrBridgeAccountNotOwned = errors.New("account not owned by current user")
	// ErrBridgeAccountOffline 账号收不到下行（扩展断开或心跳超出宽限窗）
	ErrBridgeAccountOffline = errors.New("bridge account offline")
)

// ListAccounts 管理面：当前租户内当前用户可见的桥接账号（含实时在线位）。
func ListAccounts(ctx context.Context, userID uint) ([]BridgeAccountView, error) {
	if GlobalBridgeAccountRepo == nil {
		return nil, ErrBridgeAccountRepoNotReady
	}
	return GlobalBridgeAccountRepo.ListForUser(ctx, userID)
}

// SendManualMessage 人工座席经桥接代发文本消息。
//
// 渠道先归一：bridge_accounts、在线判定与出站白名单用的都是规范名，
// 别名（douyin_web）原样下传会查不到账号行、也过不了出站白名单。
//
// 归属成立后把 ctx 钉在账号租户上（OnlyTenant）：调用方可能是带
// SystemScope 的平台超管，隔离 callback 见到 SystemScope 会放行跨租户读、
// 且 Create 不填租户列；在线判定与 message_hub 落库都必须落在账号租户内。
func SendManualMessage(ctx context.Context, userID uint, channel, accountID, conversationID, content string) error {
	channel = NormalizeBridgeChannel(channel)
	if GlobalOwnershipChecker != nil {
		owns, accTenant, err := GlobalOwnershipChecker(ctx, userID, channel, accountID)
		if err != nil {
			return err
		}
		if !owns {
			return ErrBridgeAccountNotOwned
		}
		if accTenant != 0 {
			ctx = tenant.OnlyTenant(ctx, accTenant)
		}
	}
	if GlobalBridgeAccountRepo == nil {
		return ErrBridgeAccountRepoNotReady
	}
	online, err := GlobalBridgeAccountRepo.IsOnline(ctx, channel, accountID)
	if err != nil {
		return err
	}
	if !online {
		return ErrBridgeAccountOffline
	}
	return service.DeliverBridgeOutbound(ctx, channel, accountID, conversationID, "text", content, "")
}

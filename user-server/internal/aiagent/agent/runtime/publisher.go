package agent_runtime

import (
	"context"
	"time"

	"hivemtk-user/internal/event"
	"hivemtk-user/internal/pkg/tenant"
)

// PublishCustomerMessage 发布客户消息事件
//
// 调用方:WebhookService.triggerSalesEngine / 各 Channel Adapter
//
// 参数:
//   - channelType: telegram / wecom / feishu / douyin / ...
//   - accountID:   渠道账号主键
//   - customerID:  客户 OneID(已归一化)
//   - sessionID:   会话唯一 ID(方向8 核心数据流必备,缺省由 channel:customer 构造)
//   - content:     消息内容
//   - traceID:     全链路追踪 ID(空字符串时自动生成)
//
// 返回:
//   - 生成的 TraceID(便于调用方记录到日志)
func PublishCustomerMessage(ctx context.Context, channelType, accountID, customerID, sessionID, content, traceID string) string {
	if traceID == "" {
		traceID = "agent_runtime_" + time.Now().Format("20060102150405.000000")
	}
	if sessionID == "" {
		sessionID = channelType + ":" + customerID
	}

	payload := event.CustomerMessagePayload{
		// 租户行自证（T021）：发布点从入站 ctx 提取（解析链已绑定）
		TenantID:    tenantIDFromCtx(ctx),
		ChannelType: channelType,
		AccountID:   accountID,
		CustomerID:  customerID,
		SessionID:   sessionID,
		Content:     content,
		MessageType: "text",
		Timestamp:   time.Now(),
		TraceID:     traceID,
	}

	event.Publish(event.TopicCustomerMessageReceived, payload)
	return traceID
}

// PublishCustomerMessageWithType 发布客户消息事件(支持自定义消息类型)
//
// 用于图片/语音/事件 等非文本消息
func PublishCustomerMessageWithType(ctx context.Context, channelType, accountID, customerID, sessionID, content, messageType, traceID string) string {
	if traceID == "" {
		traceID = "agent_runtime_" + time.Now().Format("20060102150405.000000")
	}
	if sessionID == "" {
		sessionID = channelType + ":" + customerID
	}

	payload := event.CustomerMessagePayload{
		TenantID:    tenantIDFromCtx(ctx),
		ChannelType: channelType,
		AccountID:   accountID,
		CustomerID:  customerID,
		SessionID:   sessionID,
		Content:     content,
		MessageType: messageType,
		Timestamp:   time.Now(),
		TraceID:     traceID,
	}

	event.Publish(event.TopicCustomerMessageReceived, payload)
	return traceID
}

// tenantIDFromCtx 发布侧租户提取（0 = off 模式/系统级发布）
func tenantIDFromCtx(ctx context.Context) uint {
	if ctx == nil {
		return 0
	}
	id, _ := tenant.FromContext(ctx)
	return id
}

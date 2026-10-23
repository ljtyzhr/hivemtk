package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// 线程容器回声：扩展把整个会话线程（客户旧消息 + AI 多条回复）当一个气泡抓回时，
// 出站文本被客户旧消息隔开，逐条剥离后仍会剩下客户的历史发言（≥4 字），
// 旧的「剩余近空」判据放行 → 整段线程再触发一次 AI 回复，线程越滚越长。
// 判据必须能识别：这段内容的**结尾**就是本账号刚发出去的话（客户没有新发言）。
func TestInboxIngress_ThreadContainerTailEcho_Blocked(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressService()
	ctx := context.Background()

	const (
		platform = "douyin"
		account  = "acct-thread-1"
		conv     = "conv-thread-1"
	)
	out1 := "收到！武汉的业主您好，方便告诉我您的建筑面积和全包半包想法吗？"
	out2 := "还有一个关键问题：您是想全包还是半包？全包省心点，半包自己掌控主材，价格差挺多的~"
	for i, c := range []string{out1, out2} {
		if err := db.Create(&model.MessageHub{
			MsgID:          "mh:ob-thread-" + string(rune('0'+i)),
			Platform:       platform,
			AccountID:      account,
			Direction:      "outbound",
			Status:         "delivered",
			MsgType:        "text",
			ConversationID: conv,
			SenderID:       account,
			Content:        c,
			SentAt:         time.Now().Add(-30 * time.Second),
		}).Error; err != nil {
			t.Fatalf("插入 outbound 失败: %v", err)
		}
	}

	// 线程内容按时间顺序：客户旧问题 → AI 回复1 → 客户补充 → AI 回复2。
	// 结尾是我们的回复2 ⇒ 客户在这段内容里没有新发言。
	threadContent := "多少钱一平？" + out1 + "全包吧" + out2
	event := &model.MessageEvent{
		Channel:        platform,
		SenderID:       conv,
		SenderName:     "抖音客户",
		SenderType:     "customer",
		Content:        threadContent,
		EventID:        "mh:thread-echo-1",
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	decision, err := svc.interceptInbound(ctx, event)
	if err != nil {
		t.Fatalf("interceptInbound 失败: %v", err)
	}
	if decision == nil || !decision.Blocked || !decision.IsSelfEcho {
		t.Fatalf("线程容器回声应被拦截, got: %+v", decision)
	}
	if !strings.Contains(decision.Reason, "recent outbound") {
		t.Fatalf("拦截原因应包含 recent outbound, got: %q", decision.Reason)
	}
}

// 客户只回一句短话，恰好是近期出站尾部出现过的小词（如「可以吗？」）：
// 短入站不参与尾部命中，避免把真实短回复吞掉。
func TestInboxIngress_ShortCustomerEchoOfTailPhrase_NotBlocked(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressService()
	ctx := context.Background()

	const (
		platform = "douyin"
		account  = "acct-short-tail"
		conv     = "conv-short-tail"
	)
	if err := db.Create(&model.MessageHub{
		MsgID:          "mh:ob-short-tail",
		Platform:       platform,
		AccountID:      account,
		Direction:      "outbound",
		Status:         "delivered",
		MsgType:        "text",
		ConversationID: conv,
		SenderID:       account,
		Content:        "这个方案我们明天上午安排师傅上门量房，量完当天出报价，您看可以吗？",
		SentAt:         time.Now().Add(-20 * time.Second),
	}).Error; err != nil {
		t.Fatalf("插入 outbound 失败: %v", err)
	}

	event := &model.MessageEvent{
		Channel:        platform,
		SenderID:       conv,
		SenderName:     "抖音客户",
		SenderType:     "customer",
		Content:        "可以吗？",
		EventID:        "mh:short-tail-1",
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	decision, err := svc.interceptInbound(ctx, event)
	if err != nil {
		t.Fatalf("interceptInbound 失败: %v", err)
	}
	if decision != nil && decision.Blocked {
		t.Fatalf("客户短回复不应被尾部命中拦截, got: %+v", decision)
	}
}

// 客户在引用我们的长段落之后补了一句自己的问题（结尾是客户的话）：
// 尾部命中不成立，必须放行。
func TestInboxIngress_ThreadWithCustomerTail_NotBlocked(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressService()
	ctx := context.Background()

	const (
		platform = "douyin"
		account  = "acct-thread-tail"
		conv     = "conv-thread-tail"
	)
	out1 := "全包的话大概每平一千二到一千五，具体要量房之后才能给您出准确报价单"
	if err := db.Create(&model.MessageHub{
		MsgID:          "mh:ob-thread-tail",
		Platform:       platform,
		AccountID:      account,
		Direction:      "outbound",
		Status:         "delivered",
		MsgType:        "text",
		ConversationID: conv,
		SenderID:       account,
		Content:        out1,
		SentAt:         time.Now().Add(-20 * time.Second),
	}).Error; err != nil {
		t.Fatalf("插入 outbound 失败: %v", err)
	}

	event := &model.MessageEvent{
		Channel:        platform,
		SenderID:       conv,
		SenderName:     "抖音客户",
		SenderType:     "customer",
		Content:        "之前你说" + out1 + "，那半包的话能便宜多少呢？",
		EventID:        "mh:thread-tail-1",
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	decision, err := svc.interceptInbound(ctx, event)
	if err != nil {
		t.Fatalf("interceptInbound 失败: %v", err)
	}
	if decision != nil && decision.Blocked && decision.IsSelfEcho {
		t.Fatalf("客户结尾有自己提问时不应判回声, got: %+v", decision)
	}
}

package repository

// 契约锁（实体审查 #61，ChampionDialogue）：销冠对话的读腿不许跨租户取到别人的对话正文。
//
// 风险形状：FetchChampionDialogueCandidates 的 SELECT 里有两条**相关子查询**
// （`(SELECT customer_msg FROM feedback_events WHERE session_id = fs.session_id ...)`），
// 它们是写在 Select 里的 SQL 片段，租户 callback **注入不到里面**——callback 只能给
// 外层表（`feedback_signals fs`）加 `tenant_id` 条件。
//
// 于是要验的那一格是：子查询按 `session_id` 取最近一条 feedback_events，而 session_id
// 在两个租户里**可以是同一个值**（两边各自有一套会话命名空间）⇒ A 租户读候选时，
// 子查询可能命中 B 租户那一行，把 B 的客户消息/AI 回包原文带进 A 的销冠对话语料
// （而 A 的外层行仍是 A 自己的——所以从外层看「一切正常」）。
//
// 另一条腿：PersistChampionDialogue 的 ON CONFLICT 目标列必须与库里的唯一索引形状
// 逐列一致（`idx_champion_dialogues_dialogue_fingerprint` 被 Normalize 复合成
// `(tenant_id, dialogue_fingerprint)`）——对不上就不是「重复对话按 reward 覆盖」，
// 而是冲突目标不存在（真故障）或根本不去重。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	cdTenantA = uint(23)
	cdTenantB = uint(37)
	// 两租户用同一个 session_id：子查询的跨租户面靠它暴露。
	cdSharedSession = "s-shared-across-tenants"
)

func cdDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.FeedbackSignal{}, &model.FeedbackEvent{}, &model.ChampionDialogue{})
	if db == nil {
		t.Fatal("测试库不可达：销冠对话的租户性无法判定（不 Skip，跳过等于没锁）")
	}
	return db
}

// cdSeedSignal 落一条 feedback_signals（外层表）。
func cdSeedSignal(t *testing.T, db *gorm.DB, ctx context.Context, customerID string, reward float64) {
	t.Helper()
	sig := &model.FeedbackSignal{
		SessionID: cdSharedSession, CustomerID: customerID,
		AggregatedReward: reward, IsChampion: true,
		SignalBreakdown: model.JSONMap{}, Outcome: model.FeedbackSignalOutcomeSuccess,
	}
	if err := db.WithContext(ctx).Create(sig).Error; err != nil {
		t.Fatalf("落 feedback_signal 失败: %v", err)
	}
}

// cdSeedEvent 落一条 feedback_events（子查询的目标表）。
func cdSeedEvent(t *testing.T, db *gorm.DB, ctx context.Context, customerMsg, aiReply string) {
	t.Helper()
	ev := &model.FeedbackEvent{
		EventID: "ev-" + customerMsg, SessionID: cdSharedSession, CustomerID: "c-x",
		EventType: "reply", SignalKey: "k", SignalValue: model.JSONMap{},
		CustomerMsg: customerMsg, AIReply: aiReply,
	}
	if err := db.WithContext(ctx).Create(ev).Error; err != nil {
		t.Fatalf("落 feedback_event 失败: %v", err)
	}
}

// TestChampionDialogueCandidatesDoNotLeakAcrossTenants 候选拉取里的子查询不许取到别租户的正文。
//
// 形状：两个租户用**同一个 session_id** 各落一条 feedback_signals + 一条 feedback_events。
// A 租户读候选时，外层只会看到 A 自己的 signal 行；但 SELECT 里的两条相关子查询
// （按 session_id 取最近一条 feedback_events 的 customer_msg / ai_reply）若没有租户条件，
// 就会取到 B 租户那一行 ⇒ A 的销冠语料里混进 B 的客户原文。
//
// 判据：A 读到的 customer_msg / ai_reply 必须是 A 自己的那两条。
func TestChampionDialogueCandidatesDoNotLeakAcrossTenants(t *testing.T) {
	db := cdDB(t)
	ctxA := tenant.WithTenantID(context.Background(), cdTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cdTenantB)

	cdSeedSignal(t, db, ctxA, "cust-A", 9.0)
	cdSeedEvent(t, db, ctxA, "A 的客户消息", "A 的 AI 回包")
	cdSeedSignal(t, db, ctxB, "cust-B", 8.0)
	cdSeedEvent(t, db, ctxB, "B 的客户消息", "B 的 AI 回包")

	repo := NewFeedbackLoopRepositoryWithDB(db)
	rows, err := repo.FetchChampionDialogueCandidates(ctxA, "general", 0, time.Now().AddDate(0, -1, 0), 10)
	if err != nil {
		t.Fatalf("A 租户拉候选失败: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("A 租户读到 0 条候选：前提不成立（A 自己的 signal 行不在结果里），本腿无判据")
	}
	for _, r := range rows {
		if r.CustomerMsg == "B 的客户消息" || r.AIReply == "B 的 AI 回包" {
			t.Errorf("A 租户读到的候选里带了 B 租户的正文（customer_msg=%q ai_reply=%q）："+
				"SELECT 里的相关子查询没有 tenant_id 条件，"+
				"而 session_id 在两租户可以是同一个值 ⇒ 跨租户取到了别人的对话原文",
				r.CustomerMsg, r.AIReply)
		}
	}
}

// TestChampionDialoguePersistIsTenantLocal 写侧：同指纹在两租户各存一行，互不覆盖。
//
// 这一格盯的是 ON CONFLICT 目标列：库里的唯一索引是
// `idx_champion_dialogues_dialogue_fingerprint (tenant_id, dialogue_fingerprint)`
// （Normalize 把单列索引复合化后的形状），而代码的 ON CONFLICT 目标列必须**逐列一致**。
// 若目标列漏了 tenant_id，两租户同指纹的行会互相覆盖 ⇒ 一租户的销冠语料被另一租户
// 的 reward/conversion_achieved 改写，而对话正文与指纹都还是自己的 ⇒ 从行上看不出来。
func TestChampionDialoguePersistIsTenantLocal(t *testing.T) {
	db := cdDB(t)
	ctxA := tenant.WithTenantID(context.Background(), cdTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cdTenantB)
	repo := NewFeedbackLoopRepositoryWithDB(db)

	// embedding 是 vector(1024) NOT NULL，写入用 1024 维零向量字面量。
	vec := make([]string, 1024)
	for i := range vec {
		vec[i] = "0"
	}
	lit := "[" + joinComma(vec) + "]"

	persist := func(ctx context.Context, reward float64, conv bool) {
		t.Helper()
		err := repo.PersistChampionDialogue(ctx, ChampionDialoguePersist{
			Fingerprint: "fp-shared", SessionID: cdSharedSession, CustomerID: "c-x",
			Scenario: "general", CustomerMsg: "客户消息", ChampionReply: "销冠回包",
			EmbeddingLiteral: lit, ClusterID: 1,
			Reward: reward, ConversionAchieved: conv,
		})
		if err != nil {
			t.Fatalf("持久化销冠对话失败: %v", err)
		}
	}
	persist(ctxA, 1.5, false)
	persist(ctxB, 9.5, true)

	var rows []model.ChampionDialogue
	if err := db.WithContext(ctxA).Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("读 A 租户的行失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("A 租户读到 %d 行 want=1（同指纹在两租户各一行，A 只该看到自己那行）", len(rows))
	}
	if rows[0].TenantID != cdTenantA {
		t.Errorf("A 租户读到的行 tenant_id=%d want=%d", rows[0].TenantID, cdTenantA)
	}
	if rows[0].Reward != 1.5 {
		t.Errorf("A 的行 reward=%v want=1.5：被 B 租户的写入覆盖了 ⇒ "+
			"ON CONFLICT 目标列漏了 tenant_id（库里的唯一索引是 (tenant_id, dialogue_fingerprint)）",
			rows[0].Reward)
	}
	if rows[0].ConversionAchieved {
		t.Error("A 的行 conversion_achieved=true：被 B 租户的写入覆盖了（同上）")
	}
}

// joinComma 把向量分量拼成 pgvector 字面量。
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

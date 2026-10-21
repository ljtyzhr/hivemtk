package feedbackloop

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

type mockBanditUpdater struct {
	calls   []mockRewardCall
	failN   int
	failErr error
}

type mockRewardCall struct {
	ExperimentID string
	ArmKey       string
	Success      bool
	Reward       float64
}

func (m *mockBanditUpdater) UpdateReward(_ context.Context, experimentID, armKey string, success bool, reward float64) error {
	m.calls = append(m.calls, mockRewardCall{experimentID, armKey, success, reward})
	if m.failN > 0 {
		m.failN--
		return m.failErr
	}
	return nil
}

// D04: 转化事件正确映射到 running 实验臂并回流；台账防重复
func TestD04_RefluxOnceEndToEnd(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)

	exp := &model.PromptABTest{
		ExperimentID:   "exp-d04-1",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          42,
		ArmKeys:        model.JSONArray{"A", "B"},
		Status:         "running",
	}
	if err := db.Create(exp).Error; err != nil {
		t.Fatal(err)
	}
	arm := &model.BanditArm{
		ExperimentID:   "exp-d04-1",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		ArmKey:         "A",
		SOPID:          42,
		Variant:        "A",
	}
	if err := db.Create(arm).Error; err != nil {
		t.Fatal(err)
	}

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)

	ev := &model.FeedbackEvent{
		EventID:    "evt-d04-1",
		SessionID:  "sess-1",
		CustomerID: "c1",
		SOPID:      42,
		SignalKey:  "conversion",
		Reward:     2.0,
		Variant:    "A",
		CreatedAt:  time.Now(),
	}
	if err := db.Create(ev).Error; err != nil {
		t.Fatal(err)
	}

	stats, err := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 1 || stats.Refluxed != 1 {
		t.Fatalf("scanned=%d refluxed=%d, want 1/1 (skipped=%d duped=%d failed=%d)", stats.Scanned, stats.Refluxed, stats.Skipped, stats.Duped, stats.Failed)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("UpdateReward 应被调用 1 次, got %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.ExperimentID != "exp-d04-1" || call.ArmKey != "A" || !call.Success || call.Reward != 2.0 {
		t.Errorf("回流参数不符: %+v", call)
	}

	stats2, _ := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if stats2.Refluxed != 0 || stats2.Duped != 1 {
		t.Errorf("重扫应 duped=1 refluxed=0, got %+v", stats2)
	}
	if len(mock.calls) != 1 {
		t.Errorf("重扫不应再调 UpdateReward, got %d 次", len(mock.calls))
	}
}

// D04: 白名单外信号不入回流（tool_call 高频低值）
func TestD04_SignalWhitelist(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)

	ev := &model.FeedbackEvent{
		EventID:    "evt-d04-tool",
		SessionID:  "sess-w",
		CustomerID: "c1",
		SignalKey:  "tool_call",
		Reward:     0.3,
		CreatedAt:  time.Now(),
	}
	if err := db.Create(ev).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 0 || stats.Refluxed != 0 {
		t.Errorf("tool_call 应被白名单过滤, got scanned=%d refluxed=%d", stats.Scanned, stats.Refluxed)
	}
}

// D04: 负信号（complaint -2.0）→ success=false 且 reward 取绝对值
func TestD04_NegativeSignal(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)

	exp := &model.PromptABTest{
		ExperimentID:   "exp-d04-2",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          7,
		ArmKeys:        model.JSONArray{"A"},
		Status:         "running",
	}
	db.Create(exp)
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-2", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "A", SOPID: 7})

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	db.Create(&model.FeedbackEvent{
		EventID:    "evt-d04-neg",
		SessionID:  "sess-n",
		CustomerID: "c1",
		SOPID:      7,
		SignalKey:  "complaint",
		Reward:     -2.0,
		CreatedAt:  time.Now(),
	})
	stats, err := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Refluxed != 1 || len(mock.calls) != 1 {
		t.Fatalf("负信号应回流 1 次, got %+v", stats)
	}
	if mock.calls[0].Success || mock.calls[0].Reward != 2.0 {
		t.Errorf("负信号应 success=false reward=|−2.0|, got %+v", mock.calls[0])
	}
}

// D04: 无运行中实验 → skip
func TestD04_NoRunningExperimentSkips(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	db.Create(&model.FeedbackEvent{
		EventID:    "evt-d04-orphan",
		SessionID:  "sess-x",
		CustomerID: "c1",
		SOPID:      999,
		SignalKey:  "conversion",
		Reward:     2.0,
		CreatedAt:  time.Now(),
	})
	stats, _ := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if stats.Skipped != 1 {
		t.Errorf("无实验应 skip, got %+v", stats)
	}
}

// D04: 事件数超过单批上限时要按 id 游标续扫，不能只处理前 500 条
func TestD04_RefluxPaginatesBeyondBatchSize(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	db.Create(&model.PromptABTest{
		ExperimentID:   "exp-d04-page",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          88,
		ArmKeys:        model.JSONArray{"A"},
		Status:         "running",
	})
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-page", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "A", SOPID: 88, Variant: "A"})

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	r.batchSize = 2

	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := db.Create(&model.FeedbackEvent{
			EventID:    fmt.Sprintf("evt-page-%d", i),
			SessionID:  fmt.Sprintf("sess-page-%d", i),
			CustomerID: "c1",
			SOPID:      88,
			SignalKey:  "conversion",
			Reward:     2.0,
			CreatedAt:  now,
		}).Error; err != nil {
			t.Fatalf("seed event %d: %v", i, err)
		}
	}
	stats, err := r.RefluxOnce(context.Background(), now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Refluxed != 5 || stats.Duped != 0 || stats.Failed != 0 {
		t.Errorf("5 条事件应全部回流, got %+v", stats)
	}
	if len(mock.calls) != 5 {
		t.Errorf("UpdateReward 应调用 5 次, got %d", len(mock.calls))
	}
}

// D04: 同会话同 SOP 的第二笔 conversion 由复合唯一键去重，只能入账一次
func TestD04_ConversionDedupedBySessionAndSOP(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	db.Create(&model.PromptABTest{
		ExperimentID:   "exp-d04-conv",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          77,
		ArmKeys:        model.JSONArray{"A"},
		Status:         "running",
	})
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-conv", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "A", SOPID: 77, Variant: "A"})

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	now := time.Now()
	for i := 0; i < 2; i++ {
		db.Create(&model.FeedbackEvent{
			EventID:    fmt.Sprintf("evt-conv-%d", i),
			SessionID:  "sess-conv",
			CustomerID: "c1",
			SOPID:      77,
			SignalKey:  "conversion",
			Reward:     2.0,
			CreatedAt:  now,
		})
	}
	stats, err := r.RefluxOnce(context.Background(), now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Refluxed != 1 || stats.Duped != 1 {
		t.Errorf("同会话转化应 refluxed=1 duped=1, got %+v", stats)
	}
	if len(mock.calls) != 1 {
		t.Errorf("同会话转化只应更新一次臂, got %d", len(mock.calls))
	}
}

// D04: 克隆变体的臂挂在克隆 SOP 的 ID 上，事件按克隆 SOP 也必须解析到该臂
func TestD04_ClonedVariantArmResolvedByArmSOP(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	db.Create(&model.PromptABTest{
		ExperimentID:   "exp-d04-clone",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          100,
		ArmKeys:        model.JSONArray{"arm_a_original", "arm_b_variant"},
		Status:         "running",
	})
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-clone", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "arm_a_original", SOPID: 100, Variant: "A"})
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-clone", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "arm_b_variant", SOPID: 200, Variant: "B"})

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	now := time.Now()
	db.Create(&model.FeedbackEvent{
		EventID:    "evt-clone-b",
		SessionID:  "sess-clone",
		CustomerID: "c1",
		SOPID:      200,
		SignalKey:  "conversion",
		Reward:     2.0,
		Variant:    "B",
		CreatedAt:  now,
	})

	stats, err := r.RefluxOnce(context.Background(), now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Refluxed != 1 || len(mock.calls) != 1 {
		t.Fatalf("克隆变体事件应回流 1 次, got %+v calls=%d", stats, len(mock.calls))
	}
	if mock.calls[0].ExperimentID != "exp-d04-clone" || mock.calls[0].ArmKey != "arm_b_variant" {
		t.Errorf("应记到克隆变体臂, got %+v", mock.calls[0])
	}
}

// D04: 解析期的 DB 错误必须让本轮失败（cron 不推进游标），不能记 Skip 永久丢单
func TestD04_ResolveErrorAbortsWindow(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	db.Create(&model.PromptABTest{
		ExperimentID:   "exp-d04-err",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          55,
		ArmKeys:        model.JSONArray{"A"},
		Status:         "running",
	})
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-err", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "A", SOPID: 55, Variant: "A"})
	now := time.Now()
	db.Create(&model.FeedbackEvent{
		EventID:    "evt-err",
		SessionID:  "sess-err",
		CustomerID: "c1",
		SOPID:      55,
		SignalKey:  "conversion",
		Reward:     2.0,
		CreatedAt:  now,
	})
	// 拆掉解析链依赖的 JOIN 表：模拟解析期 DB 故障（表缺失/连接异常同类）
	if err := db.Exec("ALTER TABLE prompt_ab_tests RENAME TO prompt_ab_tests_broken").Error; err != nil {
		t.Fatalf("rename prompt_ab_tests: %v", err)
	}

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	if _, err := r.RefluxOnce(context.Background(), now.Add(-time.Minute), now.Add(time.Minute)); err == nil {
		t.Fatal("解析失败时 RefluxOnce 应返回 error，否则游标推进后该事件永不重试")
	}
	var logs int64
	db.Model(&model.BanditRefluxLog{}).Count(&logs)
	if logs != 0 {
		t.Errorf("解析失败不应写台账, got %d 行", logs)
	}
	if len(mock.calls) != 0 {
		t.Errorf("解析失败不应更新臂, got %d 次", len(mock.calls))
	}
}

// D04: 臂更新失败时补偿删除台账，否则这笔事件会被永久标记为已入账
func TestD04_UpdateFailureRollsBackLedger(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	db.Create(&model.PromptABTest{
		ExperimentID:   "exp-d04-comp",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          66,
		ArmKeys:        model.JSONArray{"A"},
		Status:         "running",
	})
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-comp", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "A", SOPID: 66, Variant: "A"})

	mock := &mockBanditUpdater{failN: 1, failErr: errors.New("db down")}
	r := NewBanditRewardReflux(db, mock)
	now := time.Now()
	db.Create(&model.FeedbackEvent{
		EventID:    "evt-comp-1",
		SessionID:  "sess-comp",
		CustomerID: "c1",
		SOPID:      66,
		SignalKey:  "conversion",
		Reward:     2.0,
		CreatedAt:  now,
	})

	stats, err := r.RefluxOnce(context.Background(), now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 || stats.Refluxed != 0 {
		t.Fatalf("首次更新失败应 failed=1 refluxed=0, got %+v", stats)
	}
	var logs int64
	db.Model(&model.BanditRefluxLog{}).Count(&logs)
	if logs != 0 {
		t.Fatalf("更新失败后台账应被补偿删除, got %d 行", logs)
	}

	stats2, err := r.RefluxOnce(context.Background(), now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats2.Refluxed != 1 {
		t.Errorf("补偿后重扫应成功回流, got %+v", stats2)
	}
	db.Model(&model.BanditRefluxLog{}).Count(&logs)
	if logs != 1 {
		t.Errorf("重扫成功后台账应有 1 行, got %d", logs)
	}
}

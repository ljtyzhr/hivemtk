// Package service 报表服务测试（T3）。
//
// 环境要求：集成用例需导出 POSTGRES_TEST_* 环境变量（无 PG 时跳过）；
// 本文件先覆盖纯逻辑分支（ClassifyError 表驱动、指标计算）。
package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		msg  string
		want string
	}{
		{"", model.ErrClassUnknown},
		{"Request timed out after 30s", model.ErrClassTimeout},
		{"context deadline exceeded", model.ErrClassTimeout},
		{"调用超时", model.ErrClassTimeout},
		{"HTTP 429 Too Many Requests", model.ErrClassRateLimit},
		{"rate limit exceeded", model.ErrClassRateLimit},
		{"401 Unauthorized: invalid api key", model.ErrClassAuth},
		{"上游返回 502 Bad Gateway", model.ErrClassUpstream5xx},
		{"Internal Server Error (500)", model.ErrClassUpstream5xx},
		{"maximum context length exceeded", model.ErrClassContextLength},
		{"该模型上下文 token limit 为 8192", model.ErrClassContextLength},
		{"平台模型余额不足，请购买 LLM 套餐后重试", model.ErrClassInsuffBalance},
		{"empty response from upstream", model.ErrClassEmptyResponse},
		{"完全未知的天书错误", model.ErrClassUnknown},
	}
	for _, c := range cases {
		if got := ClassifyError(c.msg); got != c.want {
			t.Errorf("ClassifyError(%q) = %q, want %q", c.msg, got, c.want)
		}
	}
}

// TestClassifyErrorPriority 优先级：context_length 与 rate_limit 同时命中时取 context_length
// （switch 顺序保证；记录为回归约束）
func TestClassifyErrorPriority(t *testing.T) {
	got := ClassifyError("context length exceeded, rate limit also hit")
	if got != model.ErrClassContextLength {
		t.Fatalf("want context_length priority, got %q", got)
	}
}

// ============================================================
// 集成用例（需 PG 测试库；本地无库自动 skip，CI 有库必须跑）
// ============================================================

// TestAggregateDailyIdempotent 聚合任务幂等：同窗口跑两遍结果一致；upsert 列序与唯一索引一致
func TestAggregateDailyIdempotent(t *testing.T) {
	db := testutil.NewTestDB(t, &model.LLMRoutingLog{}, &model.LLMUsageDaily{})
	addT3Columns(t, db)
	svc := NewReportService(db)

	// 造明细：2 行成功 + 1 行失败（含 error_class 空待归类）
	now := time.Now()
	rows := []map[string]any{
		{"tenant_id": 1, "trace_id": "t1", "scenario": "chat", "provider": "p1", "model": "m1",
			"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150, "cost": 0.02,
			"latency_ms": 800, "success": true, "channel": "web", "source": "platform", "created_at": now},
		{"tenant_id": 1, "trace_id": "t2", "scenario": "chat", "provider": "p1", "model": "m1",
			"prompt_tokens": 200, "completion_tokens": 80, "total_tokens": 280, "cost": 0.03,
			"latency_ms": 1200, "success": true, "channel": "web", "source": "platform", "created_at": now},
		{"tenant_id": 1, "trace_id": "t3", "scenario": "chat", "provider": "p1", "model": "m1",
			"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0, "cost": 0,
			"latency_ms": 5000, "success": false, "error_msg": "Request timed out", "error_class": "timeout",
			"channel": "web", "source": "platform", "created_at": now},
	}
	for _, r := range rows {
		if err := db.Table("llm_routing_logs").Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	ctx := context.Background()
	win := DailyAggWindow{From: now.Add(-time.Hour), To: now.Add(time.Hour)}
	if err := svc.aggregateWindow(ctx, win.From, win.To); err != nil {
		t.Fatalf("aggregate #1: %v", err)
	}
	snap1 := dumpDaily(t, db)
	snap1 = stripUpdatedAt(snap1)
	if err := svc.aggregateWindow(ctx, win.From, win.To); err != nil {
		t.Fatalf("aggregate #2 (idempotent rerun): %v", err)
	}
	snap2 := dumpDaily(t, db)
	snap2 = stripUpdatedAt(snap2)
	if len(snap1) == 0 {
		t.Fatal("聚合后 llm_usage_daily 为空")
	}
	if fmt.Sprint(snap1) != fmt.Sprint(snap2) {
		t.Fatalf("幂等破坏：\n#1=%v\n#2=%v", snap1, snap2)
	}
	logger.Infof("daily buckets: %v", snap1)
}

// TestAggregateErrorClassBackfill 失败行落错误桶（error_class 保留）且成本为 0
func TestAggregateErrorClassBackfill(t *testing.T) {
	db := testutil.NewTestDB(t, &model.LLMRoutingLog{}, &model.LLMUsageDaily{})
	addT3Columns(t, db)
	svc := NewReportService(db)
	now := time.Now()
	if err := db.Table("llm_routing_logs").Create(map[string]any{
		"tenant_id": 1, "trace_id": "t9", "scenario": "chat", "provider": "p1", "model": "m1",
		"latency_ms": 3000, "success": false, "error_msg": "HTTP 429 rate limit", "error_class": "rate_limit",
		"created_at": now,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx := context.Background()
	if err := svc.aggregateWindow(ctx, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	var bucket map[string]any
	if err := db.Table("llm_usage_daily").Where("error_class = ?", "rate_limit").Take(&bucket).Error; err != nil {
		t.Fatalf("error bucket missing: %v", err)
	}
	if n, _ := bucket["error_count"].(int64); n != 1 {
		t.Fatalf("error_count = %v, want 1", bucket["error_count"])
	}
	if c, _ := bucket["total_cost_cents"].(int64); c != 0 {
		t.Fatalf("失败桶 total_cost_cents = %v, want 0", c)
	}
}

// addT3Columns 给 routing_logs 补 T3 新列（模拟 v3.83.0 迁移后的形状；
// model.LLMRoutingLog 尚未声明这两列，AutoMigrate 不会建它们）
func addT3Columns(t *testing.T, db *gorm.DB) {
	t.Helper()
	// 测试库可能残留旧形状的 llm_usage_daily（列名漂移期间建的），重建为当前模型形状
	_ = db.Exec(`DROP TABLE IF EXISTS llm_usage_daily`).Error
	if err := db.AutoMigrate(&model.LLMUsageDaily{}); err != nil {
		t.Fatalf("automigrate llm_usage_daily: %v", err)
	}
	// upsert 冲突目标：显式建复合唯一索引（AutoMigrate 不认迁移里的手写索引）
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uk_llm_usage_daily_dims ON llm_usage_daily
		(tenant_id, stat_date, provider, model, scenario, channel, source, error_class)`).Error; err != nil {
		t.Fatalf("create unique index: %v", err)
	}
	for _, ddl := range []string{
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS channel varchar(30) NOT NULL DEFAULT ''`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS error_class varchar(32) NOT NULL DEFAULT ''`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("addT3Columns: %v", err)
		}
	}
}

// stripUpdatedAt 比对时剔除 updated_at（重跑必然刷新时间戳，语义上不算破坏幂等）
func stripUpdatedAt(rows []map[string]any) []map[string]any {
	for _, r := range rows {
		delete(r, "updated_at")
	}
	return rows
}

func dumpDaily(t *testing.T, db *gorm.DB) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := db.Table("llm_usage_daily").Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("dump daily: %v", err)
	}
	return rows
}

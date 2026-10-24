package llm

// daily_usage_test.go P1-4 回归：/api/llm/usage/daily 必须在真实 PG 上返回 200。
//
// 真缺陷（API 冒烟抓到的）：SELECT 与 GROUP BY 共用同一份带别名的列串，
// PG 不接受 `GROUP BY created_at::date AS stat_date`（42601 syntax error at "AS"），
// 于是该接口自上线起对任何窗口都 500——前端「按日趋势」永远空表。
// 判据：三种 groupBy 都要跑通，且聚合值与插入行对得上。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

func TestQueryDailyUsage_AllGroupBysOnRealDB(t *testing.T) {
	db := testutil.NewTestDB(t, &model.LLMRoutingLog{})
	AttachAuditDB(db)
	t.Cleanup(func() { AttachAuditDB(nil) })

	now := time.Now()
	rows := []model.LLMRoutingLog{
		{Scenario: "sales", Provider: "p1", Model: "m", TotalTokens: 100, Cost: 0.5, LatencyMs: 100, Success: true, CreatedAt: now},
		{Scenario: "sales", Provider: "p1", Model: "m", TotalTokens: 200, Cost: 1.0, LatencyMs: 300, Success: true, CreatedAt: now},
		{Scenario: "sales", Provider: "p2", Model: "m", TotalTokens: 300, Cost: 1.5, LatencyMs: 200, Success: false, CreatedAt: now},
		{Scenario: "support", Provider: "p1", Model: "m", TotalTokens: 400, Cost: 2.0, LatencyMs: 400, Success: true, CreatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("种日志失败: %v", err)
	}
	// success 列有 default:true：结构体零值 false 会被默认值覆盖（本仓第三次踩到），
	// 失败行必须建完显式 UPDATE，否则「失败数」这条腿测不到。
	if err := db.Model(&model.LLMRoutingLog{}).Where("id = ?", rows[2].ID).Update("success", false).Error; err != nil {
		t.Fatalf("置失败标记失败: %v", err)
	}

	for _, groupBy := range []string{"day", "day_scenario", "day_provider"} {
		pts, err := QueryDailyUsage(context.Background(), "month", groupBy, 100)
		if err != nil {
			t.Fatalf("groupBy=%s 查询失败: %v", groupBy, err)
		}
		if len(pts) == 0 {
			t.Fatalf("groupBy=%s 返回空（接口若 500 这里会先红）", groupBy)
		}
		var calls, failed, tokens int64
		for _, p := range pts {
			calls += p.CallCount
			failed += p.FailedCount
			tokens += p.TotalTokens
			if len(p.StatDate) != 10 {
				t.Errorf("groupBy=%s stat_date 应裁成 YYYY-MM-DD，实际 %q", groupBy, p.StatDate)
			}
		}
		if groupBy == "day" {
			if calls != 4 || failed != 1 || tokens != 1000 {
				t.Errorf("day 聚合 = calls %d failed %d tokens %d，期望 4/1/1000", calls, failed, tokens)
			}
		}
	}
}

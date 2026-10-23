package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupLayerLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.LayerDecisionLog{})
	if err := db.Exec("TRUNCATE TABLE layer_decision_logs RESTART IDENTITY").Error; err != nil {
		t.Fatalf("truncate layer_decision_logs: %v", err)
	}
	return db
}

func setupLayerLogRepoWithTX(t *testing.T) (repo *LayerDecisionLogRepository, tx *gorm.DB, cleanup func()) {
	t.Helper()
	db := setupLayerLogTestDB(t)
	tx = db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	repo = NewLayerDecisionLogRepository(tx)
	cleanup = func() { tx.Rollback() }
	return
}

func TestLayerDecisionLogRepository_Record(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	trueVal := true
	log := &model.LayerDecisionLog{
		TraceID:    "trace_1",
		SessionID:  "sess_1",
		CustomerID: "cust_1",
		Layer:      "layer1",
		Reason:     "faq_match",
		Intent:     "logistics",
		ConfIn:     0.6,
		ConfOut:    0.9,
		WallMs:     15,
		LLMSkipped: &trueVal,
	}
	if err := repo.Record(ctx, log); err != nil {
		t.Fatal(err)
	}
	if log.ID == 0 {
		t.Error("expected auto-increment ID")
	}
}

func TestLayerDecisionLogRepository_GetByTraceID(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	logs := []*model.LayerDecisionLog{
		{TraceID: "trace_a", Layer: "layer1", Reason: "faq_match", Intent: "logistics", WallMs: 10},
		{TraceID: "trace_a", Layer: "layer2", Reason: "llm_response", Intent: "logistics", WallMs: 1500},
		{TraceID: "trace_b", Layer: "layer1", Reason: "sop_template", Intent: "pricing", WallMs: 5},
	}
	for _, l := range logs {
		if err := repo.Record(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.GetByTraceID(ctx, "trace_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 logs for trace_a, got %d", len(got))
	}
}

func TestLayerDecisionLogRepository_Recent(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := repo.Record(ctx, &model.LayerDecisionLog{
			Layer: "layer1", Reason: "faq_match", Intent: "logistics",
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Recent(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 recent, got %d", len(got))
	}
}

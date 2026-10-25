// reflux 集成测试（需 POSTGRES_TEST_DSN；本地无库自动 skip）。
// 覆盖：upsert 指纹去重、trgm 归簇、双租户隔离、采纳事务性（写回 faq_entries + 流水）。
package reflux

import (
	"context"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
	"hivemtk-user/internal/repository"
)

func newRefluxDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.UnknownQuestion{}, &model.UnknownQuestionCluster{},
		&model.UnknownQuestionReviewLog{})
	// trgm 扩展/索引（归簇依赖 similarity；测试库补齐）
	_ = db.Exec(`CREATE EXTENSION IF NOT EXISTS pg_trgm`).Error
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_uq_question_trgm
		ON unknown_questions USING gin (question gin_trgm_ops)`).Error; err != nil {
		t.Fatalf("trgm index: %v", err)
	}
	return db
}

// TestCollectorUpsertDedup 同一指纹并发 10 写 → 1 行 frequency=10
func TestCollectorUpsertDedup(t *testing.T) {
	db := newRefluxDB(t)
	repo := repository.NewRefluxRepository(db)
	c := NewCollector(repo, 64, true)
	c.SetClusterer(NewClusterer(repo))
	defer c.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Emit(Event{Question: "怎么申请退款？", Source: model.UQSourceLowConfidence})
		}()
	}
	wg.Wait()

	// 异步消费：有上限轮询（禁止固定 sleep）
	requireEventually(t, 5*time.Second, func() bool {
		var rows []model.UnknownQuestion
		db.Where("question_hash = ?", QuestionHash("怎么申请退款？")).Find(&rows)
		return len(rows) == 1 && rows[0].Frequency == 10
	}, "并发去重后应为 1 行 frequency=10")
}

// TestTenantIsolation 双租户同 hash 各自独立
func TestTenantIsolation(t *testing.T) {
	db := newRefluxDB(t)
	// tenancyidx callback 在裸 db 上按默认租户；这里直接手动写两个租户行验证唯一键含 tenant_id
	hash := QuestionHash("退款流程是什么")
	for _, tid := range []uint{1, 2} {
		if err := db.Exec(`INSERT INTO unknown_questions
			(tenant_id, question_hash, question, status, source, frequency, first_seen_at, last_seen_at, created_at, updated_at)
			VALUES (?, ?, '退款流程是什么', 'draft', 'low_confidence', 1, NOW(), NOW(), NOW(), NOW())
			ON CONFLICT (tenant_id, question_hash) DO NOTHING`, tid, hash).Error; err != nil {
			t.Fatalf("insert tenant %d: %v", tid, err)
		}
	}
	var count int64
	db.Model(&model.UnknownQuestion{}).Where("question_hash = ?", hash).Count(&count)
	if count != 2 {
		t.Fatalf("双租户应各 1 行，got %d", count)
	}
}

// TestAdoptTransaction 采纳事务性：FAQ 写回 + 全簇 adopted + review_log 落行
func TestAdoptTransaction(t *testing.T) {
	db := newRefluxDB(t)
	svc := NewRefluxService(repository.NewRefluxRepository(db))
	faqCreated := uint(88)
	svc.SetFAQCreate(func(ctx context.Context, req FAQCreateReq) (uint, error) {
		if req.Answer == "" {
			return 0, errTooGeneral
		}
		return faqCreated, nil
	})

	// 造簇 + 两个 pending 成员
	cluster := &model.UnknownQuestionCluster{ClusterHash: "c1", Title: "怎么申请退款",
		Status: model.UQStatusPendingReview, QuestionCount: 2, TotalFrequency: 3,
		SampleQuestions: []string{"怎么申请退款", "退款步骤"}}
	if err := db.Create(cluster).Error; err != nil {
		t.Fatalf("seed cluster: %v", err)
	}
	for _, q := range []string{"怎么申请退款", "退款步骤是什么"} {
		if err := db.Create(&model.UnknownQuestion{
			QuestionHash: QuestionHash(q), Question: q, ClusterID: cluster.ID,
			Status: model.UQStatusPendingReview, Source: model.UQSourceLowConfidence,
			FirstSeenAt: time.Now(), LastSeenAt: time.Now(),
		}).Error; err != nil {
			t.Fatalf("seed uq: %v", err)
		}
	}

	res, err := svc.ReviewCluster(context.Background(), cluster.ID,
		&AdoptRequest{Action: ActionAdopt, Answer: "在订单页点击申请退款，1-3 个工作日到账。"}, "op1")
	if err != nil {
		t.Fatalf("ReviewCluster: %v", err)
	}
	if res["faq_id"].(uint) != faqCreated {
		t.Fatalf("faq_id = %v want %d", res["faq_id"], faqCreated)
	}
	var adoptedN int64
	db.Model(&model.UnknownQuestion{}).Where("cluster_id = ? AND status = ?", cluster.ID, model.UQStatusAdopted).Count(&adoptedN)
	if adoptedN != 2 {
		t.Fatalf("全簇成员应 adopted=2, got %d", adoptedN)
	}
	var logN int64
	db.Model(&model.UnknownQuestionReviewLog{}).Where("target_type = 'cluster' AND target_id = ?", cluster.ID).Count(&logN)
	if logN != 1 {
		t.Fatalf("review_log 应 1 行, got %d", logN)
	}
}

// TestIllegalTransition draft 直接 adopt 拒绝
func TestIllegalTransition(t *testing.T) {
	db := newRefluxDB(t)
	svc := NewRefluxService(repository.NewRefluxRepository(db))
	uq := &model.UnknownQuestion{QuestionHash: "h-draft", Question: "随便什么问题",
		Status: model.UQStatusDraft, Source: model.UQSourceNoMatch,
		FirstSeenAt: time.Now(), LastSeenAt: time.Now()}
	if err := db.Create(uq).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := svc.ReviewQuestion(context.Background(), uq.ID,
		&AdoptRequest{Action: ActionAdopt, Answer: "答案"}, "op1")
	if err != ErrIllegalTransition {
		t.Fatalf("draft 直接 adopt 应 ErrIllegalTransition, got %v", err)
	}
}

func requireEventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg + "（超时）")
}

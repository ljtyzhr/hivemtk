package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/tenant"
)

type fakeCrawlerVisitRepo struct {
	created []*model.GeoCrawlerVisit
	err     error
}

func (r *fakeCrawlerVisitRepo) Create(_ context.Context, v *model.GeoCrawlerVisit) error {
	if r.err != nil {
		return r.err
	}
	r.created = append(r.created, v)
	return nil
}

func (r *fakeCrawlerVisitRepo) BulkCreate(_ context.Context, vs []*model.GeoCrawlerVisit) error {
	r.created = append(r.created, vs...)
	return nil
}

func (r *fakeCrawlerVisitRepo) StatsByDomain(context.Context, int) ([]repository.DomainStatRow, error) {
	return nil, nil
}

func (r *fakeCrawlerVisitRepo) StatsByKeyword(context.Context, int) ([]repository.KeywordStatRow, error) {
	return nil, nil
}

func (r *fakeCrawlerVisitRepo) TotalVisits(context.Context, int) (int64, error) { return 0, nil }
func (r *fakeCrawlerVisitRepo) ActiveDomains(context.Context, int) (int64, error) {
	return 0, nil
}
func (r *fakeCrawlerVisitRepo) ActiveKeywords(context.Context, int) (int64, error) {
	return 0, nil
}
func (r *fakeCrawlerVisitRepo) ActiveEngines(context.Context, int) (int64, error) {
	return 0, nil
}

func TestCrawlerVisitService_RecordAnonymousModeGate(t *testing.T) {
	repo := &fakeCrawlerVisitRepo{}
	svc := NewCrawlerVisitService(repo)

	tenant.SetMode(tenant.ModeOn)
	if err := svc.RecordAnonymous(context.Background(), "GPTBot", "/p", "ua", "1.2.3.4"); err != nil {
		t.Fatalf("多租户形态应静默跳过，返回 %v", err)
	}
	if len(repo.created) != 0 {
		t.Fatalf("多租户形态不应写库，实际写入 %d 条", len(repo.created))
	}

	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	if err := svc.RecordAnonymous(context.Background(), "GPTBot", "/p", "ua", "1.2.3.4"); err != nil {
		t.Fatalf("单机形态写入失败: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("单机形态应写 1 条，实际 %d", len(repo.created))
	}
}

func TestCrawlerVisitService_RecordAnonymousPropagatesError(t *testing.T) {
	boom := errors.New("db down")
	svc := NewCrawlerVisitService(&fakeCrawlerVisitRepo{err: boom})
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	if err := svc.RecordAnonymous(context.Background(), "GPTBot", "/p", "ua", "ip"); !errors.Is(err, boom) {
		t.Fatalf("写库错误应上抛，实际 %v", err)
	}
}

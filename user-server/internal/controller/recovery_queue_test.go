package controller

// 挽回队列写口的合法调用腿：用内存仓储驱动真实的 controller + service，
// 不依赖 PG。路由接线与"未带 admin 凭据被拒"在
// internal/router/recovery_queue_routes_test.go 里断言；这里补的是
// "admin 调通之后队列项确实迁移到 succeed"以及非法迁移被 400 挡住。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// fakeRecoveryRepo 只实现本组用例走到的仓储面，语义对齐 repository/customer_rfm.go
// 的白名单写入（MarkRecovered 同时写 stage/recovered_at/recovery_value）。
type fakeRecoveryRepo struct {
	items map[uint64]*model.RecoveryQueue
	next  uint64
	now   time.Time
}

func newFakeRecoveryRepo(stage string) *fakeRecoveryRepo {
	r := &fakeRecoveryRepo{items: map[uint64]*model.RecoveryQueue{}, next: 2, now: time.Now()}
	r.items[1] = &model.RecoveryQueue{
		ID: 1, CustomerID: "c1", Stage: stage, MaxAttempts: 3,
	}
	return r
}

func (f *fakeRecoveryRepo) Create(_ context.Context, item *model.RecoveryQueue) error {
	item.ID = f.next
	f.next++
	f.items[item.ID] = item
	return nil
}

func (f *fakeRecoveryRepo) MarkRecovered(_ context.Context, id uint64, value int64, at time.Time) error {
	item, ok := f.items[id]
	if !ok {
		return errFakeNotFound
	}
	item.Stage = model.RecoveryStageSucceed
	item.RecoveryValue = value
	item.RecoveredAt = &at
	return nil
}

func (f *fakeRecoveryRepo) DeferAttempt(_ context.Context, id uint64, next time.Time) error {
	item, ok := f.items[id]
	if !ok {
		return errFakeNotFound
	}
	item.NextAttemptAt = &next
	return nil
}

func (f *fakeRecoveryRepo) GetByID(_ context.Context, id uint64) (*model.RecoveryQueue, error) {
	item, ok := f.items[id]
	if !ok {
		return nil, errFakeNotFound
	}
	return item, nil
}

func (f *fakeRecoveryRepo) GetActiveByCustomerID(_ context.Context, customerID string) (*model.RecoveryQueue, error) {
	for _, item := range f.items {
		if item.CustomerID == customerID &&
			(item.Stage == model.RecoveryStageQueued || item.Stage == model.RecoveryStageRunning) {
			return item, nil
		}
	}
	return nil, errFakeNotFound
}

func (f *fakeRecoveryRepo) ListByStage(_ context.Context, stage string, _, _ int) ([]*model.RecoveryQueue, int64, error) {
	out := make([]*model.RecoveryQueue, 0, len(f.items))
	for _, item := range f.items {
		if stage == "" || item.Stage == stage {
			out = append(out, item)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeRecoveryRepo) ListReadyForAttempt(_ context.Context, _ time.Time, limit int) ([]*model.RecoveryQueue, error) {
	out := make([]*model.RecoveryQueue, 0, len(f.items))
	for _, item := range f.items {
		if item.Stage == model.RecoveryStageQueued && item.Attempts < item.MaxAttempts {
			out = append(out, item)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRecoveryRepo) MarkAttempt(_ context.Context, id uint64, channel, result string, nextAt *time.Time) error {
	item, ok := f.items[id]
	if !ok {
		return errFakeNotFound
	}
	item.Attempts++
	item.LastChannel = channel
	item.LastResult = result
	item.LastAttemptAt = &f.now
	item.NextAttemptAt = nextAt
	return nil
}

func (f *fakeRecoveryRepo) MarkStage(_ context.Context, id uint64, stage string) error {
	item, ok := f.items[id]
	if !ok {
		return errFakeNotFound
	}
	item.Stage = stage
	return nil
}

func (f *fakeRecoveryRepo) CountByStage(_ context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	for _, item := range f.items {
		out[item.Stage]++
	}
	return out, nil
}

func (f *fakeRecoveryRepo) Delete(_ context.Context, id uint64) error {
	delete(f.items, id)
	return nil
}

var errFakeNotFound = &fakeNotFoundError{}

type fakeNotFoundError struct{}

func (*fakeNotFoundError) Error() string { return "队列项不存在" }

// recoveryQueueCtrlEngine 复刻生产注册形状：身份桩 + 真实 AdminAuthMiddleware，
// 四条写口挂在 /api/recovery-queue 下。
func recoveryQueueCtrlEngine(role string, svc *service.RecoveryQueueService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctrl := &RecoveryQueueController{svc: svc}
	r := gin.New()
	api := r.Group("/api", func(c *gin.Context) {
		if role != "" {
			c.Set("role", role)
		}
		c.Next()
	})
	admin := api.Group("/recovery-queue", middleware.AdminAuthMiddleware())
	admin.POST("/enqueue", ctrl.Enqueue)
	admin.POST("/:id/attempt", ctrl.MarkAttempt)
	admin.POST("/:id/recovered", ctrl.MarkRecovered)
	admin.POST("/:id/cancel", ctrl.Cancel)
	return r
}

func doRecoveryReq(engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestRecoveryQueueController_MarkRecoveredRejectsNonAdmin(t *testing.T) {
	repo := newFakeRecoveryRepo(model.RecoveryStageQueued)
	svc := service.NewRecoveryQueueServiceWithRepo(repo)

	for _, tc := range []struct {
		name, role string
		want       int
	}{
		{"匿名", "", http.StatusUnauthorized},
		{"非管理员", "agent", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRecoveryReq(recoveryQueueCtrlEngine(tc.role, svc),
				"/api/recovery-queue/1/recovered", `{"recovery_value":100}`)
			if rec.Code != tc.want {
				t.Fatalf("期望 %d，实得 %d（body=%s）", tc.want, rec.Code, rec.Body.String())
			}
			if got := repo.items[1].Stage; got != model.RecoveryStageQueued {
				t.Errorf("被拒的请求改了状态：stage=%s", got)
			}
		})
	}
}

func TestRecoveryQueueController_MarkRecoveredWritesSucceed(t *testing.T) {
	repo := newFakeRecoveryRepo(model.RecoveryStageQueued)
	svc := service.NewRecoveryQueueServiceWithRepo(repo)
	engine := recoveryQueueCtrlEngine("admin", svc)

	rec := doRecoveryReq(engine, "/api/recovery-queue/1/recovered", `{"recovery_value":99000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin 标记挽回应 200，实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	item := repo.items[1]
	if item.Stage != model.RecoveryStageSucceed {
		t.Fatalf("stage=%s，期望 %s", item.Stage, model.RecoveryStageSucceed)
	}
	if item.RecoveryValue != 99000 {
		t.Errorf("recovery_value=%d，期望 99000", item.RecoveryValue)
	}
	if item.RecoveredAt == nil {
		t.Error("recovered_at 未写入：succeed 必须与确认时间同笔落库")
	}

	// 幂等重放：不报错、不改首次确认时间。
	first := *item.RecoveredAt
	rec = doRecoveryReq(engine, "/api/recovery-queue/1/recovered", `{"recovery_value":99000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("重复标记应幂等 200，实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	if !item.RecoveredAt.Equal(first) {
		t.Errorf("重复标记改写了 recovered_at：%s → %s", first, *item.RecoveredAt)
	}

	// 已是 succeed 后不许再取消。
	rec = doRecoveryReq(engine, "/api/recovery-queue/1/cancel", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("取消已挽回项应 400，实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	if item.Stage != model.RecoveryStageSucceed {
		t.Errorf("取消被拒后 stage 变了：%s", item.Stage)
	}
}

func TestRecoveryQueueController_MarkRecoveredRejectsCancelled(t *testing.T) {
	repo := newFakeRecoveryRepo(model.RecoveryStageCancelled)
	svc := service.NewRecoveryQueueServiceWithRepo(repo)

	rec := doRecoveryReq(recoveryQueueCtrlEngine("admin", svc),
		"/api/recovery-queue/1/recovered", `{"recovery_value":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cancelled → succeed 应 400，实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	if repo.items[1].Stage != model.RecoveryStageCancelled {
		t.Errorf("非法迁移改动了状态：%s", repo.items[1].Stage)
	}
}

// failed 是"触达次数耗尽"，不等于客户不会自己回来：事后确认挽回必须允许。
func TestRecoveryQueueController_MarkRecoveredFromFailedAllowed(t *testing.T) {
	repo := newFakeRecoveryRepo(model.RecoveryStageFailed)
	svc := service.NewRecoveryQueueServiceWithRepo(repo)

	rec := doRecoveryReq(recoveryQueueCtrlEngine("admin", svc),
		"/api/recovery-queue/1/recovered", `{"recovery_value":500}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed → succeed 应 200，实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	if repo.items[1].Stage != model.RecoveryStageSucceed {
		t.Errorf("stage=%s，期望 succeed", repo.items[1].Stage)
	}
}

func TestRecoveryQueueController_MarkAttemptStageWhitelist(t *testing.T) {
	repo := newFakeRecoveryRepo(model.RecoveryStageQueued)
	svc := service.NewRecoveryQueueServiceWithRepo(repo)
	engine := recoveryQueueCtrlEngine("admin", svc)

	// succeed 必须走 /recovered（要带金额与时间），attempt 口拒收。
	rec := doRecoveryReq(engine, "/api/recovery-queue/1/attempt",
		`{"channel":"sms","result":"replied","stage":"succeed"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("attempt 写 succeed 应 400，实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	if repo.items[1].Stage != model.RecoveryStageQueued {
		t.Errorf("非法 stage 改动了状态：%s", repo.items[1].Stage)
	}

	rec = doRecoveryReq(engine, "/api/recovery-queue/1/attempt",
		`{"channel":"sms","result":"error:timeout","stage":"failed","next_delay_seconds":30}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("合法 attempt 应 200，实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	item := repo.items[1]
	if item.Stage != model.RecoveryStageFailed || item.Attempts != 1 || item.LastChannel != "sms" {
		t.Errorf("attempt 未落库：stage=%s attempts=%d channel=%s", item.Stage, item.Attempts, item.LastChannel)
	}
}

func TestRecoveryQueueController_MarkRecoveredBadJSON(t *testing.T) {
	repo := newFakeRecoveryRepo(model.RecoveryStageQueued)
	svc := service.NewRecoveryQueueServiceWithRepo(repo)

	rec := doRecoveryReq(recoveryQueueCtrlEngine("admin", svc),
		"/api/recovery-queue/1/recovered", `{"recovery_value":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("坏 JSON 应 400（不能静默按 0 落库），实得 %d（body=%s）", rec.Code, rec.Body.String())
	}
	if repo.items[1].Stage != model.RecoveryStageQueued {
		t.Errorf("坏 JSON 改动了状态：%s", repo.items[1].Stage)
	}
}

package service

// 契约锁（实体审查 #12，4.1）：`TaskService.Update` 是「读快照 → mutator → 写回」的读改写，
// 写回的列面已收口到「用户可编辑列」（见 repository/task.go 的 taskOwnedByExecutorCols）。
// 收口之后，返回给界面的那条结构体不能再是读到的快照——快照里的执行侧列此刻可能已经被
// 执行体改写（重试腿就跑在 failed 态上），把旧值交给界面就是「库里 failed/已认领，
// 响应里 ready/仍挂起」。

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	barepo "hivemtk-user/internal/browser_automation/repository"

	"gorm.io/datatypes"
)

// TestTaskUpdateReturnsPersistedExecutorColumns 编辑成功后返回的行必须带库里的执行侧列。
//
// 竞态用 mutator 复现：它就跑在 GetByID 之后、写回之前，正是执行体插进来的那个位置。
func TestTaskUpdateReturnsPersistedExecutorColumns(t *testing.T) {
	taskSvc, _, db, ctx := newCascadeSvcs(t)
	at := time.Now().Add(5 * time.Minute)
	task := &model.BrowserTask{
		Name: "save-columns", TaskType: "one_shot", Status: "failed",
		Url: "https://example.com", UserID: cascadeOwner, Platform: "xiaohongshu", TimeoutSec: 120,
		Steps:             datatypes.JSON(`[{"action":"open_tab"}]`),
		RetryCount:        2,
		NextRetryAt:       &at,
		LastResult:        "steps 3/5 成功",
		CampaignKey:       "camp-12",
		CampaignActBudget: 3,
		CampaignActUsed:   1,
	}
	repo := barepo.NewBrowserTaskRepositoryWithDB(db)
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("种子任务落库失败：%v", err)
	}
	t.Cleanup(func() { _ = db.WithContext(context.Background()).Delete(&model.BrowserTask{}, task.ID) })

	updated, err := taskSvc.Update(ctx, task.ID, cascadeOwner, func(edit *model.BrowserTask) error {
		edit.Name = "改了个名字"
		// 执行体在读到快照之后改写这三列：收口 + 认领掉挂起重试 + 扣一条活动额度。
		if err := repo.UpdateRunResult(ctx, edit.ID, "failed", "steps 4/5 成功", "", 3); err != nil {
			return err
		}
		if err := repo.SetNextRetryAt(ctx, edit.ID, nil); err != nil {
			return err
		}
		if ok, err := repo.TryConsumeCampaignActBudget(ctx, edit.ID); err != nil {
			return err
		} else if !ok {
			return errors.New("预算夹具未扣减")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("编辑失败：%v", err)
	}
	if updated.Name != "改了个名字" {
		t.Fatalf("先决条件坏了：期望返回改名后的行，实得 %q", updated.Name)
	}
	if updated.RetryCount != 3 || updated.NextRetryAt != nil || updated.LastResult != "steps 4/5 成功" ||
		updated.CampaignActUsed != 2 {
		t.Errorf("返回的是编辑前的快照：retry_count=%d next_retry_at=%v last_result=%q used=%d，"+
			"这三列由执行侧窄口写，界面不该看到旧值",
			updated.RetryCount, updated.NextRetryAt, updated.LastResult, updated.CampaignActUsed)
	}
	// 库里的执行侧列也必须还是执行体写的那一份（编辑不许覆回快照）。
	var persisted model.BrowserTask
	if err := db.WithContext(ctx).First(&persisted, task.ID).Error; err != nil {
		t.Fatalf("回读失败：%v", err)
	}
	if persisted.RetryCount != 3 || persisted.NextRetryAt != nil || persisted.CampaignActUsed != 2 {
		t.Errorf("编辑覆回了执行侧的列：retry_count=%d next_retry_at=%v used=%d",
			persisted.RetryCount, persisted.NextRetryAt, persisted.CampaignActUsed)
	}
}

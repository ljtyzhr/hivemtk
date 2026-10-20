package repository

// 契约锁（实体审查 #14，TENANT_MODE=on）：`internal/content` 这一域的持久层**整层没有 ctx**。
//
// 改前实测（用当时的无 ctx 签名直接跑 on 模式，每条都撞闸）：
//   [tenant] 表 ai_generation_records 需要租户上下文但 ctx 未绑定（create）；多租户模式下拒绝执行……
//   [tenant] 表 prompt_templates 需要租户上下文但 ctx 未绑定（query）；……
// 仓储拿的是 `db.GetDB()` 那个无上下文的全局句柄，`Statement.Context` 恒为 nil ⇒ on 模式下
// 这 15 个方法**全部** fail-closed ⇒ saas 形态下「AI 生成记录 + 提示词模板」整域不可用，
// 而 off 模式（本地独立部署）全绿——这正是「两套部署一套代码」只在其中一套成立的那类缺陷。
// 所以本文件先钉租户地基（ctx 必须一路传到底），再按**路由权限宽度**钉两道门：
// `AIGenerationRecord` 有 `user_id`，而改前所有按主键的口只看 `id`——
// 同租户内任何登录用户改自己的 id 就能读/改/删别人的生成记录（内容里还含用户输入的原文）。
// 改后按 `business_routes.go` 的实际挂载分两档：挂 auth 组的用户口（读详情/收藏/评分）必须带归属，
// 挂 admin 组的管理口（save/delete）只按租户收窄——越权写与不存在写都必须报 0 行，不能被回成「操作成功」。
// `prompt_templates` 没有归属列（模板在租户内共享，靠 `is_system` 分档），只钉租户面。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hivemtk-user/internal/content/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	aiTenantA  = uint(43)
	aiTenantB  = uint(67)
	aiUserOwn  = uint(798101) // 记录归属人
	aiUserPeek = uint(798102) // 同租户内的另一个用户
)

func aiTenantDB(t *testing.T) (*gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.AIGenerationRecord{}, &model.PromptTemplate{})
	if db == nil {
		t.Fatal("测试库不可达：租户归属与遮挡全都判不了（不 Skip，跳过等于没锁）")
	}
	return db,
		tenant.WithTenantID(context.Background(), aiTenantA),
		tenant.WithTenantID(context.Background(), aiTenantB)
}

func aiNewRecord(userID uint) *model.AIGenerationRecord {
	return &model.AIGenerationRecord{
		UserID: userID,
		Type:   model.AIGenerationTypeTitle,
		Input:  "原文：改前实测用的一段文本",
		Output: "五个标题",
		Model:  "gpt-test",
	}
}

// aiRecordPhysical 跨租户物理读：判断「到底是谁的行挡住了我」只能用这一条。
func aiRecordPhysical(t *testing.T, db *gorm.DB, id uint) *model.AIGenerationRecord {
	t.Helper()
	var row model.AIGenerationRecord
	err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		t.Fatalf("物理读取 record(%d) 失败: %v", id, err)
	}
	return &row
}

// TestAICreateFillsTenantAndReadsAreTenantFactored 写侧填租户 + 读侧遮挡 + 裸 ctx 报错。
//
// 两个租户刻意用**同一个 user_id**：否则「查不到」既可能是租户谓词生效也可能是归属谓词生效，
// 红的时候分不清是哪道闸立的。同 user 之下只剩租户闸，读不到才是租户遮挡的证据。
func TestAICreateFillsTenantAndReadsAreTenantFactored(t *testing.T) {
	db, ctxA, ctxB := aiTenantDB(t)
	repo := NewAIGenerationRecordRepository(db)

	rec := aiNewRecord(aiUserOwn)
	if err := repo.Create(ctxA, rec); err != nil {
		t.Fatalf("A 的 ctx 下建记录失败: %v", err)
	}
	if row := aiRecordPhysical(t, db, rec.ID); row == nil || row.TenantID != aiTenantA {
		t.Fatalf("记录的租户归属 %+v（期望 tenant_id=%d）：ctx 没传到底，行会落列默认值 1", rec, aiTenantA)
	}

	if got, err := repo.GetByID(ctxA, rec.ID, aiUserOwn); err != nil || got.ID != rec.ID {
		t.Fatalf("A 读自己的记录 got=%+v err=%v", got, err)
	}
	if got, err := repo.GetByID(ctxB, rec.ID, aiUserOwn); err == nil {
		t.Errorf("B 在 A 的租户外读到了记录 %+v：租户遮挡失效", got)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// ctx 已绑定 ⇒ 这条链上「行不在我的租户里」的唯一诚实读数就是查不到；
		// 报成 [tenant] 或别的错误会把它在出口映射成 500，用户看到的是故障而不是「没有这条记录」。
		t.Errorf("B 的跨租户读 err=%v，期望 ErrRecordNotFound（遮挡后查不到）", err)
	}

	// 列表口：同一 user_id 在两个租户各看各的。
	if _, total, err := repo.GetByMerchantAndUser(ctxB, aiUserOwn, 1, 10, nil); err != nil || total != 0 {
		t.Errorf("B 的列表 total=%d err=%v，期望 0（跨租户一条都不该看见）", total, err)
	}
	if _, total, err := repo.GetByMerchantAndUser(ctxA, aiUserOwn, 1, 10, nil); err != nil || total != 1 {
		t.Errorf("A 的列表 total=%d err=%v，期望 1（正向对照：防「谓词太严」造成的假绿）", total, err)
	}

	// 裸 ctx：三个口都必须是闸报错，不能被读成「没有数据」。
	for _, name := range []string{"create", "read", "list"} {
		var err error
		switch name {
		case "create":
			err = repo.Create(context.Background(), aiNewRecord(aiUserOwn))
		case "read":
			_, err = repo.GetByID(context.Background(), rec.ID, aiUserOwn)
		case "list":
			_, _, err = repo.GetByMerchantAndUser(context.Background(), aiUserOwn, 1, 10, nil)
		}
		if err == nil {
			t.Errorf("裸 ctx 的 %s 未被拒绝", name)
			continue
		}
		if !strings.Contains(err.Error(), "[tenant]") {
			t.Errorf("裸 ctx 的 %s 报错但不是租户闸报的（got %v）", name, err)
		}
	}
}

// TestAIRecordUserPortsRequireOwnership 用户口同租户内也不能按 id 动别人的记录（改前 IDOR）。
//
// 只有 `GetByID/UpdateFavorite/UpdateRating` 是用户口（路由挂在 auth 组，任何登录用户可达）；
// `UpdateSaved/Delete` 走 admin 组，由下面的租户口测试钉。
func TestAIRecordUserPortsRequireOwnership(t *testing.T) {
	db, ctxA, _ := aiTenantDB(t)
	repo := NewAIGenerationRecordRepository(db)
	rec := aiNewRecord(aiUserOwn)
	if err := repo.Create(ctxA, rec); err != nil {
		t.Fatalf("建记录失败: %v", err)
	}

	for _, p := range []struct {
		name string
		call func(ctx context.Context, id, userID uint) error
	}{
		{"UpdateFavorite", func(ctx context.Context, id, u uint) error { return repo.UpdateFavorite(ctx, id, u, true) }},
		{"UpdateRating", func(ctx context.Context, id, u uint) error { return repo.UpdateRating(ctx, id, u, 5) }},
	} {
		if err := p.call(ctxA, rec.ID, aiUserPeek); err == nil {
			t.Errorf("%s 以别人的 user_id 竟然写成功：归属门缺失，同租户内可互改内容", p.name)
		} else if !strings.Contains(err.Error(), "不存在") {
			t.Errorf("%s 越权写的错误信息没有「不存在」（got %v）：归属缺失必须报 404 语义，不能报成故障", p.name, err)
		}
	}

	// 遮挡之后，行必须还是原样（写没落地而不是落了一半）。
	row := aiRecordPhysical(t, db, rec.ID)
	if row == nil || row.IsSaved || row.IsFavorite || row.Rating != 0 {
		t.Fatalf("越权写之后物理行 %+v，期望完全未被改动", row)
	}
	if got, err := repo.GetByID(ctxA, rec.ID, aiUserPeek); err == nil {
		t.Errorf("以别人的 user_id 读到了 %+v", got)
	}

	// 正向对照：归属人自己三条口都动得动。
	if err := repo.UpdateFavorite(ctxA, rec.ID, aiUserOwn, true); err != nil {
		t.Errorf("归属人 UpdateFavorite 失败: %v", err)
	}
	if err := repo.UpdateRating(ctxA, rec.ID, aiUserOwn, 4); err != nil {
		t.Errorf("归属人 UpdateRating 失败: %v", err)
	}
	row = aiRecordPhysical(t, db, rec.ID)
	if row == nil || row.IsSaved || !row.IsFavorite || row.Rating != 4 {
		t.Errorf("归属人两次写入后的物理行 %+v，期望 favorite/rating=4 且 saved 未被带动", row)
	}
	if got, err := repo.GetByID(ctxA, rec.ID, aiUserOwn); err != nil || got.ID != rec.ID {
		t.Errorf("归属人读自己的记录 got=%+v err=%v", got, err)
	}
}

// TestAIRecordAdminPortsAreTenantFactored 管理口（save/delete）只按租户收窄。
//
// 这两个口的签名里刻意没有 userID：管理员整理的是本租户的内容库，不该只看自己那几条；
// 代价是边界只剩租户一道，所以必须钉死「跨租户一条都动不了、本租户谁都动得了」。
func TestAIRecordAdminPortsAreTenantFactored(t *testing.T) {
	db, ctxA, ctxB := aiTenantDB(t)
	repo := NewAIGenerationRecordRepository(db)
	rec := aiNewRecord(aiUserOwn)
	if err := repo.Create(ctxA, rec); err != nil {
		t.Fatalf("建记录失败: %v", err)
	}

	if err := repo.UpdateSaved(ctxB, rec.ID, true); err == nil {
		t.Error("B 租户把 A 的记录标成已保存：管理口漏了租户谓词")
	} else if !strings.Contains(err.Error(), "不存在") {
		t.Errorf("跨租户 UpdateSaved 报错 %v，期望「不存在」（404 语义）", err)
	}
	if err := repo.Delete(ctxB, rec.ID); err == nil {
		t.Error("B 租户删掉了 A 的记录")
	} else if !strings.Contains(err.Error(), "不存在") {
		t.Errorf("跨租户 Delete 报错 %v，期望「不存在」（404 语义）", err)
	}
	if row := aiRecordPhysical(t, db, rec.ID); row == nil || row.IsSaved {
		t.Fatalf("跨租户越权写之后物理行 %+v，期望未被改动且仍在", row)
	}

	// 正向对照：同租户内不是归属人（这里是 B 的 ctx 换成 A 的 ctx，用户仍是 aiUserOwn）也动得动。
	if err := repo.UpdateSaved(ctxA, rec.ID, true); err != nil {
		t.Errorf("本租户 UpdateSaved 失败: %v", err)
	}
	if row := aiRecordPhysical(t, db, rec.ID); row == nil || !row.IsSaved {
		t.Errorf("本租户 UpdateSaved 后的物理行 %+v，期望 is_saved=true", row)
	}
	if err := repo.Delete(ctxA, rec.ID); err != nil {
		t.Errorf("本租户 Delete 失败: %v", err)
	}
	if aiRecordPhysical(t, db, rec.ID) != nil {
		t.Error("本租户删除后物理行仍在")
	}
}

// TestAIPortsOnMissingRowReportZeroRows 命中 0 行不得静默成功（#12 同类的写侧静默）。
func TestAIPortsOnMissingRowReportZeroRows(t *testing.T) {
	db, ctxA, _ := aiTenantDB(t)
	repo := NewAIGenerationRecordRepository(db)
	const missing = uint(798999)

	if err := repo.UpdateSaved(ctxA, missing, true); err == nil {
		t.Error("UpdateSaved 命中 0 行静默成功")
	}
	if err := repo.UpdateFavorite(ctxA, missing, aiUserOwn, true); err == nil {
		t.Error("UpdateFavorite 命中 0 行静默成功")
	}
	if err := repo.UpdateRating(ctxA, missing, aiUserOwn, 3); err == nil {
		t.Error("UpdateRating 命中 0 行静默成功")
	}
	if err := repo.Delete(ctxA, missing); err == nil {
		t.Error("Delete 命中 0 行静默成功（调用方随后回「删除成功」）")
	}
	if _, err := repo.GetByID(ctxA, missing, aiUserOwn); err == nil {
		t.Error("GetByID 不存在的 id 返回了 nil error")
	}
}

// TestPromptTemplateWritesAndReadsAreTenantFactored 模板只有租户面这道闸。
func TestPromptTemplateWritesAndReadsAreTenantFactored(t *testing.T) {
	db, ctxA, ctxB := aiTenantDB(t)
	repo := NewPromptTemplateRepository(db)

	tpl := &model.PromptTemplate{Name: "A 的模板", Type: model.AIGenerationTypeCopywriting, Template: "正文 {{product}}", Status: 1}
	if err := repo.Create(ctxA, tpl); err != nil {
		t.Fatalf("A 建模板失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.PromptTemplate{
		Name: "B 的同名模板", Type: model.AIGenerationTypeCopywriting, Template: "另一份正文", Status: 1,
	}); err != nil {
		t.Fatalf("B 建模板失败: %v", err)
	}

	got, err := repo.GetByID(ctxA, tpl.ID)
	if err != nil || got.Name != "A 的模板" {
		t.Fatalf("A 读自己的模板 got=%+v err=%v", got, err)
	}
	if other, err := repo.GetByID(ctxB, tpl.ID); err == nil {
		t.Errorf("B 读到了 A 的模板 %+v", other)
	}

	listA, err := repo.ListByType(ctxA, string(model.AIGenerationTypeCopywriting))
	if err != nil || len(listA) != 1 || listA[0].TenantID != aiTenantA {
		t.Errorf("A 的模板列表 %+v err=%v，期望只有自己的 1 条", listA, err)
	}
	listB, err := repo.ListByType(ctxB, string(model.AIGenerationTypeCopywriting))
	if err != nil || len(listB) != 1 || listB[0].Name != "B 的同名模板" {
		t.Errorf("B 的模板列表 %+v err=%v，期望只有自己的 1 条", listB, err)
	}

	// 同 (type, name) 在不同租户各自存在：这条口不能变成跨租户查找。
	found, err := repo.GetByTypeAndName(ctxB, model.AIGenerationTypeCopywriting, "A 的模板")
	if err == nil {
		t.Errorf("B 按类型+名字查到了 A 的模板 %+v", found)
	}
	if found, err := repo.GetByTypeAndName(ctxA, model.AIGenerationTypeCopywriting, "A 的模板"); err != nil || found.ID != tpl.ID {
		t.Errorf("A 按类型+名字查自己的模板 found=%+v err=%v", found, err)
	}

	// 跨租户 Save：UPDATE 命中 0 行，必须报出来（静默成功就是「界面显示已保存、库里没变」）。
	peek := &model.PromptTemplate{ID: tpl.ID, TenantID: aiTenantB, Name: "被 B 覆写", Type: tpl.Type, Template: tpl.Template}
	if err := repo.Update(ctxB, peek); err == nil {
		t.Error("B 用 A 的主键 Save 成功：跨租户写穿了别人的模板")
	}
	after, err := repo.GetByID(ctxA, tpl.ID)
	if err != nil || after.Name != "A 的模板" {
		t.Errorf("B 的越权 Save 之后 A 的模板变成 %+v err=%v", after, err)
	}
	// 本租户改得动（正向对照）。
	after.Name = "A 改过名"
	if err := repo.Update(ctxA, after); err != nil {
		t.Errorf("A 改自己的模板失败: %v", err)
	}
	if got, err := repo.GetByID(ctxA, tpl.ID); err != nil || got.Name != "A 改过名" {
		t.Errorf("改后回读 got=%+v err=%v", got, err)
	}

	// 删除：B 删不动 A 的，A 删得掉自己的。
	if err := repo.Delete(ctxB, tpl.ID); err == nil {
		t.Error("B 删掉了 A 的模板")
	}
	if err := repo.Delete(ctxA, tpl.ID); err != nil {
		t.Errorf("A 删自己的模板失败: %v", err)
	}
	if err := repo.Create(context.Background(), &model.PromptTemplate{Name: "x", Type: "title", Template: "y"}); err == nil {
		t.Error("裸 ctx 建模板未被拒绝")
	} else if !strings.Contains(err.Error(), "[tenant]") {
		t.Errorf("裸 ctx 建模板报错但不是租户闸报的（got %v）", err)
	}
}

package repository

// 契约锁（实体审查 #19，仓储层）。锁的是话术模板三张表
// （script_templates / script_categories / script_recommendations）在**读写语义**上的
// 底线；租户作用域本身由 controller/script_template_tenant_test.go 在 on 模式锁。
//
// 这一组刻意跑在 off 模式：下面每条缺陷都与租户无关，在两种模式下同形，
// 放在 off 模式才能让「失败」归因到语义本身，而不是归因到隔离闸。
//
// 立项依据（改前实测）：
//   - GetByID 失败时返回 (&零值, err)：调用方一旦漏判 err，拿到的是
//     「ID=0、Title 空」的假模板，而不是「没有这行」。
//   - Update 用 Save()：整行 upsert，把已删除的模板重新插回来；分类同形。
//   - 分页参数不夹紧：page=0 时 Offset 为负（GORM 静默丢掉，不是报错），
//     page_size=0 时发成 LIMIT 0 ⇒ 恒回空列表。
//   - SearchTemplates 的 LIKE 串不转义：%_ 直接透传，搜「%」命中全表。
//   - Count 的错误被丢：计数失败时 total 静默留 0，列表却照常回数据。

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"hivemtk-user/internal/content/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func scrRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.ScriptTemplate{},
		&model.ScriptCategory{},
	)
	if database == nil {
		t.Fatal("测试库不可达（不 Skip，跳过等于没锁）")
	}
	db.SetTestDB(database)
	return database
}

func scrSeedTemplate(t *testing.T, database *gorm.DB, category, title, content string) *model.ScriptTemplate {
	t.Helper()
	row := &model.ScriptTemplate{
		Category: category,
		Name:     strings.ReplaceAll(title, " ", "_"),
		Title:    title,
		Content:  content,
		IsPublic: false,
	}
	if err := database.WithContext(context.Background()).Create(row).Error; err != nil {
		t.Fatalf("预置话术 %s 失败: %v", title, err)
	}
	return row
}

// physicalTemplateCount 物理读回：绕开一切业务口，只看库里到底有没有这行。
func physicalTemplateCount(t *testing.T, database *gorm.DB, id uint) int64 {
	t.Helper()
	var n int64
	if err := database.WithContext(context.Background()).
		Model(&model.ScriptTemplate{}).Where("id = ?", id).Count(&n).Error; err != nil {
		t.Fatalf("物理计数模板 %d 失败: %v", id, err)
	}
	return n
}

func physicalCategoryCount(t *testing.T, database *gorm.DB, id uint) int64 {
	t.Helper()
	var n int64
	if err := database.WithContext(context.Background()).
		Model(&model.ScriptCategory{}).Where("id = ?", id).Count(&n).Error; err != nil {
		t.Fatalf("物理计数分类 %d 失败: %v", id, err)
	}
	return n
}

// TestScriptGetByIDReturnsNilOnError 取不到行时必须交出 nil：
// 跟着 err 一起递回去的零值结构体指针，会被漏判 err 的调用方当成真模板。
func TestScriptGetByIDReturnsNilOnError(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptTemplateRepository(database)

	got, err := repo.GetByID(context.Background(), 999999999)
	if err == nil {
		t.Fatalf("取不存在的模板应报错")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("应交出 ErrRecordNotFound，实得 %v", err)
	}
	if got != nil {
		t.Errorf("err 非空时返回值应为 nil，实得 %+v", got)
	}
}

// TestScriptUpdateOfDeletedRowDoesNotResurrect 按主键更新已删除的模板：
// 只能报「没有这行」，不许把它插回来。Save() 是全字段 upsert，
// 命中 0 行时就走 INSERT——删掉的模板带着原来的 id 复活。
func TestScriptUpdateOfDeletedRowDoesNotResurrect(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptTemplateRepository(database)

	row := scrSeedTemplate(t, database, "sales", "被删掉的报价话术", "先讲价值再谈价格")
	if err := repo.Delete(context.Background(), row.ID); err != nil {
		t.Fatalf("删除模板失败: %v", err)
	}
	if n := physicalTemplateCount(t, database, row.ID); n != 0 {
		t.Fatalf("删除后物理行数应为 0，实得 %d", n)
	}

	row.Title = "改后的标题"
	err := repo.Update(context.Background(), row)
	if err == nil {
		t.Errorf("更新已删除的模板应报错，实得 nil")
	}
	if n := physicalTemplateCount(t, database, row.ID); n != 0 {
		t.Errorf("已删除的模板被写口复活了，物理行数 %d", n)
	}
}

// TestScriptCategoryUpdateOfDeletedRowDoesNotResurrect 分类口同形：
// script_categories 的 Update 也是 Save()。
func TestScriptCategoryUpdateOfDeletedRowDoesNotResurrect(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptCategoryRepository(database)

	row := &model.ScriptCategory{Name: "被删掉的分类", ParentID: 0, SortOrder: 3}
	if err := repo.Create(context.Background(), row); err != nil {
		t.Fatalf("建分类失败: %v", err)
	}
	if err := repo.Delete(context.Background(), row.ID); err != nil {
		t.Fatalf("删分类失败: %v", err)
	}
	if n := physicalCategoryCount(t, database, row.ID); n != 0 {
		t.Fatalf("删除后物理行数应为 0，实得 %d", n)
	}

	row.Name = "改后的分类"
	if err := repo.Update(context.Background(), row); err == nil {
		t.Errorf("更新已删除的分类应报错，实得 nil")
	}
	if n := physicalCategoryCount(t, database, row.ID); n != 0 {
		t.Errorf("已删除的分类被写口复活了，物理行数 %d", n)
	}
}

// TestScriptPagingClampsDegenerateParams 退化分页不许被静默解释：
// page_size=0 发成 LIMIT 0 ⇒ 库里明明有数据却回空列表；
// page=0 让 Offset 变负（GORM 直接丢掉 OFFSET ⇒ 悄悄回到第一页）。
func TestScriptPagingClampsDegenerateParams(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptTemplateRepository(database)
	for i := 0; i < 3; i++ {
		scrSeedTemplate(t, database, "sales", "分页夹具", "内容")
	}

	rows, _, err := repo.GetAll(context.Background(), "sales", 1, 0)
	if err != nil {
		t.Fatalf("page_size=0 查询失败: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("page_size=0 应夹紧到默认页长而不是发成 LIMIT 0 回空列表，应为 3，实得 %d", len(rows))
	}

	rows, _, err = repo.GetAll(context.Background(), "sales", 0, 2)
	if err != nil {
		t.Fatalf("page=0 查询失败: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("page=0 应夹紧到第 1 页并回 2 行，实得 %d", len(rows))
	}
}

// TestScriptSearchEscapesLikeWildcards 关键字里的 LIKE 元字符必须按字面匹配：
// 不转义时搜 "%" 等于搜 "%%%…%"，把全表当成命中。
func TestScriptSearchEscapesLikeWildcards(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptTemplateRepository(database)
	scrSeedTemplate(t, database, "sales", "正常话术甲", "内容里没有百分号")
	scrSeedTemplate(t, database, "sales", "正常话术乙", "内容里没有百分号")

	rows, _, err := repo.SearchTemplates(context.Background(), "%", 1, 10)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("字面 %% 应命中 0 行，实得 %d（LIKE 通配符未转义）", len(rows))
	}
}

// TestScriptSearchEscapesUnderscore 同上，单字符通配符 _ 也必须按字面。
func TestScriptSearchEscapesUnderscore(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptTemplateRepository(database)
	scrSeedTemplate(t, database, "sales", "没有下划线的话术", "内容")

	rows, _, err := repo.SearchTemplates(context.Background(), "_", 1, 10)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("字面 _ 应命中 0 行，实得 %d（LIKE 通配符未转义）", len(rows))
	}
}

// TestScriptCountReflectsFilteredTotal total 是筛选后的全集计数，
// 不是本页行数——前端分页条按 total 算页数，这里少报就是假页码。
func TestScriptCountReflectsFilteredTotal(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptTemplateRepository(database)
	for i := 0; i < 3; i++ {
		scrSeedTemplate(t, database, "counting", "计数夹具", "内容")
	}
	scrSeedTemplate(t, database, "other", "别的分类", "内容")

	rows, total, err := repo.GetAll(context.Background(), "counting", 1, 2)
	if err != nil {
		t.Fatalf("列表查询失败: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("本页应回 2 行，实得 %d", len(rows))
	}
	if total != 3 {
		t.Errorf("total 应为筛选后的全集 3，实得 %d", total)
	}
}

// TestScriptIncrementUsageIsAtomic 计次必须是库内原子自增：
// 读改写在并发下丢更新（40 次点用只留 2 次）。
func TestScriptIncrementUsageIsAtomic(t *testing.T) {
	database := scrRepoDB(t)
	repo := NewScriptTemplateRepository(database)
	row := scrSeedTemplate(t, database, "sales", "并发计次话术", "内容")

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := repo.IncrementUsage(context.Background(), row.ID); err != nil {
				t.Errorf("计次失败: %v", err)
			}
		}()
	}
	wg.Wait()

	var got model.ScriptTemplate
	if err := database.WithContext(context.Background()).First(&got, row.ID).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if got.UsageCount != 40 {
		t.Errorf("应为 40，实得 %d（读改写丢更新）", got.UsageCount)
	}
}

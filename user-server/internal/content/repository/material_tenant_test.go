package repository

// 契约锁（实体审查 #18，仓储写腿 / off 模式）。
//
// 两处 Update 改前都是 r.db.Save(整行)。Save 在 GORM v2 里走
// INSERT … ON CONFLICT (id) DO UPDATE：主键命中不到行时不是「更新 0 行」，
// 而是按结构体快照把**整行重新插入**——读改写之间素材/分类被别处删掉了，
// 一次「更新」就把已删的行连同它的 license_id 一起复活；行属于别租户时，
// 同一条 upsert 会把它的归属列并进本租户。
//
// ⚠️ 别指望隔离闸拦这条：闸只拒「DO UPDATE 把 tenant_id 改成别的值」，Save 生成的
// 恰是 tenant_id = excluded.tenant_id，在放行名单里（变异 M2 实测：on 模式下出口层锁全绿）。
// 本文件锁在 off 模式，是因为那里没有服务层的租户读前置替它兜住，缺陷形状看得最清；
// on 模式同一形状由出口层的「别租户 404 + 本租户 200」锁与仓储的 RowsAffected 哨兵共同钉住。
//
// 本文件的调用形状原样保留 RED 时的断言，GREEN 只机械补 ctx 实参，并把
// 「err == nil 即失败」升级成「必须是那条缺失哨兵」——防止用任意错误蒙过哨兵位。

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/content/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func matRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t, &model.Material{}, &model.MaterialCategory{})
	db.SetTestDB(database)
	return database
}

// TestMaterialUpdateDoesNotResurrectDeletedRow 已删素材的行不许被一次更新写回来。
func TestMaterialUpdateDoesNotResurrectDeletedRow(t *testing.T) {
	database := matRepoDB(t)
	repo := NewMaterialRepository(database)

	row := &model.Material{
		Name:      "待删素材",
		Type:      model.MaterialTypeImage,
		URL:       "https://cdn.example.com/a.png",
		Size:      1024,
		LicenseID: "lic-a",
		Status:    "active",
	}
	if err := repo.Create(context.Background(), row); err != nil {
		t.Fatalf("建素材失败: %v", err)
	}
	if err := repo.Delete(context.Background(), row.ID); err != nil {
		t.Fatalf("删素材失败: %v", err)
	}

	// 拿删除前读到的那份快照再更新一次（读改写的现场）。
	stale := *row
	stale.Name = "被复活的素材"
	stale.LicenseID = "lic-other"

	if err := repo.Update(context.Background(), &stale); !errors.Is(err, ErrMaterialMissing) {
		t.Errorf("更新已删除的素材应交出缺失错误（ErrMaterialMissing），实得 %v", err)
	}

	var n int64
	if err := database.Model(&model.Material{}).Where("id = ?", row.ID).Count(&n).Error; err != nil {
		t.Fatalf("物理计数失败: %v", err)
	}
	if n != 0 {
		t.Errorf("Update 失败后不应把已删除的素材写回库，实得 %d 行（Save 的 upsert 复活了它）", n)
	}
}

// TestMaterialCategoryUpdateDoesNotResurrectDeletedRow 分类同理。
func TestMaterialCategoryUpdateDoesNotResurrectDeletedRow(t *testing.T) {
	database := matRepoDB(t)
	repo := NewMaterialCategoryRepository(database)

	row := &model.MaterialCategory{
		Name:      "待删分类",
		Type:      model.MaterialTypeImage,
		LicenseID: "lic-a",
		Status:    "active",
	}
	if err := repo.Create(context.Background(), row); err != nil {
		t.Fatalf("建分类失败: %v", err)
	}
	if err := repo.Delete(context.Background(), row.ID); err != nil {
		t.Fatalf("删分类失败: %v", err)
	}

	stale := *row
	stale.Name = "被复活的分类"
	stale.LicenseID = "lic-other"

	if err := repo.Update(context.Background(), &stale); !errors.Is(err, ErrMaterialCategoryMissing) {
		t.Errorf("更新已删除的分类应交出缺失错误（ErrMaterialCategoryMissing），实得 %v", err)
	}

	var n int64
	if err := database.Model(&model.MaterialCategory{}).Where("id = ?", row.ID).Count(&n).Error; err != nil {
		t.Fatalf("物理计数失败: %v", err)
	}
	if n != 0 {
		t.Errorf("Update 失败后不应把已删除的分类写回库，实得 %d 行", n)
	}
}

// TestMaterialDeleteMissingIDIsError 删除命中 0 行不能算成功：
// 改前 Delete 只看 Error，行不存在时回 nil，出口层报「删除成功」。
func TestMaterialDeleteMissingIDIsError(t *testing.T) {
	repo := NewMaterialRepository(matRepoDB(t))

	if err := repo.Delete(context.Background(), "no-such-material"); !errors.Is(err, ErrMaterialMissing) {
		t.Errorf("删除不存在的素材应交出缺失错误（ErrMaterialMissing），实得 %v", err)
	}
}

// TestMaterialIncrementUsageIsAtomic 计次必须是库内自增：
// 读改写 + 整行写回会丢更新，也会把并发改过的别的列顶回陈旧快照。
func TestMaterialIncrementUsageIsAtomic(t *testing.T) {
	database := matRepoDB(t)
	repo := NewMaterialRepository(database)

	row := &model.Material{
		Name:      "计次素材",
		Type:      model.MaterialTypeImage,
		URL:       "https://cdn.example.com/a.png",
		Size:      1024,
		LicenseID: "lic-a",
		Status:    "active",
	}
	if err := repo.Create(context.Background(), row); err != nil {
		t.Fatalf("建素材失败: %v", err)
	}

	// 别处先把体积改了；自增只动 usage_count / last_used_at 两列。
	if err := database.Model(&model.Material{}).Where("id = ?", row.ID).
		Update("size", 4096).Error; err != nil {
		t.Fatalf("外部改体积失败: %v", err)
	}

	const bumps = 30
	errCh := make(chan error, bumps)
	for i := 0; i < bumps; i++ {
		go func() { errCh <- repo.IncrementUsage(context.Background(), row.ID) }()
	}
	for i := 0; i < bumps; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("第 %d 次自增失败: %v", i, err)
		}
	}

	var got model.Material
	if err := database.Where("id = ?", row.ID).First(&got).Error; err != nil {
		t.Fatalf("物理读取失败: %v", err)
	}
	if got.UsageCount != bumps {
		t.Errorf("并发 %d 次自增后 usage_count 应为 %d，实得 %d", bumps, bumps, got.UsageCount)
	}
	if got.Size != 4096 {
		t.Errorf("自增把外部改过的 size 顶回了旧快照值：%+v", got)
	}
	if got.LastUsedAt == nil {
		t.Errorf("自增没落 last_used_at")
	}

	// 命中不到行的自增不能静默成功。
	if err := repo.IncrementUsage(context.Background(), "no-such-material"); !errors.Is(err, ErrMaterialMissing) {
		t.Errorf("对不存在的素材计次应交出缺失错误（ErrMaterialMissing），实得 %v", err)
	}
}

// TestMaterialGetByIDReturnsNilOnError 取不到行时不能回一个零值结构体指针：
// 调用方一旦漏判 error，拿到的就是「ID 为空、URL 为空」的假素材。
func TestMaterialGetByIDReturnsNilOnError(t *testing.T) {
	repo := NewMaterialRepository(matRepoDB(t))

	got, err := repo.GetByID(context.Background(), "no-such-id")
	if err == nil {
		t.Fatalf("取不存在的素材应报错")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("应交出 ErrRecordNotFound，实得 %v", err)
	}
	if got != nil {
		t.Errorf("err 非空时返回值应为 nil，实得 %+v", got)
	}
}

// TestMaterialCategoryGetTreeReturnsRootsOnly 分类树只回根节点（ParentID 为空的那些行）。
// 这条是既有口径的锁：改前 Preload("Children") 只展开一层，三层树会丢第三层，
// 本锁只钉「根集合」这一层，深度缺陷记在 model-list.md 的观察项里。
func TestMaterialCategoryGetTreeReturnsRootsOnly(t *testing.T) {
	database := matRepoDB(t)
	repo := NewMaterialCategoryRepository(database)

	root := &model.MaterialCategory{Name: "根", Type: model.MaterialTypeImage, LicenseID: "lic-a", Status: "active"}
	if err := repo.Create(context.Background(), root); err != nil {
		t.Fatalf("建根分类失败: %v", err)
	}
	child := &model.MaterialCategory{Name: "子", Type: model.MaterialTypeImage, LicenseID: "lic-a", ParentID: &root.ID, Status: "active"}
	if err := repo.Create(context.Background(), child); err != nil {
		t.Fatalf("建子分类失败: %v", err)
	}

	roots, err := repo.GetTree(context.Background(), "lic-a", "")
	if err != nil {
		t.Fatalf("取分类树失败: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("分类树应只回 1 个根，实得 %d", len(roots))
	}
	if roots[0].Name != "根" {
		t.Errorf("根节点名字不对：%+v", roots[0])
	}
}

// TestMaterialCategoryDeleteNotEmptySentinel 「分类非空」必须是可分类的业务哨兵，
// 而不是混在外键错误/缺失错误里：服务层据此翻译成 400，别的一律照常上抛。
func TestMaterialCategoryDeleteNotEmptySentinel(t *testing.T) {
	database := matRepoDB(t)
	repo := NewMaterialCategoryRepository(database)
	materials := NewMaterialRepository(database)

	parent := &model.MaterialCategory{Name: "有子分类", Type: model.MaterialTypeImage, LicenseID: "lic-a", Status: "active"}
	if err := repo.Create(context.Background(), parent); err != nil {
		t.Fatalf("建父分类失败: %v", err)
	}
	if err := repo.Create(context.Background(), &model.MaterialCategory{
		Name: "孙分类", Type: model.MaterialTypeImage, LicenseID: "lic-a", ParentID: &parent.ID, Status: "active",
	}); err != nil {
		t.Fatalf("建子分类失败: %v", err)
	}
	if err := materials.Create(context.Background(), &model.Material{
		Name: "挂在父分类下", Type: model.MaterialTypeImage, CategoryID: parent.ID,
		URL: "https://cdn.example.com/a.png", Size: 1024, LicenseID: "lic-a", Status: "active",
	}); err != nil {
		t.Fatalf("建素材失败: %v", err)
	}

	if err := repo.Delete(context.Background(), parent.ID); !errors.Is(err, ErrMaterialCategoryNotEmpty) {
		t.Errorf("删非空分类应交出 ErrMaterialCategoryNotEmpty，实得 %v", err)
	}
	var n int64
	if err := database.Model(&model.MaterialCategory{}).Where("id = ?", parent.ID).Count(&n).Error; err != nil {
		t.Fatalf("物理计数失败: %v", err)
	}
	if n != 1 {
		t.Errorf("非空分类不该被删掉，实得 %d 行", n)
	}
	if err := repo.Delete(context.Background(), "no-such-category"); !errors.Is(err, ErrMaterialCategoryMissing) {
		t.Errorf("删不存在的分类应交出 ErrMaterialCategoryMissing，实得 %v", err)
	}
}

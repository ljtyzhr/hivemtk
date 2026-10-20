package repository

// 契约锁（实体审查 #16，TENANT_MODE=on）：`market_templates` / `market_template_downloads`
// 都是租户表（各有 tenant_id 列），而改前这两个仓储整面都不带 ctx——全仓实测原文：
//   裸句柄 GetList: total=0 err=[tenant] 表 market_templates 需要租户上下文但 ctx 未绑定（query）；
//     多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
//   裸句柄 GetByID: 返回指针是否非nil=true err=[tenant] 表 market_templates 需要租户上下文…（query）
//   裸句柄 GetAll 下载记录: len=0 total=0 err=[tenant] 表 market_template_downloads 需要租户上下文…（query）
// ⇒ saas 形态下模板市场 8 个 handler（17 条挂载）全废：列表/详情/官方/搜索/我的下载/建模板/下载/评分，
//   off 形态（本地独立部署）全绿——两形态行为分裂，不是「测试环境问题」。
// 另有两条独立缺陷，一起锁住：
//   `GetByID` 失败时仍回**非 nil 结构体**，调用方无从分辨「记录不存在」与「配置级故障」；
//   `IncrementDownload` / `UpdateRating` 命中 0 行（跨租户 id 被租户谓词挡掉就是 0 行）静默返回 nil，
//     把「没写成」报成「写成」。
// 下载记录改前没有归属列，`/templates/my-downloads` 实际回的是**本租户全员**的下载流水（跨用户可见）。

import (
	"context"
	"errors"
	"strings"
	"testing"

	contentmodel "hivemtk-user/internal/content/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	mtTenantA = uint(91)
	mtTenantB = uint(92)
	mtUserA   = uint(9001)
	mtUserB   = uint(9002)
)

type mtFixture struct {
	gdb       *gorm.DB
	templates *MarketTemplateRepository
	downloads *MarketTemplateDownloadRepository
	ctxA      context.Context
	ctxB      context.Context
}

func mtDB(t *testing.T) *mtFixture {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	gdb := testutil.NewTestDB(t, &contentmodel.MarketTemplate{}, &contentmodel.MarketTemplateDownload{})
	if gdb == nil {
		t.Fatal("测试库不可达：租户遮挡与归属判定全都判不了（不 Skip，跳过等于没锁）")
	}
	return &mtFixture{
		gdb:       gdb,
		templates: NewMarketTemplateRepository(gdb),
		downloads: NewMarketTemplateDownloadRepository(gdb),
		ctxA:      tenant.WithTenantID(context.Background(), mtTenantA),
		ctxB:      tenant.WithTenantID(context.Background(), mtTenantB),
	}
}

// mtSeedTemplate 用 SystemScope 直插，好让用例自己决定这行属于哪个租户。
func mtSeedTemplate(t *testing.T, f *mtFixture, tenantID uint, name, keyword string, official bool) *contentmodel.MarketTemplate {
	t.Helper()
	row := &contentmodel.MarketTemplate{
		TenantID:    tenantID,
		Name:        name,
		Description: keyword,
		Category:    "dashboard",
		Type:        "flow",
		IsOfficial:  official,
	}
	if err := f.gdb.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置 %d 租户模板 %s 失败: %v", tenantID, name, err)
	}
	return row
}

func mtPhysicalTemplate(t *testing.T, f *mtFixture, id uint) *contentmodel.MarketTemplate {
	t.Helper()
	var row contentmodel.MarketTemplate
	err := f.gdb.WithContext(tenant.WithSystemScope(context.Background())).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		t.Fatalf("物理读取模板(%d) 失败: %v", id, err)
	}
	return &row
}

// TestMarketTemplateReadsAreTenantFactored 三条读链必须按调用方租户裁剪，
// 且「查不到」与「ctx 没绑定」必须是两种可分辨的失败。
func TestMarketTemplateReadsAreTenantFactored(t *testing.T) {
	f := mtDB(t)

	a1 := mtSeedTemplate(t, f, mtTenantA, "销售大盘", "sales", true)
	a2 := mtSeedTemplate(t, f, mtTenantA, "库存大盘", "stock", false)
	b1 := mtSeedTemplate(t, f, mtTenantB, "对手大盘", "sales", true)

	list, total, err := f.templates.GetList(f.ctxA, "", "", 1, 10)
	if err != nil {
		t.Fatalf("A 租户列表读取失败: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Errorf("A 租户只应看到自己的 2 条，实得 len=%d total=%d", len(list), total)
	}

	official, cnt, err := f.templates.GetOfficialTemplates(f.ctxA, 1, 10)
	if err != nil {
		t.Fatalf("A 租户官方模板读取失败: %v", err)
	}
	if cnt != 1 || len(official) != 1 || official[0].ID != a1.ID {
		t.Errorf("A 租户官方模板只应是自己那条，实得 len=%d total=%d", len(official), cnt)
	}

	hits, _, err := f.templates.SearchTemplates(f.ctxA, "sales", 1, 10)
	if err != nil {
		t.Fatalf("A 租户搜索失败: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != a1.ID {
		t.Errorf("A 租户搜 sales 只应命中自己 1 条（B 的同名模板不可见），实得 len=%d", len(hits))
	}

	// 别租户详情：物理存在，但 A 读不到，必须如实报 ErrRecordNotFound 且不回结构体
	got, err := f.templates.GetByID(f.ctxA, b1.ID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("读别租户模板应报 ErrRecordNotFound，实得 %v", err)
	}
	if got != nil {
		t.Errorf("读失败仍回了模板结构体 %+v，调用方无从判断真假", got)
	}

	// 本租户详情正常
	own, err := f.templates.GetByID(f.ctxA, a2.ID)
	if err != nil || own == nil || own.Name != "库存大盘" {
		t.Errorf("A 租户读本租户模板失败: got=%+v err=%v", own, err)
	}

	// 裸 ctx：配置级故障，不能伪装成「不存在」
	_, _, err = f.templates.GetList(context.Background(), "", "", 1, 10)
	if err == nil || !strings.Contains(err.Error(), "需要租户上下文") {
		t.Errorf("未绑定 ctx 的列表读取应报租户闸错，实得 %v", err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("未绑定 ctx 被折叠成 ErrRecordNotFound，等于对上层谎报「没有数据」")
	}
	got, err = f.templates.GetByID(context.Background(), a1.ID)
	if err == nil || !strings.Contains(err.Error(), "需要租户上下文") || got != nil {
		t.Errorf("未绑定 ctx 的详情读取应报租户闸错且不回结构体，实得 got=%+v err=%v", got, err)
	}
	_, cnt, err = f.templates.SearchTemplates(context.Background(), "sales", 1, 10)
	if err == nil || !strings.Contains(err.Error(), "需要租户上下文") || cnt != 0 {
		t.Errorf("未绑定 ctx 的搜索应报租户闸错、total 归零，实得 total=%d err=%v", cnt, err)
	}
}

// TestMarketTemplateWritesAreTenantFactored 写面：计数与评分都必须挡住别租户的行。
func TestMarketTemplateWritesAreTenantFactored(t *testing.T) {
	f := mtDB(t)

	a1 := mtSeedTemplate(t, f, mtTenantA, "本家模板", "x", false)
	b1 := mtSeedTemplate(t, f, mtTenantB, "别家模板", "x", false)

	// 建模板：租户由调用方 ctx 决定，不能落成默认 1
	created := &contentmodel.MarketTemplate{Name: "新建模板", Type: "flow"}
	if err := f.templates.Create(f.ctxA, created); err != nil {
		t.Fatalf("A 租户建模板失败: %v", err)
	}
	if created.TenantID != mtTenantA {
		t.Errorf("A 租户建的模板应属 A，实得 tenant_id=%d", created.TenantID)
	}

	// 下载计数：本租户可加，别租户必须报错且物理行不变
	if err := f.templates.IncrementDownload(f.ctxA, a1.ID); err != nil {
		t.Errorf("A 租户给自己的模板加下载量失败: %v", err)
	}
	if ph := mtPhysicalTemplate(t, f, a1.ID); ph.DownloadCount != 1 {
		t.Errorf("A 模板下载量应为 1，实得 %d", ph.DownloadCount)
	}
	if err := f.templates.IncrementDownload(f.ctxA, b1.ID); err == nil {
		t.Errorf("给别租户模板加下载量应失败（命中 0 行不是成功）")
	}
	if err := f.templates.IncrementDownload(f.ctxB, b1.ID); err != nil {
		t.Errorf("B 租户给自己的模板加下载量失败: %v", err)
	}
	if ph := mtPhysicalTemplate(t, f, b1.ID); ph.DownloadCount != 1 {
		t.Errorf("B 模板下载量应为 1（A 的那次必须没写进去），实得 %d", ph.DownloadCount)
	}

	// 评分：同上
	if err := f.templates.UpdateRating(f.ctxA, a1.ID, 4); err != nil {
		t.Errorf("A 租户给自己模板评分失败: %v", err)
	}
	if err := f.templates.UpdateRating(f.ctxA, b1.ID, 5); err == nil {
		t.Errorf("给别租户模板评分应失败")
	}
	if ph := mtPhysicalTemplate(t, f, b1.ID); ph.Rating != 0 {
		t.Errorf("别租户模板评分被改写，实得 %v", ph.Rating)
	}
	if err := f.templates.UpdateRating(context.Background(), a1.ID, 5); err == nil ||
		!strings.Contains(err.Error(), "需要租户上下文") {
		t.Errorf("未绑定 ctx 的评分应报租户闸错，实得 %v", err)
	}
}

// TestMarketTemplateDownloadOwnership 「我的下载」必须按用户裁剪，不能回全租户流水。
func TestMarketTemplateDownloadOwnership(t *testing.T) {
	f := mtDB(t)

	for _, rec := range []*contentmodel.MarketTemplateDownload{
		{TenantID: mtTenantA, UserID: mtUserA, TemplateID: 1, TemplateType: "flow"},
		{TenantID: mtTenantA, UserID: mtUserB, TemplateID: 2, TemplateType: "flow"},
		{TenantID: mtTenantB, UserID: mtUserB, TemplateID: 3, TemplateType: "report"},
	} {
		if err := f.gdb.WithContext(tenant.WithSystemScope(context.Background())).Create(rec).Error; err != nil {
			t.Fatalf("预置下载记录失败: %v", err)
		}
	}

	mine, total, err := f.downloads.GetByUser(f.ctxA, mtUserA, 1, 10)
	if err != nil {
		t.Fatalf("A/%d 的下载流水读取失败: %v", mtUserA, err)
	}
	if total != 1 || len(mine) != 1 || mine[0].TemplateID != 1 {
		t.Errorf("「我的下载」只应是自己那 1 条，实得 len=%d total=%d", len(mine), total)
	}

	// 同租户另一个用户看不到 mtUserA 的流水
	other, _, err := f.downloads.GetByUser(f.ctxA, mtUserB, 1, 10)
	if err != nil {
		t.Fatalf("A/%d 的下载流水读取失败: %v", mtUserB, err)
	}
	if len(other) != 1 || other[0].TemplateID != 2 {
		t.Errorf("同租户他人只应看到自己的流水，实得 len=%d", len(other))
	}

	// 跨租户同样遮挡
	_, cnt, err := f.downloads.GetByUser(f.ctxB, mtUserA, 1, 10)
	if err != nil {
		t.Fatalf("B 租户读取失败: %v", err)
	}
	if cnt != 0 {
		t.Errorf("B 租户不应看到 A 用户任何流水，实得 total=%d", cnt)
	}

	if err := f.downloads.Create(f.ctxA, &contentmodel.MarketTemplateDownload{UserID: mtUserA, TemplateID: 9, TemplateType: "flow"}); err != nil {
		t.Fatalf("写入下载流水失败: %v", err)
	}
	if err := f.downloads.Create(context.Background(), &contentmodel.MarketTemplateDownload{UserID: mtUserA, TemplateID: 10}); err == nil ||
		!strings.Contains(err.Error(), "需要租户上下文") {
		t.Errorf("未绑定 ctx 写下载流水应报租户闸错，实得 %v", err)
	}
}

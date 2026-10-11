package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// l1WindowFixture 一次把两件事备齐：参数中心的库（读 memory.l1_window_size）
// 与记忆库（验证窗口真的作用到 L1List / l1Trim / BuildFullContext）。
func l1WindowFixture(t *testing.T) (*gorm.DB, *MemorySystem) {
	t.Helper()
	db := testutil.NewTestDB(t,
		&model.MemoryItem{},
		&model.SOPStateMemory{},
		&model.BusinessMemory{},
		&model.ConfigParam{},
		&model.ConfigParamAuditLog{},
	)
	if db == nil {
		t.Skip("no DB")
	}
	ctx := context.Background()
	if err := SeedConfigParams(ctx, db); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	SetGlobalForTest(NewConfigParamService(db))
	t.Cleanup(func() { SetGlobalForTest(nil) })
	return db, &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
}

// setL1Window 直接改库而不是走 service.UpdateValue：service 层有 min/max 守卫，
// 造不出 0/负数这类坏值。守卫要防的恰是「库里已经有坏值」（历史数据 / 迁移写入 /
// 绕过 service 直连改库），所以这类值只能在测试里绕过它写进去。
func setL1Window(t *testing.T, db *gorm.DB, raw string) {
	t.Helper()
	if err := repository.NewConfigParamRepository(db).
		UpdateValue(context.Background(), ParamGroupMemory, MemoryL1WindowSizeParam, raw, 1); err != nil {
		t.Fatalf("直写 memory.l1_window_size=%s 失败：%v", raw, err)
	}
	// 参数服务按 group.key 缓存 60s，换一个冷缓存实例才能立刻读到新值。
	SetGlobalForTest(NewConfigParamService(db))
}

func TestRuntimeL1WindowSizeFallsBackToConst(t *testing.T) {
	SetGlobalForTest(nil)
	if got := runtimeL1WindowSize(context.Background()); got != L1WindowSize {
		t.Errorf("无参数中心时应回落编译期常量 %d，实际 %d", L1WindowSize, got)
	}
}

func TestRuntimeL1WindowSizeReadsParamCenter(t *testing.T) {
	db, _ := l1WindowFixture(t)
	if got := runtimeL1WindowSize(context.Background()); got != L1WindowSize {
		t.Fatalf("种子默认值应等于代码兜底 %d，实际 %d", L1WindowSize, got)
	}
	setL1Window(t, db, "5")
	if got := runtimeL1WindowSize(context.Background()); got != 5 {
		t.Errorf("参数改为 5 后应读到 5，实际 %d", got)
	}
}

func TestRuntimeL1WindowSizeRejectsNonPositive(t *testing.T) {
	db, _ := l1WindowFixture(t)
	// 0 是最危险的一种：窗口 0 会让 l1Trim 把整个会话的短期记忆删光。
	for _, bad := range []string{"0", "-3"} {
		setL1Window(t, db, bad)
		if got := runtimeL1WindowSize(context.Background()); got != L1WindowSize {
			t.Errorf("坏值 %s 应回落 %d，实际 %d", bad, L1WindowSize, got)
		}
	}
}

func TestL1TrimRespectsConfiguredWindow(t *testing.T) {
	db, m := l1WindowFixture(t)
	setL1Window(t, db, "3")
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		if err := m.L1Append(ctx, "s-trim", "c-1", "user", "msg-"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	var count int64
	if err := db.Model(&model.MemoryItem{}).
		Where("layer = ?", model.MemoryLayerShortTerm).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("窗口配 3、写入 8 条后应只剩 3 条，实际 %d", count)
	}
}

func TestL1ListZeroLimitFollowsConfiguredWindow(t *testing.T) {
	db, m := l1WindowFixture(t)
	setL1Window(t, db, "4")
	ctx := context.Background()
	for i := 0; i < 7; i++ {
		if err := m.L1Append(ctx, "s-list", "c-1", "user", "msg-"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	items, err := m.L1List(ctx, "s-list", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("limit=0 应回落到配置窗口 4，实际取回 %d 条", len(items))
	}
}

// TestBuildFullContextUsesConfiguredWindow 钉住 G4 补接时漏掉的一处：
// BuildFullContext 是 LLM 提示词的生产构造入口，它原先直接传 L1WindowSize
// 常量，运维调了 memory.l1_window_size 在这里不生效——L1List 与 l1Trim
// 都读参数、唯独提示词不读，这种半接线最难被发现。
func TestBuildFullContextUsesConfiguredWindow(t *testing.T) {
	db, m := l1WindowFixture(t)
	setL1Window(t, db, "3")
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		if err := m.L1Append(ctx, "s-ctx", "c-1", "user", fmt.Sprintf("第%d条", i)); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	out, err := m.BuildFullContext(ctx, "s-ctx", "")
	if err != nil {
		t.Fatalf("BuildFullContext: %v", err)
	}
	if got := strings.Count(out, "客户: "); got != 3 {
		t.Errorf("提示词里应只带最近 3 条短期记忆，实际带了 %d 条：\n%s", got, out)
	}
}

func TestMemoryL1WindowSizeSeedMatchesFallback(t *testing.T) {
	var found bool
	for _, d := range DefaultParamDefs() {
		if d.Group != ParamGroupMemory || d.Key != MemoryL1WindowSizeParam {
			continue
		}
		found = true
		if d.DefaultValue != fmt.Sprintf("%d", L1WindowSize) {
			t.Errorf("种子默认值 %q 应等于代码兜底 %d", d.DefaultValue, L1WindowSize)
		}
		if d.ValueType != "int" {
			t.Errorf("value_type 应为 int，实际 %q", d.ValueType)
		}
		if d.Min == nil || *d.Min != "1" {
			t.Errorf("min 应为 1（窗口 0 会把短期记忆全删光），实际 %v", d.Min)
		}
		if d.Restart {
			t.Error("本条每请求读取，不该标 Restart")
		}
	}
	if !found {
		t.Fatalf("memory.%s 不在 DefaultParamDefs() 里", MemoryL1WindowSizeParam)
	}
}

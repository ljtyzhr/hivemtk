package migrations

// v3.80.0 文件 URL 重签的判据（SaaS 审计 A1/A12）。
//
// 判据落在"库里那一列的值变成什么样"上，不看日志、不看本文件的辅助函数。

import (
	"context"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/storage"

	"gorm.io/gorm"
)

// probeFileURL 探针表：只需 id 与一个 URL 列。
type probeFileURL struct {
	TenantID uint   `gorm:"not null;default:1;index"`
	ID       uint   `gorm:"primaryKey"`
	MediaURL string `gorm:"type:varchar(500)"`
}

func (probeFileURL) TableName() string { return "probe_file_urls" }

// probeShortURL 窄列探针：签名形态必然装不下。
type probeShortURL struct {
	TenantID uint   `gorm:"not null;default:1;index"`
	ID       uint   `gorm:"primaryKey"`
	MediaURL string `gorm:"type:varchar(60)"`
}

func (probeShortURL) TableName() string { return "probe_short_urls" }

// probeMultiImage 多值列探针：一列里装着多条 URL。
type probeMultiImage struct {
	TenantID uint   `gorm:"not null;default:1;index"`
	ID       uint   `gorm:"primaryKey"`
	Images   string `gorm:"type:varchar(500)"`
}

func (probeMultiImage) TableName() string { return "probe_multi_images" }

func probeTargets() []fileURLTarget {
	return []fileURLTarget{{"probe_file_urls", "media_url"}}
}

func withSigner(t *testing.T) {
	t.Helper()
	if err := storage.ConfigureFileURLSigner("migration-test-secret-32+chars-long!!", 0); err != nil {
		t.Fatalf("装配签名器失败: %v", err)
	}
}

// readBack on 模式下回读也必须声明租户（SystemScope）——迁移跨租户，用例跟着同一口径。
func readBack(t *testing.T, db *gorm.DB, id uint) probeFileURL {
	t.Helper()
	var got probeFileURL
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("id = ?", id).First(&got).Error; err != nil {
		t.Fatalf("回读 id=%d 失败: %v", id, err)
	}
	return got
}

func newProbeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &probeFileURL{})
	if db == nil {
		t.Skip("测试库不可达")
	}
	return db
}

// 存量明文租户段 URL ⇒ 重签成"无租户段 + 签名"，且能过 storage.VerifyFileURL。
func TestFileURLSignMigration_RewritesLegacyTenantURL(t *testing.T) {
	db := newProbeDB(t)
	withSigner(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	ctx := tenant.WithTenantID(context.Background(), 5)
	if err := db.WithContext(ctx).Create(&probeFileURL{
		MediaURL: "/files/tenant_5/materials/2026/10/a.pdf",
	}).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	// 已经是签名形态（含 ?s=）的不该被再签一次
	if err := db.WithContext(ctx).Create(&probeFileURL{
		MediaURL: "/files/materials/2026/10/b.pdf?s=already",
	}).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}

	m := NewFileURLSignMigration(db).withTargets(probeTargets()...)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}

	got := readBack(t, db, 1)
	if strings.Contains(got.MediaURL, "tenant_5") {
		t.Fatalf("重签后 URL 里仍有租户段：%q（A12）", got.MediaURL)
	}
	if !strings.HasPrefix(got.MediaURL, "/files/materials/2026/10/a.pdf?s=") {
		t.Fatalf("重签形态不对：%q", got.MediaURL)
	}
	rel := strings.TrimPrefix(strings.SplitN(got.MediaURL, "?", 2)[0], "/files/")
	sig := strings.TrimPrefix(strings.SplitN(got.MediaURL, "?", 2)[1], "s=")
	tid, path, err := storage.VerifyFileURL(rel, sig, time.Now())
	if err != nil {
		t.Fatalf("重签出来的 URL 验不过（形同把链接签废了）：%v", err)
	}
	if tid != 5 || path != "materials/2026/10/a.pdf" {
		t.Fatalf("签名解出 (tenant=%d,path=%q)，期望 (5, materials/2026/10/a.pdf)", tid, path)
	}

	// 已有签名的行不动
	other := readBack(t, db, 2)
	if other.MediaURL != "/files/materials/2026/10/b.pdf?s=already" {
		t.Fatalf("已签名的值被改动了：%q", other.MediaURL)
	}
}

// 幂等：第二次跑不动任何一行。
func TestFileURLSignMigration_Idempotent(t *testing.T) {
	db := newProbeDB(t)
	withSigner(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	ctx := tenant.WithTenantID(context.Background(), 7)
	if err := db.WithContext(ctx).Create(&probeFileURL{MediaURL: "/files/tenant_7/covers/2026/10/c.png"}).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	m := NewFileURLSignMigration(db).withTargets(probeTargets()...)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("第一次 Up 失败: %v", err)
	}
	after1 := readBack(t, db, 1)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("第二次 Up 失败: %v", err)
	}
	after2 := readBack(t, db, 1)
	if after1.MediaURL != after2.MediaURL {
		t.Fatalf("第二次跑把值又改了一遍（不幂等）：\n第一次 %q\n第二次 %q", after1.MediaURL, after2.MediaURL)
	}
}

// off 模式：一行都不动（离线单机升级必须零副作用）。
func TestFileURLSignMigration_OffModeNoop(t *testing.T) {
	db := newProbeDB(t)
	withSigner(t)
	tenant.SetMode(tenant.ModeOff)

	const legacy = "/files/tenant_5/materials/2026/10/a.pdf"
	if err := db.Create(&probeFileURL{MediaURL: legacy}).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	m := NewFileURLSignMigration(db).withTargets(probeTargets()...)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	got := readBack(t, db, 1)
	if got.MediaURL != legacy {
		t.Fatalf("off 模式下值被改了：%q，期望原样 %q", got.MediaURL, legacy)
	}
}

// 没有签名器（on 模式缺密钥）：不报错、不改写、但行要留下来（改由运维配好密钥重跑）。
func TestFileURLSignMigration_NoSignerKeepsRows(t *testing.T) {
	db := newProbeDB(t)
	if err := storage.ConfigureFileURLSigner("reset-secret-32+chars-long-enough!!!", 0); err != nil {
		t.Fatalf("装载签名器失败: %v", err)
	}
	storage.DisableFileURLSignerForTest()

	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	const legacy = "/files/tenant_5/materials/2026/10/a.pdf"
	if err := db.WithContext(tenant.WithTenantID(context.Background(), 5)).
		Create(&probeFileURL{MediaURL: legacy}).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	m := NewFileURLSignMigration(db).withTargets(probeTargets()...)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("缺密钥时不该让迁移失败（否则升级全卡住）: %v", err)
	}
	if got := readBack(t, db, 1); got.MediaURL != legacy {
		t.Fatalf("缺密钥时不该改写，实际 %q", got.MediaURL)
	}
}

// 列宽装不下签名形态：跳过该行、不报错 —— 一行数据不许把整个升级卡住。
//
// 真库里 `live_code_qrs.image_url` 只有 varchar(255)，而签名形态比明文长 ~100 字符，
// 某条格外长的路径就会越界。照写会让 UPDATE 因超长报错、迁移失败、升级停在那张表上。
func TestFileURLSignMigration_TooLongRowIsSkippedNotFatal(t *testing.T) {
	db := testutil.NewTestDB(t, &probeShortURL{})
	if db == nil {
		t.Skip("测试库不可达")
	}
	withSigner(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	const legacy = "/files/tenant_5/materials/2026/10/a.pdf"
	if err := db.WithContext(tenant.WithTenantID(context.Background(), 5)).
		Create(&probeShortURL{MediaURL: legacy}).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	m := NewFileURLSignMigration(db).withTargets(fileURLTarget{"probe_short_urls", "media_url"})
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("列宽装不下时不该让迁移失败（否则升级卡在一行数据上）: %v", err)
	}
	var got probeShortURL
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Where("id = ?", 1).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.MediaURL != legacy {
		t.Fatalf("装不下的行被硬写进去了（或写坏了）：%q", got.MediaURL)
	}
}

// 未登记的多值列绝不被盲改：JSON/逗号分隔的列用"整列换成一条 URL"去改就是数据销毁。
//
// 这一格钉的是"体检只报告、不写库"这条自限：将来有人把 reportUnregisteredLegacyColumns
// "顺手升级"成也去重签，这里会立刻红。
func TestFileURLSignMigration_UnregisteredMultiValueColumnUntouched(t *testing.T) {
	db := testutil.NewTestDB(t, &probeMultiImage{})
	if db == nil {
		t.Skip("测试库不可达")
	}
	withSigner(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	const multi = `["/files/tenant_5/covers/2026/10/a.png","/files/tenant_5/covers/2026/10/b.png"]`
	if err := db.WithContext(tenant.WithTenantID(context.Background(), 5)).
		Create(&probeMultiImage{Images: multi}).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	// targets 只登记单值那列 ⇒ images 属于"未登记"
	m := NewFileURLSignMigration(db).withTargets(fileURLTarget{"probe_file_urls", "media_url"})
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	var got probeMultiImage
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Where("id = ?", 1).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Images != multi {
		t.Fatalf("未登记的多值列被改写了（数据销毁）：%q", got.Images)
	}
}

// 登记清单本身：不许有重复项（重复 = 同一列扫两遍，第二遍空跑却让人以为覆盖了两处）。
func TestFileURLSignMigration_TargetsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, tg := range fileURLTargets {
		key := tg.table + "." + tg.column
		if seen[key] {
			t.Errorf("登记清单里有重复项 %s", key)
		}
		seen[key] = true
		if tg.table == "" || tg.column == "" {
			t.Errorf("登记项有空字段: %+v", tg)
		}
	}
	if len(fileURLTargets) == 0 {
		t.Fatal("登记清单是空的：本迁移会变成空转")
	}
}

// resignLegacyFileURL 的形状边界：只动 /files/tenant_<数字>/ 形态。
func TestResignLegacyFileURL_Shapes(t *testing.T) {
	withSigner(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	cases := []struct {
		in   string
		want bool
	}{
		{"/files/tenant_5/materials/a.pdf", true},
		{"/files/tenant_5/materials/a.pdf?x=1", true}, // 带 query 也认（路径部分才是判据）
		{"/files/materials/a.pdf", false},             // 无租户段：不是本迁移的对象
		{"https://cdn.example.com/files/tenant_5/a.pdf", false},
		{"tenant_5/materials/a.pdf", false},
		{"/files/tenant_x/materials/a.pdf", false}, // 租户段不是数字
		{"", false},
	}
	for _, tc := range cases {
		got, ok := resignLegacyFileURL(tc.in)
		if ok != tc.want {
			t.Errorf("resignLegacyFileURL(%q) ok=%v，期望 %v（返回 %q）", tc.in, ok, tc.want, got)
		}
		if ok && strings.Contains(got, "tenant_5") {
			t.Errorf("重签结果里仍有租户段：%q", got)
		}
	}
}

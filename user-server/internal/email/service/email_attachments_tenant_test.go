// email_attachments_tenant_test.go on 模式下的附件出口（SaaS 审计 A18）。
//
// 缺陷形状：附件落盘在 on 模式分域到 `{baseDir}/tenant_{id}/attachments/…`（T011 起就这样），
// 而附件解析器只认 `{baseDir}/attachments/…` 一个根 ⇒ on 模式下**每一条**附件都 Lstat 不中、
// 静默丢弃，只有一条 WARN。表现是"邮件写着已发送、用户从来没收到附件"，任何用例都不红：
// 既有用例注入的解析器指向平根（off 形态），从没走过带租户的那条路。
//
// 判据：on 模式下，URL（带签名 token，租户只在 token 里 —— A12 之后路径里不再有 tenant 段）
// 必须解析到 `{baseDir}/{tenant_dir}/attachments/{yyyy}/{mm}/{name}` 这棵真树。
package email

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/storage"
)

// seedTenantAttachment 在临时根下造一棵"on 模式真落盘"的附件树，返回（根, 相对附件根的路径）。
func seedTenantAttachment(t *testing.T, tenantDir string) (string, string) {
	t.Helper()
	root := t.TempDir()
	rel := "2026/09/quote.pdf"
	full := filepath.Join(root, tenantDir, "attachments", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("建租户附件目录失败: %v", err)
	}
	if err := os.WriteFile(full, []byte("报价单"), 0o600); err != nil {
		t.Fatalf("写附件失败: %v", err)
	}
	t.Setenv("STORAGE_LOCAL_BASE_DIR", root)
	t.Setenv("STORAGE_LOCAL_PUBLIC_URL", "/files")
	return root, rel
}

// on 模式：签名 URL ⇒ 解析到租户目录里的那个文件（这条修前是红的）。
func TestAttachmentPaths_OnModeResolvesTenantRoot(t *testing.T) {
	_, rel := seedTenantAttachment(t, "tenant_5")

	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	if err := storage.ConfigureFileURLSigner("email-attachment-test-secret-32+chars!", 0); err != nil {
		t.Fatalf("装配签名器失败: %v", err)
	}
	t.Cleanup(func() { storage.ResetFileURLSignerForTest() })

	signedRel, sig, ok := storage.SignStoredPath("tenant_5/attachments/" + rel)
	if !ok {
		t.Fatal("没签出 URL：签名器未装配")
	}
	csv := "/files/" + signedRel + "?s=" + sig

	svc := NewEmailSendService() // 不注入解析器：走生产兜底那条路
	got := svc.attachmentPaths(csv)
	if len(got) != 1 {
		t.Fatalf("on 模式附件没解析出来（附件会被静默丢弃）：got=%v", got)
	}
	if _, err := os.Stat(got[0]); err != nil {
		t.Fatalf("解析出的路径不是真文件：%q (%v)", got[0], err)
	}
	want := string(filepath.Separator) + "tenant_5" + string(filepath.Separator) + "attachments" + string(filepath.Separator)
	if !strings.Contains(got[0], want) {
		t.Fatalf("解析出的不是租户 5 那棵树（期望含 %q）：%q", want, got[0])
	}
}

// on 模式：签名对不上（篡改/无签名）⇒ 不许回落到平根去碰运气。
func TestAttachmentPaths_OnModeUnsignedDoesNotFallBack(t *testing.T) {
	// 平根下也放一份同名文件：如果"验不过"时回落到平根，就会误挂到别人的（或残留的）文件上。
	seedTenantAttachment(t, "tenant_5")
	t.Setenv("STORAGE_LOCAL_BASE_DIR", t.TempDir())

	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	if err := storage.ConfigureFileURLSigner("email-attachment-test-secret-32+chars!", 0); err != nil {
		t.Fatalf("装配签名器失败: %v", err)
	}
	t.Cleanup(func() { storage.ResetFileURLSignerForTest() })

	svc := NewEmailSendService()
	if got := svc.attachmentPaths("/files/attachments/2026/09/quote.pdf"); len(got) != 0 {
		t.Fatalf("无签名的值不该解析出磁盘路径（否则等于按路径猜文件）：%v", got)
	}
}

// off 模式：逐字节等价历史（平根解析，不需要签名）。
func TestAttachmentPaths_OffModeUnchanged(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "attachments", "2026", "09", "quote.pdf")
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(full, []byte("报价单"), 0o600); err != nil {
		t.Fatalf("写附件失败: %v", err)
	}
	t.Setenv("STORAGE_LOCAL_BASE_DIR", root)
	t.Setenv("STORAGE_LOCAL_PUBLIC_URL", "/files")

	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	svc := NewEmailSendService()
	got := svc.attachmentPaths("/files/attachments/2026/09/quote.pdf")
	if len(got) != 1 || got[0] != full {
		t.Fatalf("off 模式解析结果变了：got=%v want=[%s]", got, full)
	}
}

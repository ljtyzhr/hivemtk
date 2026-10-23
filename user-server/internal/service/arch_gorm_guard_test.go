package service

import (
	"os/exec"
	"strings"
	"testing"
)

// arch_gorm_guard_test.go —— E-4 新代码守卫（2026-10-24，规划 §4.4）。
//
// 规划口径：先加新代码守卫——静态扫描禁止新增 `*gorm.DB` 字段或参数进入
// service 层；按域下沉是长期债，本轮不做，但基线写入门禁只许降。
//
// 形态：架构测试（go test 内跑 grep），与 scripts/check-architecture.sh 的
// 2.2.1（字段级）互补——这里把口径扩到**字段 + 形参**（形参如
// `func NewXxx(db *gorm.DB)` 也是"service 伸手要 DB 句柄"的入口），基线
// 180 处/107 文件（规划记录值；现树实测 169/107，只许降）。
//
// 与 scripts/arch-l4-getdb.baseline 同一双向口径：超出基线判红；**收窄**
// （实际 < 基线）也判红——提示把基线下调到新值，棘轮不许松。
// 基线值直接写死在本文件：改基线要改测试（显式棘轮动作，不是顺手改脚本）。

const (
	gormGuardMaxHits  = 166 // 规划基线 180，R25 现树实测 169，2026-10-24 任务一收口后现树 166 —— 只许降
	gormGuardMaxFiles = 106
	gormGuardScope    = "internal/service/" // 含 internal/geo/service 等子包
)

func TestArchServiceNoNewGormDB(t *testing.T) {
	// cwd 是包目录（internal/service），仓库根在 ../../
	cmd := exec.Command("grep", "-rnE", "--include=*.go", `\*gorm\.DB`, ".")
	out, err := cmd.Output()
	if err != nil {
		// grep 退出码 1 = 无命中 ⇒ 门过（理想终态）
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return
		}
		t.Fatalf("grep 执行失败: %v", err)
	}
	hits, files := 0, map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || strings.Contains(line, "_test.go") {
			continue
		}
		hits++
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			files[line[:idx]] = true
		}
	}
	t.Logf("service 层 *gorm.DB 出现：%d 处 / %d 个文件（基线 %d/%d）",
		hits, len(files), gormGuardMaxHits, gormGuardMaxFiles)
	if hits > gormGuardMaxHits {
		t.Fatalf("新增 *gorm.DB 进入 service 层（%d > 基线 %d）：按规划 §4.4，新增应走 Repository 注入；确需调整基线请改本文件的 gormGuardMaxHits（显式棘轮）", hits, gormGuardMaxHits)
	}
	if len(files) > gormGuardMaxFiles {
		t.Fatalf("service 层出现 *gorm.DB 的新文件数超基线（%d > %d）", len(files), gormGuardMaxFiles)
	}
	if hits < gormGuardMaxHits || len(files) < gormGuardMaxFiles {
		t.Fatalf("基线收窄（现 %d 处/%d 文件 < 基线 %d/%d）：这是好事，但请把 gormGuardMaxHits/gormGuardMaxFiles 下调到实际值——棘轮基线不许高于现实",
			hits, len(files), gormGuardMaxHits, gormGuardMaxFiles)
	}
}

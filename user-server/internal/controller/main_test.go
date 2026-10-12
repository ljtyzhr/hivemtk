// Package controller 的测试统一入口（TestMain）
//
// 上传用例会把文件按 STORAGE_LOCAL_BASE_DIR → ./uploads 落盘，而 go test 的 CWD
// 就是本包目录 —— 于是每跑一轮就往 internal/controller/uploads/ 里堆一批
// （历史峰值 993 个文件）。这里在所有用例前把三棵存储根指进临时目录，
// 收尾再 Sweep 一次，兜住中途被用例自清 env 后回退到相对路径的残留。
//
// 本包其余测试不依赖"环境变量必须未设置"，故注入是安全的；真有这种依赖的用例
// 自己 t.Setenv 覆盖即可（t.Setenv 结束后还原成注入值，不会污染后续用例）。
package controller

import (
	"os"
	"path/filepath"
	"testing"

	"hivemtk-user/internal/storage/storagetest"
)

// TestMain controller 包测试统一入口
func TestMain(m *testing.M) {
	restoreStorageRoots := storagetest.Install()

	// install.lock 的默认路径是相对的 ./install.lock，而 go test 的 CWD 就是本包目录：
	// 走过一次 InitAdmin 成功路径的用例就会把"已初始化"写进源码树（本机实测残留过一份
	// internal/controller/install.lock），于是后面每个读安装态的用例都被这份残留判成重复安装。
	// 整体挪进临时目录，收尾删掉——安装态只能由用例自己 t.Setenv 显式给。
	installLockDir, err := os.MkdirTemp("", "hivemtk-controller-install-lock")
	if err != nil {
		panic("创建测试用 install.lock 目录失败: " + err.Error())
	}
	_ = os.Setenv("INSTALL_LOCK_PATH", filepath.Join(installLockDir, "install.lock"))

	code := m.Run()
	storagetest.Sweep(".")
	restoreStorageRoots()
	_ = os.RemoveAll(installLockDir)
	os.Exit(code)
}

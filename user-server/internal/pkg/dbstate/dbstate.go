// Package dbstate 持有进程内全局 *gorm.DB 指针的唯一存取点。
//
// 拆出来的理由是 import cycle：model / db / metrics 的包内测试 import testutil，
// 而 db 非测试代码 import model。若 testutil 直接 import db 写 SetTestDB，
// 会形成 testutil→db→model←(包内测试)←testutil。本包只依赖 gorm，是叶子节点，
// testutil 与 db 都可以安全引用。
package dbstate

import (
	"sync"

	"gorm.io/gorm"
)

var (
	mu sync.RWMutex
	db *gorm.DB
)

// Get 读全局句柄（可能为 nil）。
func Get() *gorm.DB {
	mu.RLock()
	defer mu.RUnlock()
	return db
}

// Set 写全局句柄（测试用；生产路径应走 db.InitDB）。
func Set(d *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()
	db = d
}

// advisory_lock.go — 跨实例互斥锁（GORM 形态）。
//
// 为什么不用 `db.Select("pg_try_advisory_lock(?)", key).Scan(&held)` 直写：
// GORM 的 Select 在**没有 Model/Table** 时会拼出 `SELECT <expr> FROM `
// （FROM 后面是空的）⇒ 结果集多一列空值，Scan 进 bool 报
// "unsupported data type: <nil>"，而且**锁根本没拿到**。
// POST /api/monitor/trace-eval/trigger 恒 500，就是这一句；
// geo/service/job_manager.go 的分布式锁是同形缺陷（且无测试覆盖，一直没暴露）。
//
// 正确形态见下面 advisoryLockFrom 的注释。
//
// 判读与 geo/service/job_manager.go 的分布式锁一致：
// pg_try_advisory_lock / pg_advisory_unlock 都不读写租户表行，
// 裸 ctx 不构成隔离 callback 的触点。
package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// advisoryLockFrom 是拼 FROM 时用的载体：**恒一行的派生表**。
//
// 为什么是派生表而不是某张真表：
//   - 没有 Model/Table 时，GORM 拼出 `SELECT <expr> FROM `（FROM 后面是空的）
//     ⇒ 结果集多一列空值，Scan 进 bool 报 "unsupported data type: <nil>"。
//   - 改成某张真表也不对：那张表要有数据才有行，`SELECT pg_try_advisory_lock(k)
//     FROM <空表>` 返回 0 行 ⇒ **锁函数根本没被求值** ⇒ 锁没拿到，
//     而返回的 false 会被调用方读成「已有实例在运行」⇒ 多实例保护形同虚设。
//   - 派生表恒一行 ⇒ 函数必被求值一次，锁语义正确。
//
// 派生表不触达任何租户表 ⇒ 不构成隔离 callback 的触点（与 geo job_manager
// 的同一判读一致：advisory lock 是会话级全局闸）。
const advisoryLockFrom = "(SELECT 1) AS t"

// TryAdvisoryLock 尝试获取会话级 advisory lock。
//
// 返回 acquired=false 表示锁已被别人持有（不是错误）。
// key 是 int64 锁键。
func TryAdvisoryLock(ctx context.Context, gdb *gorm.DB, key int64) (bool, error) {
	if gdb == nil {
		return false, fmt.Errorf("db is nil")
	}
	var held bool
	if err := gdb.WithContext(ctx).Table(advisoryLockFrom).
		Select("pg_try_advisory_lock(?)", key).Scan(&held).Error; err != nil {
		return false, err
	}
	return held, nil
}

// ReleaseAdvisoryLock 释放会话级 advisory lock。
func ReleaseAdvisoryLock(ctx context.Context, gdb *gorm.DB, key int64) error {
	if gdb == nil {
		return fmt.Errorf("db is nil")
	}
	var unlocked bool
	return gdb.WithContext(ctx).Table(advisoryLockFrom).
		Select("pg_advisory_unlock(?)", key).Scan(&unlocked).Error
}

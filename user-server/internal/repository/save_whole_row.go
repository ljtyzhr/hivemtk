package repository

import (
	"gorm.io/gorm"

	_db "hivemtk-user/internal/pkg/db"
)

// saveWholeRow 整行回写 + "命中 0 行即不存在"的唯一入口。
//
// 实现在 `internal/pkg/db`（多个仓储包要共用同一条口径：本包、aiagent 域、
// geo 域各自的 repository 包都有整行回写）。要点：裸 `Save` 在 UPDATE 命中 0 行时
// 会**回退成 INSERT**（gorm finisher_api.Save：`RowsAffected == 0 && !selectedUpdate`
// ⇒ `OnConflict{UpdateAll:true}.Create`），于是"更新一条已删除/跨租户的行"变成
// "凭空插一条同主键新行"而接口回成功；`Select("*")` 关掉该回退且零值字段照样写回。
// 0 行 ⇒ gorm.ErrRecordNotFound。
func saveWholeRow(db *gorm.DB, row any) error {
	return _db.SaveWholeRow(db, row)
}

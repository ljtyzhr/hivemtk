package db

import (
	"gorm.io/gorm"
)

// SaveWholeRow 整行回写 + "命中 0 行即不存在"的统一入口。
//
// 为什么不能直接 `db.Save(row)`：
//
//	gorm v2 的 Save 在 UPDATE 命中 0 行后会**回退成 INSERT**
//	（gorm@v1.31.2 finisher_api.go：`updateTx.RowsAffected == 0 && !selectedUpdate`
//	⇒ `Clauses(OnConflict{UpdateAll: true}).Create(value)`）。也就是说
//	"更新一条已被删除、或 id 属于别的租户的行"不是静默成功，而是**凭空插出一条
//	主键相同的新行**——比静默成功更难查：库里多出一条孤儿配置，接口却回 200。
//	真库实测：`Save(&WebhookEvent{ID: 88888888})` 之后
//	`select count(*) from webhook_events where id = 88888888` = 1。
//
// 为什么要显式 `Select("*")`：
//
//	一是关掉上面那条回退（带 Selects 时走 selectedUpdate 分支，不再 Create）；
//	二是保持"整行写"语义——Update 回调对 Selects 为 * 的结构体会把**零值字段**
//	一起写回（清空备注、把 bool 改回 false 都靠这个），与裸 Save 的效果一致。
//	实测：先置 RawData=""、Processed=false 再回写，读回来确实是空/false。
//
// 契约：0 行 ⇒ gorm.ErrRecordNotFound（上层 response.ErrorFromDB 映射 404），
// 而不是"回退插入"或"静默成功"。
func SaveWholeRow(db *gorm.DB, row any) error {
	res := db.Select("*").Save(row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// EnsureAffected 把"定向写命中 0 行"翻成 gorm.ErrRecordNotFound。
//
// 用于 `Updates` / `Update` / `Delete` 这类定向写（它们没有 Save 的 INSERT 回退，
// 但同样会把 0 行当成成功返回）。调用点应传已完成 WithContext 的 res。
func EnsureAffected(res *gorm.DB) error {
	if res == nil {
		return gorm.ErrInvalidDB
	}
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

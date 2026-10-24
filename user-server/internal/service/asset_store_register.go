package service

import (
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// T5 资产市场商店化五表建表登记（service→db 反向注册，避开 model→db 成环，
// 先例：T4 kb_crawler_register.go / KBDocumentChunkRow）。
// 漏登记 ⇒ 全新部署建不出表 ⇒ 商店化整链路 relation does not exist。
func init() {
	db.RegisterExtraModels(
		&model.AssetListing{},
		&model.AssetVersion{},
		&model.AssetReport{},
		&model.AssetRating{},
		&model.ExtensionUsageMonthly{},
	)
}

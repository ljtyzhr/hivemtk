// kb_crawler_register.go 三张 T4 表的建表登记。
//
// 生产建表只跑 AutoMigrate（kb_release.go 文件头 T-P9-02 实测口径）；
// RegisterExtraModels 由 AutoMigrate() 统一消费（internal/pkg/db/migrate.go:643）。
package model

import _db "hivemtk-user/internal/pkg/db"

func init() {
	_db.RegisterExtraModels(
		&KBCrawlerSource{},
		&KBCrawlBatch{},
		&QACandidate{},
	)
}

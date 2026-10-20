package service

import (
	"sort"
	"sync"
	"testing"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm/schema"
)

// 外挂模型（各包 init 里的 RegisterExtraModels）不在 pkg/db 的 allModels 清单中，
// db 包测试也无法 import 它们（会成 import 环）。这里做两件事：
//  1. 每个外挂模型必须真的被注册进 ExtraModels（漏 init 会让全新部署缺表）；
//  2. 每个外挂模型必须声明 tenant_id（漏声明会静默落列默认值 1 = 串租户）。
func TestExtraModelsRegisteredAndTenantScoped(t *testing.T) {
	want := map[string]any{
		"ab_exposures":           &repository.AbExposure{},
		"kb_document_chunks":     &repository.KBDocumentChunkRow{},
		"email_accounts":         &EmailAccount{},
		"reach_compliance_log":   &model.ReachComplianceLog{},
		"reach_delayed_outbound": &model.DelayedOutboundReply{},
	}

	cache := &sync.Map{}
	registered := map[string]bool{}
	for _, m := range _db.ExtraModels() {
		s, err := schema.Parse(m, cache, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("解析已注册外挂模型 %T 失败: %v", m, err)
		}
		registered[s.Table] = true
	}

	tables := make([]string, 0, len(want))
	for table := range want {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		if !registered[table] {
			t.Errorf("外挂模型 %s（%T）未通过 RegisterExtraModels 注册：全新部署不会建表", table, want[table])
		}
		s, err := schema.Parse(want[table], cache, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", table, err)
		}
		has := false
		for _, f := range s.Fields {
			if f.DBName == "tenant_id" {
				has = true
				break
			}
		}
		if !has {
			t.Errorf("外挂模型 %s（%T）必须声明 TenantID", table, want[table])
		}
	}
}

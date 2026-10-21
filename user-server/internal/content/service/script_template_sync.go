package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	contentmodel "hivemtk-user/internal/content/model"
	"hivemtk-user/internal/content/repository"
	mainmodel "hivemtk-user/internal/model"
)

// ErrScriptLibraryMissing 读前置与写口之间那一行没了（含「不存在」⇒ 出口 404）。
var ErrScriptLibraryMissing = errors.New("运行时话术不存在")

// ScriptTemplateSyncService 话术模板 → 运行时话术库 同步服务
type ScriptTemplateSyncService struct {
	templateRepo *repository.ScriptTemplateRepository
	db           *gorm.DB
}

// NewScriptTemplateSyncService 创建同步服务：句柄由装配点注入，
// 读写都跟着调用方 ctx 走（改前的无参构造口自己摸全局句柄，
// on 模式下整条同步被隔离闸拒成 500，而 off 模式下它跨租户合并话术）。
func NewScriptTemplateSyncService(db *gorm.DB) *ScriptTemplateSyncService {
	return &ScriptTemplateSyncService{
		templateRepo: repository.NewScriptTemplateRepository(db),
		db:           db,
	}
}

// SyncToLibraryStats 同步结果统计
type SyncToLibraryStats struct {
	Scanned int `json:"scanned"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// SyncToLibrary 执行 template → library 单向同步（幂等，可重复执行）。
//
// 读取管理端话术模板，逐条按 (category, title) upsert 到 script_library：
//   - 未命中：创建（Tags 由逗号分隔字符串转 JSONArray，Scenario 取 JourneyStage）
//   - 已命中：更新 Content/Scenario/Tags，保留 library 侧 usage_count 等运行时统计
//
// 同步是**每租户各同步一次**的动作：读模板与查重键都由 ctx 决定租户，
// 新建的 library 行由隔离 callback 填上调用方租户。
// 不走 tenant.WithSystemScope：系统作用域会豁免闸，Create 时跳过 tenant_id 填充
// ⇒ 所有租户的话术都落进列默认值 1（默认租户），B 家内容变成 A 家可见。
func (s *ScriptTemplateSyncService) SyncToLibrary(ctx context.Context) (*SyncToLibraryStats, error) {
	if s == nil || s.db == nil || s.templateRepo == nil {
		return nil, errors.New("script template sync: service not initialized")
	}
	templates, err := s.templateRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	stats := &SyncToLibraryStats{Scanned: len(templates)}
	for _, t := range templates {
		if strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Content) == "" {
			stats.Skipped++
			continue
		}
		created, err := s.upsertOne(ctx, t)
		if err != nil {
			return stats, err
		}
		if created {
			stats.Created++
		} else {
			stats.Updated++
		}
	}
	return stats, nil
}

func (s *ScriptTemplateSyncService) upsertOne(ctx context.Context, t *contentmodel.ScriptTemplate) (bool, error) {
	var lib mainmodel.ScriptLibrary
	err := s.db.WithContext(ctx).Where("category = ? AND title = ?", t.Category, t.Title).First(&lib).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		rec := templateToLibrary(t)
		if err := s.db.WithContext(ctx).Create(rec).Error; err != nil {
			return false, err
		}
		return true, nil
	case err != nil:
		return false, err
	default:
		// 列级更新：只交正文/场景/标签三列，library 侧的 usage_count、
		// success_count、conversion_rate、version、status 不由同步口覆写。
		// 不用 Save——那条 upsert 会按结构体快照把归属与运行时统计一起顶回去。
		res := s.db.WithContext(ctx).Model(&lib).Updates(map[string]any{
			"content":  t.Content,
			"scenario": t.JourneyStage,
			"tags":     parseTagsToJSONArray(t.Tags),
		})
		if res.Error != nil {
			return false, res.Error
		}
		if res.RowsAffected == 0 {
			return false, ErrScriptLibraryMissing
		}
		return false, nil
	}
}

func templateToLibrary(t *contentmodel.ScriptTemplate) *mainmodel.ScriptLibrary {
	return &mainmodel.ScriptLibrary{
		Category: t.Category,
		Title:    t.Title,
		Content:  t.Content,
		Scenario: t.JourneyStage,
		Tags:     parseTagsToJSONArray(t.Tags),
	}
}

func parseTagsToJSONArray(s string) mainmodel.JSONArray {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；'
	})
	arr := make(mainmodel.JSONArray, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			arr = append(arr, p)
		}
	}
	return arr
}

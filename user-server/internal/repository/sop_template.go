package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// errSOPTemplateUnavailable 仓储句柄缺失（装配错，不是「没数据」）。
var errSOPTemplateUnavailable = errors.New("sop template repository not initialized")

type SOPTemplateRepository struct {
	db *gorm.DB
}

// NewSOPTemplateRepositoryDefault 默认构造（全局句柄；未装配时返回 nil，调用方按「repo 未装配」处理）。
func NewSOPTemplateRepositoryDefault() *SOPTemplateRepository {
	if db := GetDB(); db != nil {
		return NewSOPTemplateRepository(db)
	}
	return nil
}

func NewSOPTemplateRepository(db *gorm.DB) *SOPTemplateRepository {
	return &SOPTemplateRepository{db: db}
}

// Create 新增 SOP 模板
func (r *SOPTemplateRepository) Create(ctx context.Context, tpl *model.SOPTemplate) error {
	if r == nil || r.db == nil {
		return errSOPTemplateUnavailable
	}
	if tpl == nil {
		return errors.New("sop template: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Select("*").Create(tpl).Error
}

// GetByID 按 ID 查询
func (r *SOPTemplateRepository) GetByID(ctx context.Context, id uint) (*model.SOPTemplate, error) {
	if r == nil || r.db == nil {
		return nil, errSOPTemplateUnavailable
	}
	var tpl model.SOPTemplate
	if err := r.db.WithContext(ctx).First(&tpl, id).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

// ListEnabled 查询所有启用的 SOP 模板 (按 priority DESC 排序)
func (r *SOPTemplateRepository) ListEnabled(ctx context.Context, limit int) ([]model.SOPTemplate, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var tpls []model.SOPTemplate
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("priority DESC, confidence DESC, id ASC").
		Limit(limit).
		Find(&tpls).Error
	return tpls, err
}

// MatchByIntent 按意图匹配 SOP 模板 (取最高优先级且启用) — 兼容旧签名 (agentID=0 表示共享/全局)
//
// agentID > 0: 仅匹配该智能体私有 (agent_id=X) + 共享 (agent_id IS NULL)
// agentID = 0: 全局共享 (向后兼容)
func (r *SOPTemplateRepository) MatchByIntent(ctx context.Context, intent string) ([]model.SOPTemplate, error) {
	return r.MatchByIntentForAgent(ctx, intent, 0)
}

// MatchByIntentForAgent 按智能体隔离的意图匹配
//
// agentID = 0: 全局共享 (向后兼容)
// agentID > 0: 匹配 (agent_id = agentID OR agent_id IS NULL) AND enabled
//
// agentID = 0 分支是活的：MatchByIntent 就是它的薄壳，生产链路走的全是这一层。
// agentID > 0 的「私有 + 共享回退」分支与 MatchByAgent 的严格 1:1 语义并存，
// 删掉等于替产品决定「不再支持共享回退」，故保留待决策。
func (r *SOPTemplateRepository) MatchByIntentForAgent(ctx context.Context, intent string, agentID uint) ([]model.SOPTemplate, error) {
	if intent == "" {
		return nil, nil
	}
	var tpls []model.SOPTemplate
	q := r.db.WithContext(ctx).
		Where("intent = ? AND enabled = ?", intent, true)
	if agentID > 0 {
		q = q.Where("agent_id = ? OR agent_id IS NULL", agentID)
	}
	err := q.Order("priority DESC, confidence DESC, id ASC").
		Limit(10).
		Find(&tpls).Error
	return tpls, err
}

// MatchByIntentStage 按 (intent, stage) 精确匹配 SOP 模板 — 兼容旧签名
func (r *SOPTemplateRepository) MatchByIntentStage(ctx context.Context, intent, stage string) ([]model.SOPTemplate, error) {
	return r.MatchByIntentStageForAgent(ctx, intent, stage, 0)
}

// MatchByIntentStageForAgent 按 (intent, stage) + agentID 匹配 (隔离架构)
//
// agentID = 0: 全局共享 (向后兼容)
// agentID > 0: 匹配 (agent_id = agentID OR agent_id IS NULL) AND enabled
// 保留理由同 MatchByIntentForAgent。
func (r *SOPTemplateRepository) MatchByIntentStageForAgent(ctx context.Context, intent, stage string, agentID uint) ([]model.SOPTemplate, error) {
	if intent == "" || stage == "" {
		return nil, nil
	}
	var tpls []model.SOPTemplate
	q := r.db.WithContext(ctx).
		Where("intent = ? AND stage = ? AND enabled = ?", intent, stage, true)
	if agentID > 0 {
		q = q.Where("agent_id = ? OR agent_id IS NULL", agentID)
	}
	err := q.Order("priority DESC, confidence DESC, id ASC").
		Limit(10).
		Find(&tpls).Error
	return tpls, err
}

// MatchByAgent 按智能体严格 1:1 匹配
//
// 行为:
//   - agentID == 0  -> 返回 (nil, nil)
//   - 仅匹配 enabled = true AND agent_id = ? 的 SOP
//   - 任何 agent 都必须显式绑定才能匹配，无「空数组=全局」回退
//
// SQL: WHERE enabled = true AND agent_id = ?  (走 idx_sop_agent_id 索引)
func (r *SOPTemplateRepository) MatchByAgent(ctx context.Context, agentID uint, intent, stage string) ([]model.SOPTemplate, error) {
	if agentID == 0 {
		return nil, nil
	}
	q := r.db.WithContext(ctx).
		Where("enabled = ? AND agent_id = ?", true, agentID)
	if intent != "" {
		q = q.Where("intent = ?", intent)
	}
	if stage != "" {
		q = q.Where("stage = ?", stage)
	}
	var tpls []model.SOPTemplate
	err := q.Order("priority DESC, confidence DESC, id ASC").
		Limit(10).
		Find(&tpls).Error
	return tpls, err
}

// ListByAgent 列出某智能体的 SOP 模板集合 (不参与打分, 仅做缓存预热 / 后台同步)
func (r *SOPTemplateRepository) ListByAgent(ctx context.Context, agentID uint, limit int) ([]model.SOPTemplate, error) {
	if agentID == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	var tpls []model.SOPTemplate
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND agent_id = ?", true, agentID).
		Order("id ASC").
		Limit(limit).
		Find(&tpls).Error
	return tpls, err
}

// IncrementHitCount 命中次数 +1
func (r *SOPTemplateRepository) IncrementHitCount(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).
		Model(&model.SOPTemplate{}).
		Where("id = ?", id).
		UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).
		Error
}

// ListWithFilter 分页+过滤 (前端 SOP 模板管理页面使用)
//
// 新增 AgentID 过滤 (nil=不过滤, &0=仅共享, &X=仅该智能体)
func (r *SOPTemplateRepository) ListWithFilter(ctx context.Context, filter SOPTemplateListParams) ([]model.SOPTemplate, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errSOPTemplateUnavailable
	}
	// 夹取分页参数：pageSize = 0 ⇒ GORM 的 Limit(0) 等于**不加 limit**（模板表带
	// name/template 两列长文本，一次请求就能把整表读进内存）；page = 0 ⇒ offset
	// 为负 ⇒ PG 直接报 "OFFSET must not be negative"。
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	// Count 与 Find 必须各起一条 query（见 base()），不能在同一条链上先 Count 再 Find
	// （Count 会把 Selects 换成 count(*)，就地改 Statement）。
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.SOPTemplate{})
		if filter.Keyword != "" {
			kw := "%" + filter.Keyword + "%"
			q = q.Where("name LIKE ? OR template LIKE ?", kw, kw)
		}
		if filter.Intent != "" {
			q = q.Where("intent = ?", filter.Intent)
		}
		if filter.Stage != "" {
			q = q.Where("stage = ?", filter.Stage)
		}
		if filter.Enabled != nil {
			q = q.Where("enabled = ?", *filter.Enabled)
		}
		if filter.AgentID != nil {
			if *filter.AgentID == 0 {
				q = q.Where("agent_id IS NULL")
			} else {
				q = q.Where("agent_id = ?", *filter.AgentID)
			}
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tpls []model.SOPTemplate
	err := base().Order("priority DESC, confidence DESC, id ASC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).
		Find(&tpls).Error
	return tpls, total, err
}

// Update 更新 SOP 模板（整行写，且命中 0 行必须报错）
//
// `Select("*")` 是有意的：管理端整份提交，需要「清空某个可选字段」也能落库
// （不加 Select 时 GORM 只写非零字段，清空是假动作）。本表带 gorm.DeletedAt，
// UPDATE 会自动补 `deleted_at IS NULL`，改已删模板既不报错也不写入，
// 故 0 行必须报错，判据统一走 db.EnsureAffected（pkg/db/write_guard.go）。
func (r *SOPTemplateRepository) Update(ctx context.Context, id uint, tpl *model.SOPTemplate) error {
	if r == nil || r.db == nil {
		return errSOPTemplateUnavailable
	}
	if tpl == nil {
		return errors.New("sop template: nil 不是更新对象")
	}
	// id 参数是权威：Select("*") 包含主键列，若沿用 tpl.ID 会把 body 里带的其他 id 写回。
	tpl.ID = id
	// 只写表单拥有的列（与 faq_entry 同一条口径）：
	//   - Select("*") 会把零值 tenant_id 写回，SaaS 形态下租户插件按 fail-closed 直接拒绝
	//     （实测 PUT /api/sop-templates/{id} → 500「更新不可改写 tenant_id」）；
	//   - hit_count 不是表单字段，全量写回会被清零，命中统计从列表上凭空消失。
	updates := map[string]any{
		"name":       tpl.Name,
		"intent":     tpl.Intent,
		"stage":      tpl.Stage,
		"template":   tpl.Template,
		"vars":       tpl.Vars,
		"priority":   tpl.Priority,
		"confidence": tpl.Confidence,
		"updated_at": time.Now(),
	}
	if tpl.Enabled != nil {
		updates["enabled"] = *tpl.Enabled
	}
	if tpl.AgentID != nil && *tpl.AgentID != 0 {
		updates["agent_id"] = *tpl.AgentID
	}
	return _db.EnsureAffected(r.db.WithContext(ctx).Model(&model.SOPTemplate{}).
		Where("id = ?", id).
		Updates(updates))
}

// SetEnabled 仅切换启用状态（列表页开关专用，避免全量 Update 的 required 校验）。
//
// 0 行 ⇒ ErrRecordNotFound：模板已被删（或 id 打错）时，开关不该回"已保存"。
func (r *SOPTemplateRepository) SetEnabled(ctx context.Context, id uint, enabled bool) error {
	if r == nil || r.db == nil {
		return errSOPTemplateUnavailable
	}
	return _db.EnsureAffected(r.db.WithContext(ctx).Model(&model.SOPTemplate{}).
		Where("id = ?", id).
		Update("enabled", enabled))
}

// Delete 删除 SOP 模板。0 行 ⇒ ErrRecordNotFound（删一条已经被删的模板不该回成功）。
func (r *SOPTemplateRepository) Delete(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return errSOPTemplateUnavailable
	}
	return _db.EnsureAffected(r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.SOPTemplate{}))
}

type SOPTemplateListParams struct {
	Keyword  string
	Intent   string
	Stage    string
	Enabled  *bool
	AgentID  *uint
	Page     int
	PageSize int
}

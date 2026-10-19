package repository

import (
	"context"
	"hivemtk-user/internal/geo/model"

	"gorm.io/gorm"
)

// GeoWorkflowRepository GEO 工作流仓储接口
type GeoWorkflowRepository interface {
	Create(ctx context.Context, wf *model.GeoWorkflow) error
	Update(ctx context.Context, wf *model.GeoWorkflow) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*model.GeoWorkflow, error)
	GetList(ctx context.Context) ([]*model.GeoWorkflow, error)
}

type geoWorkflowRepo struct {
	db *gorm.DB
}

func NewGeoWorkflowRepositoryWithDB(db *gorm.DB) GeoWorkflowRepository {
	return &geoWorkflowRepo{db: db}
}

func (r *geoWorkflowRepo) Create(ctx context.Context, wf *model.GeoWorkflow) error {
	return r.db.WithContext(ctx).Create(wf).Error
}

func (r *geoWorkflowRepo) Update(ctx context.Context, wf *model.GeoWorkflow) error {
	return r.db.WithContext(ctx).Save(wf).Error
}

func (r *geoWorkflowRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.GeoWorkflow{}).Error
}

func (r *geoWorkflowRepo) GetByID(ctx context.Context, id string) (*model.GeoWorkflow, error) {
	var wf model.GeoWorkflow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&wf).Error
	return &wf, err
}

func (r *geoWorkflowRepo) GetList(ctx context.Context) ([]*model.GeoWorkflow, error) {
	var wfs []*model.GeoWorkflow
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&wfs).Error
	return wfs, err
}

// GeoWorkflowExecutionRepository GEO 工作流执行记录仓储接口
type GeoWorkflowExecutionRepository interface {
	Create(ctx context.Context, exec *model.GeoWorkflowExecution) error
	Update(ctx context.Context, exec *model.GeoWorkflowExecution) error
	GetByID(ctx context.Context, id string) (*model.GeoWorkflowExecution, error)
	GetByWorkflowID(ctx context.Context, workflowID string) ([]*model.GeoWorkflowExecution, error)
}

type geoWorkflowExecRepo struct {
	db *gorm.DB
}

func NewGeoWorkflowExecutionRepositoryWithDB(db *gorm.DB) GeoWorkflowExecutionRepository {
	return &geoWorkflowExecRepo{db: db}
}

func (r *geoWorkflowExecRepo) Create(ctx context.Context, exec *model.GeoWorkflowExecution) error {
	return r.db.WithContext(ctx).Create(exec).Error
}

func (r *geoWorkflowExecRepo) Update(ctx context.Context, exec *model.GeoWorkflowExecution) error {
	return r.db.WithContext(ctx).Model(&model.GeoWorkflowExecution{}).Where("id = ?", exec.ID).
		Updates(map[string]any{
			"status":       exec.Status,
			"completed_at": exec.CompletedAt,
			"result":       exec.Result,
			"error":        exec.Error,
		}).Error
}

func (r *geoWorkflowExecRepo) GetByID(ctx context.Context, id string) (*model.GeoWorkflowExecution, error) {
	var exec model.GeoWorkflowExecution
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&exec).Error
	return &exec, err
}

func (r *geoWorkflowExecRepo) GetByWorkflowID(ctx context.Context, workflowID string) ([]*model.GeoWorkflowExecution, error) {
	var execs []*model.GeoWorkflowExecution
	err := r.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Order("started_at DESC").Find(&execs).Error
	return execs, err
}

// GeoWorkflowTemplateRepository GEO 工作流模板仓储接口
type GeoWorkflowTemplateRepository interface {
	Create(ctx context.Context, tpl *model.GeoWorkflowTemplate) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*model.GeoWorkflowTemplate, error)
	GetList(ctx context.Context) ([]*model.GeoWorkflowTemplate, error)
}

type geoWorkflowTplRepo struct {
	db *gorm.DB
}

func NewGeoWorkflowTemplateRepositoryWithDB(db *gorm.DB) GeoWorkflowTemplateRepository {
	return &geoWorkflowTplRepo{db: db}
}

func (r *geoWorkflowTplRepo) Create(ctx context.Context, tpl *model.GeoWorkflowTemplate) error {
	return r.db.WithContext(ctx).Create(tpl).Error
}

func (r *geoWorkflowTplRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.GeoWorkflowTemplate{}).Error
}

func (r *geoWorkflowTplRepo) GetByID(ctx context.Context, id string) (*model.GeoWorkflowTemplate, error) {
	var tpl model.GeoWorkflowTemplate
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&tpl).Error
	return &tpl, err
}

func (r *geoWorkflowTplRepo) GetList(ctx context.Context) ([]*model.GeoWorkflowTemplate, error) {
	var tpls []*model.GeoWorkflowTemplate
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&tpls).Error
	return tpls, err
}

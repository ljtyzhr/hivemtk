package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// UpgradeTaskRepository 升级任务仓库
type UpgradeTaskRepository struct {
	db *gorm.DB
}

// NewUpgradeTaskRepository 创建升级任务仓库
func NewUpgradeTaskRepository() *UpgradeTaskRepository {
	return &UpgradeTaskRepository{
		db: _db.GetDB(),
	}
}

// Create 创建升级任务
func (r *UpgradeTaskRepository) Create(ctx context.Context, task *model.UpgradeTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID 根据 ID 获取升级任务
func (r *UpgradeTaskRepository) GetByID(ctx context.Context, id uint) (*model.UpgradeTask, error) {
	var task model.UpgradeTask
	err := r.db.WithContext(ctx).First(&task, id).Error
	return &task, err
}

func (r *UpgradeTaskRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.UpgradeTask, int64, error) {
	// 分页夹取：controller 侧 pagination.Parse 已夹，仓储侧自设防 ——
	// `pageSize = 0` ⇒ GORM 的 Limit(0) 等于**不加 limit**，`page = 0` ⇒ offset 为负。
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

// Count 与 Find 必须各起一条 query（见 base()），不能在同一条链上先 Count 再 Find；
// Count 失败时若不检查错误，total=0，管理页会把「有历史任务」显示成空列表。
base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.UpgradeTask{})
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tasks []*model.UpgradeTask
	err := base().Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

// UpdateStatus 更新升级任务状态
func (r *UpgradeTaskRepository) UpdateStatus(ctx context.Context, id uint, status string, progress int, currentStep int, stepDesc string, errorMessage string) error {
	updates := map[string]any{
		"status":            status,
		"progress":          progress,
		"current_step":      currentStep,
		"current_step_desc": stepDesc,
		"error_message":     errorMessage,
	}
	if status == "completed" {
		now := time.Now()
		updates["completed_at"] = now
	}
	return r.db.WithContext(ctx).Model(&model.UpgradeTask{}).Where("id = ?", id).Updates(updates).Error
}

// MigrationRecordRepository 迁移记录仓库
type MigrationRecordRepository struct {
	db *gorm.DB
}

// NewMigrationRecordRepository 创建迁移记录仓库
func NewMigrationRecordRepository() *MigrationRecordRepository {
	return &MigrationRecordRepository{
		db: _db.GetDB(),
	}
}

// Create 创建迁移记录
func (r *MigrationRecordRepository) Create(ctx context.Context, record *model.MigrationRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *MigrationRecordRepository) GetAll(ctx context.Context) ([]*model.MigrationRecord, error) {
	var records []*model.MigrationRecord
	err := r.db.WithContext(ctx).Order("executed_at DESC").Find(&records).Error
	return records, err
}

// GetByVersion 按版本获取迁移记录
func (r *MigrationRecordRepository) GetByVersion(ctx context.Context, version string) (*model.MigrationRecord, error) {
	var record model.MigrationRecord
	err := r.db.WithContext(ctx).Where("version = ?", version).First(&record).Error
	return &record, err
}

func (r *MigrationRecordRepository) GetExecutedVersions(ctx context.Context) ([]string, error) {
	var versions []string
	err := r.db.WithContext(ctx).Model(&model.MigrationRecord{}).
		Where("status = ?", "completed").
		Pluck("version", &versions).Error
	return versions, err
}

// Update 整行回写迁移记录。
//
// 走 db.SaveWholeRow：裸 Save 命中 0 行会回退成 INSERT。唯一调用方 Rollback
// 在 GetByVersion 之后改状态，若记录在两步之间被清表，裸 Save 会把状态行按原 id
// 插回——迁移簿记被伪造出一条「已回滚」的记录。
func (r *MigrationRecordRepository) Update(ctx context.Context, record *model.MigrationRecord) error {
	if record == nil || record.ID == 0 {
		return errors.New("migration record: 缺主键，不能整行回写")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), record)
}

// MigrationCheckpointRepository 迁移检查点仓库
type MigrationCheckpointRepository struct {
	db *gorm.DB
}

// NewMigrationCheckpointRepository 创建迁移检查点仓库
func NewMigrationCheckpointRepository() *MigrationCheckpointRepository {
	return &MigrationCheckpointRepository{
		db: _db.GetDB(),
	}
}

// GetByCheckpoint 按检查点名称获取
func (r *MigrationCheckpointRepository) GetByCheckpoint(ctx context.Context, checkpoint string) (*model.MigrationCheckpoint, error) {
	var cp model.MigrationCheckpoint
	err := r.db.WithContext(ctx).Where("checkpoint = ?", checkpoint).First(&cp).Error
	return &cp, err
}

func (r *MigrationCheckpointRepository) GetAll(ctx context.Context) ([]*model.MigrationCheckpoint, error) {
	var checkpoints []*model.MigrationCheckpoint
	err := r.db.WithContext(ctx).Find(&checkpoints).Error
	return checkpoints, err
}

// Upsert 创建或更新检查点。
//
// 走 db.SaveWholeRow：裸 Save 命中 0 行会回退成 INSERT。
//
// 已知竞态（记录不修）：read-then-write 窗口里同一 checkpoint 被并发 upsert
// 会撞 checkpoint 唯一索引（一方报错）或双写两行；checkpoint 链当前零生产
// 调用方，接线时应改 `ON CONFLICT (checkpoint) DO UPDATE` 一条语句。
func (r *MigrationCheckpointRepository) Upsert(ctx context.Context, checkpoint *model.MigrationCheckpoint) error {
	existing, err := r.GetByCheckpoint(ctx, checkpoint.Checkpoint)
	if err == nil && existing != nil {
		checkpoint.ID = existing.ID
		return _db.SaveWholeRow(r.db.WithContext(ctx), checkpoint)
	}
	return r.db.WithContext(ctx).Create(checkpoint).Error
}

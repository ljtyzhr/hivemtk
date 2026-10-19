package service

import (
	"context"
	"os"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/storage"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InitDefaultStorageIfEmpty 启动时检查 obs_config 表是否为空，为空则 seed 一条默认 local 配置。
//
// 私有化部署零云依赖的核心保证：无论用户有没有手动配置，系统都能立即可用。
// 幂等：只在 count == 0 时 seed；后续启动不会重复插入。
func InitDefaultStorageIfEmpty(gdb *gorm.DB) {
	if gdb == nil {
		logger.Warn("[OBS] InitDefaultStorageIfEmpty: gdb is nil, skip")
		return
	}

	// 平台级启动期操作：obs_config 是租户表，TENANT_MODE=on 时无租户 ctx 的 count
	//（query）与下面的 seed Create 会被隔离 callback fail-closed（生产日志实证：
	// "[OBS] InitDefaultStorageIfEmpty: count failed: [tenant] 表 obs_config 需要租户
	// 上下文但 ctx 未绑定（query）"）。启动期 seed 没有 HTTP 请求 ctx 可绑，走
	// tenant.WithSystemScope 这条平台级豁免通道；行上显式钉 TenantID=1（与
	// migration 的 default ai_agent 种子同规范，单租户生产数据口径）。
	sysCtx := tenant.WithSystemScope(context.Background())

	var count int64
	if err := gdb.WithContext(sysCtx).Model(&model.ObsConfig{}).Count(&count).Error; err != nil {
		logger.Warnf("[OBS] InitDefaultStorageIfEmpty: count failed: %v", err)
		return
	}
	if count > 0 {
		logger.Infof("[OBS] obs_config already has %d records, skip seed", count)
		return
	}

	baseDir, publicURL, _ := storage.LocalSource()

	cfg := &model.ObsConfig{
		TenantID:  tenant.DefaultTenantID,
		ID:        uuid.New().String(),
		Name:      "默认本地存储",
		Provider:  model.ObsProviderLocal,
		Endpoint:  baseDir,
		Domain:    publicURL,
		IsDefault: true,
		Status:    model.ObsStatusActive,
		MaxSize:   100 * 1024 * 1024,
		MaxCount:  10000,
		AccessKey: "",
		SecretKey: "",
		Bucket:    "",
	}

	if err := gdb.WithContext(sysCtx).Create(cfg).Error; err != nil {
		logger.Errorf("[OBS] InitDefaultStorageIfEmpty: seed failed: %v", err)
		return
	}

	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		logger.Warnf("[OBS] InitDefaultStorageIfEmpty: mkdir %s failed: %v", baseDir, err)
	}

	logger.Infof("[OBS] ✅ seeded default local storage: baseDir=%s, publicURL=%s", baseDir, publicURL)
}

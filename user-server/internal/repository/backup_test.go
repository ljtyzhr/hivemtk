package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupBackupTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Backup{},
		&model.RestoreRecord{},
	)
	db.SetTestDB(database)
	return database
}

func setupBackupRepositories(t *testing.T) (*BackupRepository, *RestoreRecordRepository) {
	setupBackupTestDB(t)
	database := db.GetDB()

	backupRepo := NewBackupRepositoryWithDB(database)
	restoreRepo := NewRestoreRecordRepositoryWithDB(database)

	return backupRepo, restoreRepo
}

// TestBackupRepository_Create 测试创建备份记录
func TestBackupRepository_Create(t *testing.T) {
	backupRepo, _ := setupBackupRepositories(t)
	ctx := context.Background()

	now := time.Now()
	tests := []struct {
		name    string
		backup  *model.Backup
		wantErr bool
	}{
		{
			name: "create full backup success",
			backup: &model.Backup{
				BackupName: "full-backup-1",
				BackupType: model.BackupTypeFull,
				Status:     model.BackupStatusPending,
				FileSize:   1024 * 1024 * 100,
				CreatedBy:  1,
			},
			wantErr: false,
		},
		{
			name: "create incremental backup",
			backup: &model.Backup{
				BackupName: "incremental-backup-1",
				BackupType: model.BackupTypeIncremental,
				Status:     model.BackupStatusRunning,
				StartedAt:  now,
				CreatedBy:  2,
			},
			wantErr: false,
		},
		{
			name: "create backup with file path",
			backup: &model.Backup{
				BackupName: "backup-with-path",
				BackupType: model.BackupTypeFull,
				Status:     model.BackupStatusCompleted,
				FilePath:   "/backups/merchant-1/backup.zip",
				FileSize:   2048,
				CreatedBy:  1,
			},
			wantErr: false,
		},
		{
			name: "create failed backup with error",
			backup: &model.Backup{
				BackupName:   "failed-backup",
				BackupType:   model.BackupTypeFull,
				Status:       model.BackupStatusFailed,
				ErrorMessage: "Disk full",
				CreatedBy:    1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backupRepo.Create(ctx, tt.backup)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.backup.ID == 0 {
				t.Error("Expected backup ID to be set after creation")
			}
		})
	}
}

// TestBackupRepository_GetByID 测试根据 ID 获取备份记录
func TestBackupRepository_GetByID(t *testing.T) {
	backupRepo, _ := setupBackupRepositories(t)
	ctx := context.Background()

	backup := &model.Backup{
		BackupName: "GetByID Test",
		BackupType: model.BackupTypeFull,
		Status:     model.BackupStatusCompleted,
		FileSize:   512,
		CreatedBy:  1,
	}
	backupRepo.Create(ctx, backup)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing backup",
			id:      backup.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing backup",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := backupRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.BackupName != "GetByID Test" {
					t.Errorf("Expected BackupName 'GetByID Test', got '%s'", result.BackupName)
				}
			}
		})
	}
}

// TestBackupRepository_Update 测试更新备份记录
func TestBackupRepository_Update(t *testing.T) {
	backupRepo, _ := setupBackupRepositories(t)
	ctx := context.Background()

	backup := &model.Backup{
		BackupName: "Original Name",
		BackupType: model.BackupTypeFull,
		Status:     model.BackupStatusPending,
		CreatedBy:  1,
	}
	backupRepo.Create(ctx, backup)

	tests := []struct {
		name       string
		updateFunc func(*model.Backup)
		wantErr    bool
	}{
		{
			name: "update status to completed",
			updateFunc: func(b *model.Backup) {
				b.Status = model.BackupStatusCompleted
				now := time.Now()
				b.CompletedAt = &now
			},
			wantErr: false,
		},
		{
			name: "update status to failed with error",
			updateFunc: func(b *model.Backup) {
				b.Status = model.BackupStatusFailed
				b.ErrorMessage = "Backup failed: disk error"
			},
			wantErr: false,
		},
		{
			name: "update file path and size",
			updateFunc: func(b *model.Backup) {
				b.FilePath = "/backups/merchant-1/backup.zip"
				b.FileSize = 1024 * 1024 * 50
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.updateFunc(backup)
			err := backupRepo.Update(ctx, backup)

			if (err != nil) != tt.wantErr {
				t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := backupRepo.GetByID(ctx, backup.ID)
				if updated.Status != backup.Status {
					t.Errorf("Expected status '%s', got '%s'", backup.Status, updated.Status)
				}
			}
		})
	}
}

// TestBackupRepository_Delete 测试删除备份记录
func TestBackupRepository_Delete(t *testing.T) {
	backupRepo, _ := setupBackupRepositories(t)
	ctx := context.Background()

	backup := &model.Backup{
		BackupName: "To Be Deleted",
		BackupType: model.BackupTypeFull,
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	backupRepo.Create(ctx, backup)

	tests := []struct {
		name        string
		id          uint
		wantErr     bool
		shouldExist bool
	}{
		{
			name:        "delete existing backup",
			id:          backup.ID,
			wantErr:     false,
			shouldExist: false,
		},
		{
			name:        "delete non-existing backup",
			id:          99999,
			wantErr:     true,
			shouldExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backupRepo.Delete(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.name == "delete existing backup" {
				_, err := backupRepo.GetByID(ctx, tt.id)
				if err == nil {
					t.Error("Expected backup to be deleted")
				}
			}
		})
	}
}

// TestBackupRepository_UpdatePreservesImmutableFields 钉住 Update 白名单。
//
// 老实现用 Save() 写整行，调用方手里那份结构体只要缺列，就会把备份名、
// 类型、创建人这些不该由状态更新改动的列静默覆盖成零值。Update 现在只写
// 显式白名单，本测试确保「传进来什么脏值都不会落库」。
func TestBackupRepository_UpdatePreservesImmutableFields(t *testing.T) {
	backupRepo, _ := setupBackupRepositories(t)
	ctx := context.Background()

	backup := &model.Backup{
		BackupName: "immutable-name",
		BackupType: model.BackupTypeFull,
		Status:     model.BackupStatusPending,
		CreatedBy:  7,
	}
	if err := backupRepo.Create(ctx, backup); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	mutated := &model.Backup{
		ID:         backup.ID,
		BackupName: "",
		BackupType: model.BackupTypeIncremental,
		Status:     model.BackupStatusCompleted,
		CreatedBy:  0,
	}
	if err := backupRepo.Update(ctx, mutated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := backupRepo.GetByID(ctx, backup.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.BackupName != "immutable-name" {
		t.Errorf("BackupName 被 Update 覆盖: want %q, got %q", "immutable-name", got.BackupName)
	}
	if got.BackupType != model.BackupTypeFull {
		t.Errorf("BackupType 被 Update 覆盖: want %q, got %q", model.BackupTypeFull, got.BackupType)
	}
	if got.CreatedBy != 7 {
		t.Errorf("CreatedBy 被 Update 覆盖: want 7, got %d", got.CreatedBy)
	}
	if got.Status != model.BackupStatusCompleted {
		t.Errorf("Status 未更新: want %q, got %q", model.BackupStatusCompleted, got.Status)
	}
}

// TestBackupRepository_UpdateNotFound 已删除的备份不能被 Update 复活。
func TestBackupRepository_UpdateNotFound(t *testing.T) {
	backupRepo, _ := setupBackupRepositories(t)
	ctx := context.Background()

	err := backupRepo.Update(ctx, &model.Backup{ID: 99999, Status: model.BackupStatusCompleted})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("Update() error = %v, want %v", err, gorm.ErrRecordNotFound)
	}
}

// TestRestoreRecordRepository_Create 测试创建恢复记录
func TestRestoreRecordRepository_Create(t *testing.T) {
	_, restoreRepo := setupBackupRepositories(t)
	ctx := context.Background()

	now := time.Now()
	tests := []struct {
		name    string
		record  *model.RestoreRecord
		wantErr bool
	}{
		{
			name: "create restore record success",
			record: &model.RestoreRecord{
				BackupID:   1,
				BackupName: "backup-to-restore",
				Status:     "pending",
				RestoredAt: now,
				CreatedBy:  1,
			},
			wantErr: false,
		},
		{
			name: "create running restore",
			record: &model.RestoreRecord{
				BackupID:   2,
				BackupName: "backup-running",
				Status:     "running",
				CreatedBy:  2,
			},
			wantErr: false,
		},
		{
			name: "create failed restore with error",
			record: &model.RestoreRecord{
				BackupID:     3,
				BackupName:   "backup-failed",
				Status:       "failed",
				ErrorMessage: "Restore failed: corrupted backup",
				CreatedBy:    1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := restoreRepo.Create(ctx, tt.record)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.record.ID == 0 {
				t.Error("Expected restore record ID to be set after creation")
			}
		})
	}
}

// TestRestoreRecordRepository_GetByID 测试根据 ID 获取恢复记录
func TestRestoreRecordRepository_GetByID(t *testing.T) {
	_, restoreRepo := setupBackupRepositories(t)
	ctx := context.Background()

	record := &model.RestoreRecord{
		BackupID:   1,
		BackupName: "GetByID Restore",
		Status:     "completed",
	}
	restoreRepo.Create(ctx, record)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing restore record",
			id:      record.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing restore record",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := restoreRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.BackupName != "GetByID Restore" {
					t.Errorf("Expected BackupName 'GetByID Restore', got '%s'", result.BackupName)
				}
			}
		})
	}
}

// TestRestoreRecordRepository_Update 测试更新恢复记录
func TestRestoreRecordRepository_Update(t *testing.T) {
	_, restoreRepo := setupBackupRepositories(t)
	ctx := context.Background()

	record := &model.RestoreRecord{
		BackupID:   1,
		BackupName: "Original Restore",
		Status:     "pending",
	}
	restoreRepo.Create(ctx, record)

	tests := []struct {
		name       string
		updateFunc func(*model.RestoreRecord)
		wantErr    bool
	}{
		{
			name: "update status to completed",
			updateFunc: func(r *model.RestoreRecord) {
				r.Status = "completed"
			},
			wantErr: false,
		},
		{
			name: "update status to failed with error",
			updateFunc: func(r *model.RestoreRecord) {
				r.Status = "failed"
				r.ErrorMessage = "Restore failed"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.updateFunc(record)
			err := restoreRepo.Update(ctx, record)

			if (err != nil) != tt.wantErr {
				t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := restoreRepo.GetByID(ctx, record.ID)
				if updated.Status != record.Status {
					t.Errorf("Expected status '%s', got '%s'", record.Status, updated.Status)
				}
			}
		})
	}
}

// TestRestoreRecordRepository_GetLastRestore 测试获取最近一次恢复记录
func TestRestoreRecordRepository_GetLastRestore(t *testing.T) {
	_, restoreRepo := setupBackupRepositories(t)
	ctx := context.Background()

	restoreRepo.Create(ctx, &model.RestoreRecord{
		BackupID:   1,
		BackupName: "First Restore",
		Status:     "completed",
	})
	restoreRepo.Create(ctx, &model.RestoreRecord{
		BackupID:   2,
		BackupName: "Second Restore",
		Status:     "completed",
	})
	restoreRepo.Create(ctx, &model.RestoreRecord{
		BackupID:   3,
		BackupName: "Last Restore",
		Status:     "completed",
	})

	result, err := restoreRepo.GetLastRestore(context.Background())
	if err != nil {
		t.Errorf("GetLastRestore() unexpected error = %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.BackupName != "Last Restore" {
		t.Errorf("Expected BackupName 'Last Restore', got '%s'", result.BackupName)
	}
}

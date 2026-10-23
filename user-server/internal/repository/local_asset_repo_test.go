package repository

import (
	"context"
	"sync"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupLocalAssetTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t, &model.LocalAsset{})
	db.SetTestDB(database)
	return database
}

func setupLocalAssetRepo(t *testing.T) *localAssetRepo {
	setupLocalAssetTestDB(t)
	return &localAssetRepo{db: db.GetDB()}
}

func seedLocalAsset(t *testing.T, r *localAssetRepo, assetID string, useCount, reportedCount int64) int64 {
	t.Helper()
	pid := int64(1001)
	la := &model.LocalAsset{
		AssetID:          assetID,
		AssetType:        "agent_persona",
		Industry:         "美妆",
		Name:             "test-asset",
		Version:          "1.0.0",
		Source:           model.AssetSourcePurchased,
		IsActive:         true,
		PurchaseID:       &pid,
		UseCount:         useCount,
		ReportedUseCount: reportedCount,
	}
	if err := r.db.Create(la).Error; err != nil {
		t.Fatalf("seed local asset: %v", err)
	}
	return la.ID
}

// TestIncrementUseCount_Concurrent 并发累加 use_count 不丢更新
func TestIncrementUseCount_Concurrent(t *testing.T) {
	r := setupLocalAssetRepo(t)
	ctx := context.Background()
	id := seedLocalAsset(t, r, "asset-6", useCount(0), reportedCount(0))

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := r.IncrementUseCount(ctx, id, 1); err != nil {
				t.Errorf("increment: %v", err)
			}
		}()
	}
	wg.Wait()

	var got model.LocalAsset
	if err := r.db.First(&got, id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.UseCount != N {
		t.Fatalf("expected use_count=%d, got %d", N, got.UseCount)
	}
}

func useCount(v int64) int64      { return v }
func reportedCount(v int64) int64 { return v }

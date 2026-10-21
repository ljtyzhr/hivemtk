package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAfterSaleRepo(t *testing.T) (*AfterSaleRepository, context.Context) {
	t.Helper()
	db := testutil.NewTestDB(t, &model.AfterSale{})
	return NewAfterSaleRepositoryWithDB(db), context.Background()
}

func newTestAfterSale() *model.AfterSale {
	return &model.AfterSale{
		Platform:      "taobao",
		OrderID:       "TB-20260831-001",
		CustomerPhone: "13800138000",
		CustomerName:  "张三",
		Type:          "refund",
		Reason:        "商品与描述不符",
		Amount:        9900,
		Status:        "pending",
		ExternalID:    "EXT-9999",
	}
}

func TestAfterSale_Create(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	as := newTestAfterSale()
	err := repo.Create(ctx, as)
	require.NoError(t, err)
	assert.NotZero(t, as.ID, "Create 后 ID 应被自动填充")

	list, err := repo.ListByOrder(ctx, as.Platform, as.OrderID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	got := list[0]
	assert.Equal(t, as.Platform, got.Platform)
	assert.Equal(t, as.OrderID, got.OrderID)
	assert.Equal(t, as.CustomerPhone, got.CustomerPhone)
	assert.Equal(t, as.Amount, got.Amount)
	assert.Equal(t, "pending", got.Status)
}

func TestAfterSale_Update(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	as := newTestAfterSale()
	require.NoError(t, repo.Create(ctx, as))

	as.Status = "processing"
	as.ExternalID = "EXT-8888"
	require.NoError(t, repo.Update(ctx, as))

	list, err := repo.ListByOrder(ctx, as.Platform, as.OrderID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "processing", list[0].Status)
	assert.Equal(t, "EXT-8888", list[0].ExternalID)
}

// 更新不存在的售后单不能报成功：调用方（电商回写）会把它当"已落库"。
func TestAfterSale_Update_NonExistent(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	as := newTestAfterSale()
	as.ID = 99999

	err := repo.Update(ctx, as)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestAfterSale_ListByOrder_Match(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	a1 := newTestAfterSale()
	a2 := newTestAfterSale()
	a3 := newTestAfterSale()
	a3.Platform = "jd"
	a3.OrderID = "JD-123"
	require.NoError(t, repo.Create(ctx, a1))
	require.NoError(t, repo.Create(ctx, a2))
	require.NoError(t, repo.Create(ctx, a3))

	list, err := repo.ListByOrder(ctx, "taobao", "TB-20260831-001")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	assert.GreaterOrEqual(t, list[0].ID, list[1].ID)
}

func TestAfterSale_ListByOrder_NoMatch(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	list, err := repo.ListByOrder(ctx, "taobao", "NOT-EXIST")
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

func TestAfterSale_ListByCustomer_WithPhone(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	a1 := newTestAfterSale()
	a2 := newTestAfterSale()
	a3 := newTestAfterSale()
	a3.CustomerPhone = "13900139000"
	require.NoError(t, repo.Create(ctx, a1))
	require.NoError(t, repo.Create(ctx, a2))
	require.NoError(t, repo.Create(ctx, a3))

	list, err := repo.ListByCustomer(ctx, "13800138000")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestAfterSale_ListByCustomer_EmptyPhone_ReturnAll(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	require.NoError(t, repo.Create(ctx, newTestAfterSale()))
	other := newTestAfterSale()
	other.CustomerPhone = "13900139000"
	require.NoError(t, repo.Create(ctx, other))

	list, err := repo.ListByCustomer(ctx, "")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestAfterSale_ListByCustomer_NoMatch(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	require.NoError(t, repo.Create(ctx, newTestAfterSale()))

	list, err := repo.ListByCustomer(ctx, "19900199000")
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

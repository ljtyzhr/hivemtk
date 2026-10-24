// order_draft.go 订单草稿仓储。service 只依赖本接口，不碰 *gorm.DB。
// 刻意没有"写失败就退回内存"这条路：草稿是成单工作数据，静默丢一份比明确报错严重。
package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

// ErrOrderDraftPendingConflict 同一客户同一产品已存在他人先建的 pending 草稿。
var ErrOrderDraftPendingConflict = errors.New("order_draft: 该客户同产品已有 pending 草稿")

// draftWriteColumns 更新时整行覆盖的列（不含 id/created_at）。
// 不用 Select("*")：GORM 会连 created_at 一起写，等于把草稿寿命无限续期。
var draftWriteColumns = []string{
	"customer_id", "one_id", "owner_id", "product_name", "product_id", "category",
	"quantity", "unit_price", "total_amount", "confidence",
	"source", "source_text", "intent_id",
	"status", "order_id", "note", "cancel_reason", "metadata",
	"updated_at", "expires_at", "confirmed_at", "cancelled_at",
}

// OrderDraftRepository 草稿持久化读写接口
type OrderDraftRepository interface {
	// Available 报告是否持有可用 DB 句柄（装配回显用，不用于吞错）。
	Available() bool

	// Upsert 按主键插入或整行更新；同客户同产品已有 pending 行时返回
	// ErrOrderDraftPendingConflict（部分唯一索引 uq_order_draft_pending）。
	Upsert(ctx context.Context, d *model.OrderDraft) error

	// GetByID 按草稿 ID 读取；不存在返回 (nil, nil)。
	GetByID(ctx context.Context, id string) (*model.OrderDraft, error)

	// ListPendingByCustomer 列出某客户仍处于 pending 的草稿（按创建时间升序），
	// SQL 只收窄候选集，匹配规则留在 service 层。
	ListPendingByCustomer(ctx context.Context, customerID string) ([]*model.OrderDraft, error)

	// ListPending 列出未过期的 pending 草稿，排序 = 置信度↓、金额↓、到期时间↑。
	// ownerID 为空 = 不限销售；limit<=0 = 不限条数。
	ListPending(ctx context.Context, ownerID string, now time.Time, limit int) ([]*model.OrderDraft, error)

	// ListByCustomer 某客户的全部草稿（含终态），按创建时间倒序。
	ListByCustomer(ctx context.Context, customerID string) ([]*model.OrderDraft, error)

	// ListByOwner 某销售的全部草稿，pending 优先、其余按创建时间倒序。
	ListByOwner(ctx context.Context, ownerID string) ([]*model.OrderDraft, error)

	// ExpirePendingBatch 在一条事务里锁定一批"已到期且仍 pending"的草稿并翻成 expired，
	// 返回实际被翻转的行（供上层逐条发统计事件）。limit<=0 表示整批不限。
	// FOR UPDATE SKIP LOCKED 防止查到集合与改写行之间夹进一次 Confirm（把刚成交的刷成过期），
	// 并让多副本 worker 各拿一份不相交集合。
	ExpirePendingBatch(ctx context.Context, now time.Time, limit int) ([]*model.OrderDraft, error)

	// MutatePending 在一条事务里锁住某行、确认仍是 pending 后交 fn 就地改，再写回。
	// 返回 false = 行不存在或已不是 pending（fn 未被调用）——并发点确认时落败方拿到 false 而不是"成功"。
	MutatePending(ctx context.Context, id string, fn func(*model.OrderDraft)) (bool, error)

	// PurgeTerminal 删除终态（cancelled/expired）且 updated_at 早于 before 的行，
	// 返回删除行数。pending/confirmed 不在删除集合内，见 model.OrderDraftTerminalStatuses。
	PurgeTerminal(ctx context.Context, before time.Time, limit int) (int64, error)

	// CountByStatus 各状态行数（观测"是否还在无界增长"的唯一入口）。
	CountByStatus(ctx context.Context) (map[string]int64, error)
}

type orderDraftRepo struct {
	db *gorm.DB
}

func NewOrderDraftRepository() OrderDraftRepository {
	return &orderDraftRepo{db: _db.GetDB()}
}

func NewOrderDraftRepositoryWithDB(db *gorm.DB) OrderDraftRepository {
	return &orderDraftRepo{db: db}
}

func (r *orderDraftRepo) Available() bool { return r != nil && r.db != nil }

func (r *orderDraftRepo) require() error {
	if !r.Available() {
		return errors.New("order_draft repository: db handle is nil")
	}
	return nil
}

func (r *orderDraftRepo) Upsert(ctx context.Context, d *model.OrderDraft) error {
	if err := r.require(); err != nil {
		return err
	}
	if d == nil || d.ID == "" {
		return errors.New("order_draft repository: 空草稿或空 ID")
	}
	updates := make([]clause.Assignment, 0, len(draftWriteColumns))
	for _, col := range draftWriteColumns {
		updates = append(updates, clause.Assignment{
			Column: clause.Column{Name: col},
			Value:  gorm.Expr("EXCLUDED." + quoteIdent(col)),
		})
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: updates,
	}).Create(d).Error
	if err == nil {
		return nil
	}
	if isPendingConflict(err) {
		return ErrOrderDraftPendingConflict
	}
	return err
}

// quoteIdent 双引号包裹标识符。输入全部来自包内常量 draftWriteColumns（非用户输入）。
func quoteIdent(s string) string { return `"` + s + `"` }

// isPendingConflict 判定命中 uq_order_draft_pending 而非其他约束（id 主键冲突已被 ON CONFLICT 消化）。
// 不认 gorm.ErrDuplicatedKey：本仓连接未开 TranslateError，拿到的是底层错误串。
func isPendingConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") && strings.Contains(msg, "uq_order_draft_pending")
}

func (r *orderDraftRepo) GetByID(ctx context.Context, id string) (*model.OrderDraft, error) {
	if err := r.require(); err != nil {
		return nil, err
	}
	var d model.OrderDraft
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *orderDraftRepo) ListPendingByCustomer(ctx context.Context, customerID string) ([]*model.OrderDraft, error) {
	if err := r.require(); err != nil {
		return nil, err
	}
	var rows []*model.OrderDraft
	err := r.db.WithContext(ctx).
		Where("customer_id = ? AND status = ?", customerID, model.OrderDraftStatusPending).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	return rows, err
}

func (r *orderDraftRepo) ListPending(ctx context.Context, ownerID string, now time.Time, limit int) ([]*model.OrderDraft, error) {
	if err := r.require(); err != nil {
		return nil, err
	}
	q := r.db.WithContext(ctx).
		Where("status = ? AND expires_at > ?", model.OrderDraftStatusPending, now)
	if ownerID != "" {
		q = q.Where("owner_id = ?", ownerID)
	}
	q = q.Order("confidence DESC, total_amount DESC, expires_at ASC, id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []*model.OrderDraft
	err := q.Find(&rows).Error
	return rows, err
}

func (r *orderDraftRepo) ListByCustomer(ctx context.Context, customerID string) ([]*model.OrderDraft, error) {
	if err := r.require(); err != nil {
		return nil, err
	}
	var rows []*model.OrderDraft
	err := r.db.WithContext(ctx).
		Where("customer_id = ?", customerID).
		Order("created_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}

func (r *orderDraftRepo) ListByOwner(ctx context.Context, ownerID string) ([]*model.OrderDraft, error) {
	if err := r.require(); err != nil {
		return nil, err
	}
	// 拼进 Order 的是本包编译期常量（值 "pending"），不是入参；owner_id 仍走占位符绑定。
	pendingFirst := "CASE WHEN status = '" + model.OrderDraftStatusPending + "' THEN 0 ELSE 1 END"
	var rows []*model.OrderDraft
	err := r.db.WithContext(ctx).
		Where("owner_id = ?", ownerID).
		Order(pendingFirst + ", created_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}

func (r *orderDraftRepo) ExpirePendingBatch(ctx context.Context, now time.Time, limit int) ([]*model.OrderDraft, error) {
	if err := r.require(); err != nil {
		return nil, err
	}
	var flipped []*model.OrderDraft
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND expires_at <= ?", model.OrderDraftStatusPending, now).
			Order("id ASC")
		if limit > 0 {
			q = q.Limit(limit)
		}
		var rows []*model.OrderDraft
		if err := q.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]string, 0, len(rows))
		for _, d := range rows {
			ids = append(ids, d.ID)
		}
		if err := tx.Model(&model.OrderDraft{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":     model.OrderDraftStatusExpired,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
		for _, d := range rows {
			d.Status = model.OrderDraftStatusExpired
			d.UpdatedAt = now
		}
		flipped = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return flipped, nil
}

// MutatePending 一次加锁读 + 判态 + 写回，返回 fn 是否真的落库。
// SELECT ... FOR UPDATE 把读—判—写收进同一行锁，防止两个并发确认都读到 pending 而一单变两单。
func (r *orderDraftRepo) MutatePending(ctx context.Context, id string, fn func(*model.OrderDraft)) (bool, error) {
	if err := r.require(); err != nil {
		return false, err
	}
	applied := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m model.OrderDraft
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if m.Status != model.OrderDraftStatusPending {
			return nil
		}
		if fn != nil {
			fn(&m)
		}
		res := tx.Model(&m).
			Where("id = ? AND status = ?", id, model.OrderDraftStatusPending).
			Select(draftWriteColumns).
			Updates(&m)
		if res.Error != nil {
			return res.Error
		}
		applied = res.RowsAffected == 1
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func (r *orderDraftRepo) PurgeTerminal(ctx context.Context, before time.Time, limit int) (int64, error) {
	if err := r.require(); err != nil {
		return 0, err
	}
	// 先取 id 再删：PG 不支持 DELETE ... LIMIT，分批让单轮清理的耗时与锁范围有上界。
	q := r.db.WithContext(ctx).Model(&model.OrderDraft{}).
		Where("status IN ? AND updated_at < ?", model.OrderDraftTerminalStatuses, before).
		Order("updated_at ASC, id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var ids []string
	if err := q.Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("id IN ? AND status IN ? AND updated_at < ?", ids, model.OrderDraftTerminalStatuses, before).
		Delete(&model.OrderDraft{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *orderDraftRepo) CountByStatus(ctx context.Context) (map[string]int64, error) {
	if err := r.require(); err != nil {
		return nil, err
	}
	type row struct {
		Status string
		N      int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.OrderDraft{}).
		Select("status, COUNT(*) AS n").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, g := range rows {
		out[g.Status] = g.N
	}
	return out, nil
}

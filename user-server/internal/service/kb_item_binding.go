package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// KBItemBindingService 知识库成员绑定服务（D-8 方案 A：kb_item_bindings 成员表）。
//
// 契约：GET/POST/DELETE /api/knowledge-bases/:id/items；不新增 /api/knowledge 顶层域名。
// 绑定前校验 KB 与内容同租户、内容存在；item_count 按绑定计数，
// hit_count 保持"未接入"（无检索命中写入路径）。
type KBItemBindingService struct {
	repo  *repository.KBItemBindingRepository
	kbSvc *KnowledgeBaseService
}

func NewKBItemBindingService(kbSvc *KnowledgeBaseService, repo *repository.KBItemBindingRepository) *KBItemBindingService {
	return &KBItemBindingService{repo: repo, kbSvc: kbSvc}
}

// ErrItemNotFound 内容不存在（或属于别的租户）。
var ErrItemNotFound = errors.New("绑定的内容不存在或不属于当前租户")

// ErrKBNotFound 知识库不存在。
var ErrKBNotFound = errors.New("知识库不存在")

// BindItem 一条待绑定内容。
type BindItem struct {
	Type   string
	ItemID uint64
}

// checkKB 校验知识库存在且属于该租户。
func (s *KBItemBindingService) checkKB(ctx context.Context, tenantID, kbID uint) error {
	kb, err := s.kbSvc.GetKB(ctx, kbID)
	if err != nil {
		return err
	}
	if kb == nil || kb.TenantID != tenantID {
		return ErrKBNotFound
	}
	return nil
}

// Bind 把一条内容绑定进知识库。幂等：重复绑定返回成功不报错。
func (s *KBItemBindingService) Bind(ctx context.Context, tenantID, kbID uint, itemType string, itemID uint64, boundBy string) error {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if !model.IsValidKBItemType(itemType) {
		return fmt.Errorf("item_type 只支持 faq|sop|rag，收到 %q", itemType)
	}
	if err := s.checkKB(ctx, tenantID, kbID); err != nil {
		return err
	}
	ok, err := s.repo.ItemExists(ctx, tenantID, itemType, itemID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrItemNotFound
	}
	exists, err := s.repo.Exists(ctx, tenantID, kbID, itemType, itemID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.repo.Create(ctx, &model.KBItemBinding{
		TenantID: tenantID,
		KBID:     kbID,
		ItemType: itemType,
		ItemID:   itemID,
		BoundBy:  boundBy,
	})
}

// BindMany 批量绑定。先整体校验（KB 归属 + 每条内容存在），再逐条写入，
// 避免"前半批绑上了、后半批报错"的半成品响应。
func (s *KBItemBindingService) BindMany(ctx context.Context, tenantID, kbID uint, items []BindItem, boundBy string) (int, error) {
	if len(items) == 0 {
		return 0, fmt.Errorf("至少需要一条待绑定内容")
	}
	if err := s.checkKB(ctx, tenantID, kbID); err != nil {
		return 0, err
	}
	for i, it := range items {
		it.Type = strings.ToLower(strings.TrimSpace(it.Type))
		if !model.IsValidKBItemType(it.Type) {
			return 0, fmt.Errorf("第 %d 条 item_type 只支持 faq|sop|rag，收到 %q", i+1, it.Type)
		}
		ok, err := s.repo.ItemExists(ctx, tenantID, it.Type, it.ItemID)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, fmt.Errorf("%w（第 %d 条 %s/%d）", ErrItemNotFound, i+1, it.Type, it.ItemID)
		}
		items[i] = it
	}
	bound := 0
	for _, it := range items {
		if err := s.Bind(ctx, tenantID, kbID, it.Type, it.ItemID, boundBy); err != nil {
			return bound, err
		}
		bound++
	}
	return bound, nil
}

// Unbind 解绑。幂等：未绑定时同样返回成功。
func (s *KBItemBindingService) Unbind(ctx context.Context, tenantID, kbID uint, itemType string, itemID uint64) error {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if !model.IsValidKBItemType(itemType) {
		return fmt.Errorf("item_type 只支持 faq|sop|rag，收到 %q", itemType)
	}
	_, err := s.repo.Delete(ctx, tenantID, kbID, itemType, itemID)
	return err
}

// ListItems 列出某 KB 的绑定成员，附带内容标题。
//
// 内容被删后绑定行不会自动消失（多态 item_id 无外键），本方法在读取时做一次
// 一致性修复：本页里内容已不存在（软删/硬删）的绑定行就地清理，并从 total 里扣除，
// 避免抽屉里出现"空标题幽灵条目"把 item_count 越撑越大。
func (s *KBItemBindingService) ListItems(ctx context.Context, tenantID, kbID uint, itemType string, page, pageSize int) ([]map[string]any, int64, error) {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if itemType != "" && !model.IsValidKBItemType(itemType) {
		return nil, 0, fmt.Errorf("item_type 只支持 faq|sop|rag，收到 %q", itemType)
	}
	bindings, total, err := s.repo.ListByKB(ctx, tenantID, kbID, itemType, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	idsByType := map[string][]uint64{}
	for _, b := range bindings {
		idsByType[b.ItemType] = append(idsByType[b.ItemType], b.ItemID)
	}
	titles := map[string]map[uint64]string{}
	for t, ids := range idsByType {
		m, err := s.repo.TitlesByIDs(ctx, tenantID, t, ids)
		if err != nil {
			return nil, 0, err
		}
		titles[t] = m
	}

	var orphanIDs []uint
	list := make([]map[string]any, 0, len(bindings))
	for _, b := range bindings {
		title, alive := titles[b.ItemType][b.ItemID]
		if !alive {
			orphanIDs = append(orphanIDs, b.ID)
			continue
		}
		list = append(list, map[string]any{
			"id":         b.ID,
			"kb_id":      b.KBID,
			"item_type":  b.ItemType,
			"item_id":    b.ItemID,
			"title":      title,
			"bound_by":   b.BoundBy,
			"created_at": b.CreatedAt,
		})
	}
	if len(orphanIDs) > 0 {
		if err := s.repo.DeleteByIDs(ctx, tenantID, orphanIDs); err != nil {
			return nil, 0, err
		}
		total -= int64(len(orphanIDs))
	}
	return list, total, nil
}

// CountByKB 某 KB 的有效绑定计数（GetKBStats 的 item_count 来源）。
func (s *KBItemBindingService) CountByKB(ctx context.Context, tenantID, kbID uint) (int64, error) {
	return s.repo.CountEffectiveByKB(ctx, tenantID, kbID, "")
}

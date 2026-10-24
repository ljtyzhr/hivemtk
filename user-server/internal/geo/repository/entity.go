package repository

import (
	"context"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GeoEntityRepository 实体 + 关系仓储
type GeoEntityRepository interface {
	Create(ctx context.Context, e *model.GeoEntity) error
	BatchCreate(ctx context.Context, entities []*model.GeoEntity) error
	List(ctx context.Context, search string, entityType string, page, limit int) ([]*model.GeoEntity, int64, error)
	GetRelationGraph(ctx context.Context, entityID uint, depth int) ([]*model.GeoEntityRelation, []*model.GeoEntity, error)
	UpsertRelation(ctx context.Context, rel *model.GeoEntityRelation) error
}

type geoEntityRepo struct{ db *gorm.DB }

func NewGeoEntityRepository() GeoEntityRepository {
	return &geoEntityRepo{db: _db.GetDB()}
}
func NewGeoEntityRepositoryWithDB(db *gorm.DB) GeoEntityRepository {
	return &geoEntityRepo{db: db}
}

func (r *geoEntityRepo) Create(ctx context.Context, e *model.GeoEntity) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *geoEntityRepo) BatchCreate(ctx context.Context, entities []*model.GeoEntity) error {
	if len(entities) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(entities, 100).Error
}

func (r *geoEntityRepo) List(ctx context.Context, search string, entityType string, page, limit int) ([]*model.GeoEntity, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	var list []*model.GeoEntity
	var total int64
	q := r.db.WithContext(ctx).Model(&model.GeoEntity{})
	if search != "" {
		pattern := "%" + escapeLike(search) + "%"
		q = q.Where("name ILIKE ? OR description ILIKE ?", pattern, pattern)
	}
	if entityType != "" {
		q = q.Where("type = ?", entityType)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Offset((page - 1) * limit).Limit(limit).Order("created_at DESC").Find(&list).Error
	return list, total, err
}

func (r *geoEntityRepo) GetRelationGraph(ctx context.Context, entityID uint, depth int) ([]*model.GeoEntityRelation, []*model.GeoEntity, error) {
	if depth <= 0 {
		depth = 1
	}
	seen := map[uint]bool{entityID: true}
	relSeen := map[uint]bool{}
	var rels []*model.GeoEntityRelation
	frontier := []uint{entityID}
	for h := 0; h < depth && len(frontier) > 0; h++ {
		var hop []*model.GeoEntityRelation
		if err := r.db.WithContext(ctx).
			Where("entity_a_id IN ? OR entity_b_id IN ?", frontier, frontier).
			Find(&hop).Error; err != nil {
			return nil, nil, err
		}
		next := []uint{}
		for _, rel := range hop {
			if relSeen[rel.ID] {
				continue
			}
			relSeen[rel.ID] = true
			rels = append(rels, rel)
			for _, id := range []uint{rel.EntityAID, rel.EntityBID} {
				if !seen[id] {
					seen[id] = true
					next = append(next, id)
				}
			}
		}
		frontier = next
	}
	var entities []*model.GeoEntity
	if len(seen) > 1 {
		ids := make([]uint, 0, len(seen))
		for id := range seen {
			ids = append(ids, id)
		}
		if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&entities).Error; err != nil {
			return rels, nil, err
		}
	}
	return rels, entities, nil
}

// UpsertRelation 幂等写入关系：冲突键与唯一索引
// (tenant_id, entity_a_id, entity_b_id, relation) 完全一致。
// 不用 FirstOrCreate：SELECT 后 INSERT 在并发抽取下会各写一行、同一关系出现重复边。
func (r *geoEntityRepo) UpsertRelation(ctx context.Context, rel *model.GeoEntityRelation) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"}, {Name: "entity_a_id"}, {Name: "entity_b_id"}, {Name: "relation"},
		},
		DoNothing: true,
	}).Create(rel).Error
}

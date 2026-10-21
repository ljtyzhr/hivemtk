package service

import (
	"context"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// EntityService 实体图谱读路径（列表 / 关系子图）。
// 写入由 EntityExtractorService 负责；控制器只依赖 service 层。
type EntityService struct {
	repo repository.GeoEntityRepository
}

func NewEntityService(repo repository.GeoEntityRepository) *EntityService {
	return &EntityService{repo: repo}
}

func (s *EntityService) List(ctx context.Context, search, entityType string, page, limit int) ([]*model.GeoEntity, int64, error) {
	return s.repo.List(ctx, search, entityType, page, limit)
}

// Graph 返回实体关系子图；depth 限制在 1..5，缺省 2。
func (s *EntityService) Graph(ctx context.Context, entityID uint, depth int) ([]*model.GeoEntityRelation, []*model.GeoEntity, error) {
	if depth <= 0 || depth > 5 {
		depth = 2
	}
	return s.repo.GetRelationGraph(ctx, entityID, depth)
}

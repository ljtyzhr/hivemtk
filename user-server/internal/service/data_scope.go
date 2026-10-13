package service

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// DataScopeService 行级隔离的归属查询源（供 middleware.DataScopeGuard 注入）。
//
// 本层不碰 DB：五层架构里"读一行"属 L5，SQL 落在 repository.LookupRowOwner。
// 这里只保留**登记表校验**——守卫按 (method, fullPath) 查登记表，登记表若被改坏
// （键与列不成对），在打出去之前拦下比让一条畸形 SQL 打到库里便宜。
type DataScopeService struct{}

// NewDataScopeService 构造。无状态：句柄由仓储层每次现取，
// 构造期快照会让测试里换库/还原库的动作打不到这个查询源（同 repository 层的既有教训）。
func NewDataScopeService() *DataScopeService { return &DataScopeService{} }

// LookupOwner 实现 middleware.OwnerLookupFunc。
func (s *DataScopeService) LookupOwner(ctx context.Context, table, idColumn, ownerColumn, idValue string) (string, bool, error) {
	if _, ok := model.DataScopeRowByID(table, idColumn, ownerColumn); !ok {
		return "", false, errors.New("归属查询收到不在登记表里的表/列组合")
	}
	return repository.LookupRowOwner(ctx, table, idColumn, ownerColumn, idValue)
}

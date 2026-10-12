package service

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// DataScopeService 行级隔离的归属查询源（供 middleware.DataScopeGuard 注入）。
//
// 表名/列名只来自 model 的登记表且已在装配期过标识符白名单，定位值走占位符绑定 ⇒
// 请求内容进不了 SQL 的结构部分。
type DataScopeService struct{}

// NewDataScopeService 构造。不持有 db 句柄：本仓的 db.GetDB() 是包级全局，
// 构造期快照会让测试里换库/还原库的动作打不到这个查询源（同 repository 层的既有教训）。
func NewDataScopeService() *DataScopeService { return &DataScopeService{} }

// LookupOwner 实现 middleware.OwnerLookupFunc。
func (s *DataScopeService) LookupOwner(ctx context.Context, table, idColumn, ownerColumn, idValue string) (string, bool, error) {
	// 再核一次登记表身份：守卫查的是 (method, fullPath)，若登记表被改坏（键与列不成对），
	// 这里拦下比让一条畸形 SQL 打出去便宜。
	if _, ok := model.DataScopeRowByID(table, idColumn, ownerColumn); !ok {
		return "", false, errors.New("归属查询收到不在登记表里的表/列组合")
	}

	var owner *string
	// 归属值一律取回成字符串：守卫的判定是字符串等值比较，而归属列在本仓有 bigint
	// （customer_sessions.agent_id）也有 varchar（按用户名存的那几列）两类。
	// 单列直接扫进 **string 时驱动已按列类型转成十进制/原样字符串（实测 bigint 列回 "4242"），
	// 所以这里不需要在 SQL 里额外 CAST。
	// 用 **string 而不是 string：NULL 归属（未分配）要能扫成功，扫描失败必须是真错误而不是"空归属"。
	//
	// deleted_at 这一条必须自己补：走 Table() 的原生查询不套模型的软删除条件，少了它，
	// 已删除的行仍会带着旧归属被查出来 ⇒ 非归属者收到 403，而业务读口把它当不存在（404），
	// 两道口径不一致时"删掉的会话永远拦着别人"。
	err := db.GetDB().WithContext(ctx).
		Table(table).
		Select(ownerColumn).
		Where(idColumn+" = ?", idValue).
		Where("deleted_at IS NULL").
		Limit(1).
		Scan(&owner).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if owner == nil {
		// Scan 到零行不回错，只把目标留成 nil：这一行不存在。
		return "", false, nil
	}
	return *owner, true, nil
}

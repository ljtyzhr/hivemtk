// data_scope_owner.go 行级隔离的"读一行的归属列"落点。
//
// 这件事在分层里属 L5：一条按 (method, fullPath) 登记出来的行级判据，要先把那一行的
// 归属读回来才能判，而"读库"只有本层能做。所以 service 侧只留登记表校验，SQL 在这里。
//
// 表名/列名不是请求内容：调用方（service.DataScopeService）已把三元组核过
// model.DataScopeRowByID，键集在装配期过标识符白名单；定位值走占位符绑定。
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// LookupRowOwner 读一张表里某一行的归属列值。
//
// 句柄取用走 GetDB() 而不是构造期注入：本仓的测试会换库/还原库，
// 快照会让"换了库之后还查旧库"这类错留在门后面。
//
// 三态不许并成两态：("42", true, nil) 是行在且归属明确；("", false, nil) 是行不在
// （含软删除），调用方按 404 处理；err 非空是 DB 错，调用方必须回 500 而不是当成"没这行"
// ——把库挂了读成"这行不属于你"会让值班去看错的方向。
func LookupRowOwner(ctx context.Context, table, idColumn, ownerColumn, idValue string) (string, bool, error) {
	var owner *string
	// 归属列在本仓有 bigint（customer_sessions.agent_id）和 varchar（按用户名存的那几列）两类，
	// 一律让驱动按列类型转成字符串扫进 **string，不需要在 SQL 里额外 CAST。
	// 用 **string 而不是 string：NULL 归属（未分配）要能扫成功，
	// 扫描失败必须是真错误而不是"空归属"。
	//
	// deleted_at 这一条必须自己补：走 Table() 的原生查询不套模型的软删除条件，少了它，
	// 已删除的行仍会带着旧归属被查出来 ⇒ 非归属者收到 403，而业务读口把它当不存在（404），
	// 两道口径不一致时"删掉的会话永远拦着别人"。
	err := GetDB().WithContext(ctx).
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

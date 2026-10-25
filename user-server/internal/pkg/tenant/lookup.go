package tenant

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ErrAmbiguousTenantKey 同一个业务键在多个租户各有一行——归属无法自证。
//
// 唯一索引租户复合化之后，"业务键全局唯一"不再成立：每租户自动创建的渠道、
// 按租户生成的短码等都可能在 A、B 两个租户各有一行，按业务键 `First()` 取到哪行
// 取决于数据库返回顺序，等于把请求随机指派给某个租户。判据只能取"命中行分属
// 几个租户"，多于一个就必须拒。
var ErrAmbiguousTenantKey = errors.New("租户自证歧义：该业务键在多个租户各有一行")

// SingleTenantByKey 用系统作用域反查"命中该业务键的行归属哪个租户"。
//
// 只取 distinct tenant_id、上限 2 行：够判歧义即可，不把整行数据捞出来。
// 返回值：
//   - (id, nil)    全部命中行同属一个租户，可以直接绑定该租户继续处理
//   - (0, nil)     没有命中行（或命中行的 tenant_id 为 0=平台公共池，不能当上下文租户）
//   - (0, err)     查询失败，或多租户共用同一业务键（ErrAmbiguousTenantKey 系）
//
// 调用方必须把 err 当"拒绝"处理，绝不能"取第一个候选"兜底。
func SingleTenantByKey(ctx context.Context, db *gorm.DB, table, where string, args ...any) (uint, error) {
	if db == nil {
		return 0, errors.New("SingleTenantByKey: db 未初始化")
	}
	if table == "" {
		return 0, errors.New("SingleTenantByKey: table 为空")
	}
	var ids []uint
	err := db.WithContext(WithSystemScope(ctx)).
		Table(table).
		Distinct("tenant_id").
		Where(where, args...).
		Limit(2).
		Pluck("tenant_id", &ids).Error
	if err != nil {
		return 0, fmt.Errorf("反查 %s 租户归属失败: %w", table, err)
	}
	switch len(ids) {
	case 0:
		return 0, nil
	case 1:
		// 命中行全在平台公共池（tenant_id=0）时，这里返回的就是「解析不出归属」：
		// 0 既不是可用租户，也不许被调用方当成默认租户继续写。
		return ids[0], nil
	default:
		return 0, fmt.Errorf("%w（表 %s 条件 %s 至少命中租户 %d 与 %d）",
			ErrAmbiguousTenantKey, table, where, ids[0], ids[1])
	}
}

package migrations

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// MenuGrantCeilingPruneMigration v3.55.0：把注册表已经收归超管的菜单从
// customer_service / staff 的分配行里摘掉（role_menu_permissions 收敛到上界之内）。
//
// 为什么要在库里摘，而不是只靠读侧的交集：
//
//	读侧（model.EligibleMenuKeys 那层交集）已经让越界行**当下**不生效了，但行还躺在表里。
//	留着的那一行是一颗延时引信——注册表日后把某个节点重新放开给 staff 时（改一行 Roles
//	词表，没有任何人点过"授权"），那颗历史行会立刻生效，界面上凭空多出一项谁也想不起
//	是哪次批的。这类"改注册表 = 静默复权"没有审计痕迹，比当初多给一个菜单更难查。
//
//	摘掉之后，重新放开一个节点必须走管理台的显式授权（service.MenuService.UpdateRoleMenuKeys
//	写侧也会拒绝上界之外的 key），复权这条路只剩一条，且那条有操作日志。
//
// 这不是空跑，但对读数是零变化：在有存量越界行的库上它会真删行（开发库实测摘除两档各 70 行，
// 摘完整表与上界集合逐字相同），而被摘的行本来就在读侧交集（库行 ∩ 上界）之外——不参与侧栏、
// 不参与授权回显、不参与能力面守卫的判定。唯一可见的差别是摘完后整表若为空，读侧会按出厂分配
// 重新 seed（与全新部署同一条路，不是新行为）。
//
// 只动有读家的角色：customer_service / staff 这两档是 service.MenuService 真会去查的。
// 表里如果出现别的角色码（历史遗留的 manager / sales / viewer，或已归一的 user），
// 本迁移**不删**，只警告：那些行今天没有任何读路径会取到，删它们属于"未证伪就动手"；
// 而且一旦哪天真加了同名的自定义角色，被删掉的授权是找不回来的。
type MenuGrantCeilingPruneMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*MenuGrantCeilingPruneMigration)(nil)

func NewMenuGrantCeilingPruneMigration(db *gorm.DB) *MenuGrantCeilingPruneMigration {
	return &MenuGrantCeilingPruneMigration{db: db}
}

func (m *MenuGrantCeilingPruneMigration) Version() string { return "v3.55.0" }

func (m *MenuGrantCeilingPruneMigration) Name() string {
	return "收敛菜单授权上界（摘除注册表已收归超管的分配行）"
}

func (m *MenuGrantCeilingPruneMigration) Description() string {
	return "删除 role_menu_permissions 中 customer_service/staff 两档里不在 model.EligibleMenuKeys " +
		"之内的行，并把摘除清单打进启动日志；其他角色码的行只报告不删除"
}

// prunedRow 一行待摘除的分配（列名与表一致，日志里要按 role+key 逐条打印）。
type prunedRow struct {
	RoleCode string `gorm:"column:role_code"`
	MenuKey  string `gorm:"column:menu_key"`
}

func (m *MenuGrantCeilingPruneMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !m.db.WithContext(ctx).Migrator().HasTable("role_menu_permissions") {
		// 表都还没建（首启动时 AutoMigrate 之后才会跑迁移，正常不会走到这里）：
		// 没有存量分配，自然没有可收敛的东西。
		return nil
	}

	for _, role := range []string{model.SystemRoleCodeCustomerService, model.SystemRoleCodeStaff} {
		if err := m.pruneRole(ctx, role); err != nil {
			return err
		}
	}
	m.reportForeignRoleCodes(ctx)
	return nil
}

// pruneRole 摘掉某角色上界之外的分配行。
func (m *MenuGrantCeilingPruneMigration) pruneRole(ctx context.Context, role string) error {
	eligible := model.EligibleMenuKeys(role)
	keys := make([]string, 0, len(eligible))
	for k := range eligible {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		// 上界读成空集只可能是注册表本身出了事故（编译进来的字面量表，正常不会空）。
		// 这一档不删行的后果今天看不出来：GORM 把空切片渲染成 `menu_key NOT IN (NULL)`，
		// 而 `x NOT IN (NULL)` 在 SQL 里对每行都求值为 NULL，一条也匹配不上 —— 于是"列越界行"
		// 读到 0 行、迁移照常记成功。真正的失效形态是**静默判绿**：注册表读空被写成"已收敛"，
		// 而"删 0 行"这件事整个压在驱动如何渲染空切片上（那不是一个我会去钉的契约）。
		// 所以这里显式收手并报-warning，留给下一次启动再收敛（读侧的交集本来就把越界行挡住了）。
		logger.Warnf("[migration %s] 注册表上界读成空集 role=%s，本轮不收敛（交集那层仍在兜底）",
			m.Version(), role)
		return nil
	}
	sort.Strings(keys)

	var rows []prunedRow
	if err := m.db.WithContext(ctx).Table("role_menu_permissions").
		Select("role_code", "menu_key").
		Where("role_code = ? AND menu_key NOT IN ?", role, keys).
		Order("menu_key").
		Scan(&rows).Error; err != nil {
		return fmt.Errorf("list out-of-ceiling menu grants for %s failed: %w", role, err)
	}
	if len(rows) == 0 {
		return nil
	}

	prunedKeys := make([]string, 0, len(rows))
	for _, r := range rows {
		prunedKeys = append(prunedKeys, r.MenuKey)
	}

	// 删与数在同一事务里，避免"日志说摘了 70 行、库里其实只少了 3 行"这种对不上账的读数。
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.RoleMenuPermission{}).
			Where("role_code = ? AND menu_key NOT IN ?", role, keys).
			Delete(&model.RoleMenuPermission{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != int64(len(rows)) {
			return fmt.Errorf("prune role_menu_permissions(%s): listed %d rows, deleted %d",
				role, len(rows), res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("prune out-of-ceiling menu grants failed: %w", err)
	}

	logger.Infof("[migration %s] 摘除越界菜单授权 role=%s 共 %d 行: %s",
		m.Version(), role, len(prunedKeys), strings.Join(prunedKeys, ", "))
	return nil
}

// reportForeignRoleCodes 把本迁移刻意不动的角色码念出来：它们今天没有读家，
// 但"没有读家"这件事是我在这一版代码里读出来的，不是库里的约束。留个名，
// 下次有人真加自定义角色时能立刻看出这张表里还埋着旧词表。
func (m *MenuGrantCeilingPruneMigration) reportForeignRoleCodes(ctx context.Context) {
	var codes []string
	// 走 Raw 而不是 Table().Pluck()：Pluck 会用自己的列名顶掉 Select 里的 DISTINCT，
	// 那样同一个角色码会被念好几遍，读日志的人以为表里躺着好几个陌生角色。
	err := m.db.WithContext(ctx).Raw(
		`SELECT DISTINCT role_code
		   FROM role_menu_permissions
		  WHERE role_code NOT IN (?, ?, ?)
		  ORDER BY role_code`,
		model.SystemRoleCodeAdmin, model.SystemRoleCodeCustomerService, model.SystemRoleCodeStaff,
	).Scan(&codes).Error
	if err != nil {
		// 只是报告，读失败不影响已经完成的收敛动作本身，也不该把整个迁移判失败。
		logger.Warnf("[migration %s] 盘点非三档角色码失败（不影响收敛结果）: %v", m.Version(), err)
		return
	}
	if len(codes) > 0 {
		logger.Warnf("[migration %s] 表里还有非三档角色码的分配行，本迁移不删: %s",
			m.Version(), strings.Join(codes, ", "))
	}
}

// Down 不回填：被摘掉的行是"注册表已经不授予该角色"的授权，回填等于把越权授权重新写回库里，
// 而读侧的上界交集会照样把它抹掉 —— 回滚出来的是一份看上去有、实际不生效的分配表，
// 比空表更难解释。真要恢复某个节点的可见性，正确动作是改注册表 + 管理台显式授权。
func (m *MenuGrantCeilingPruneMigration) Down(ctx context.Context) error {
	return nil
}

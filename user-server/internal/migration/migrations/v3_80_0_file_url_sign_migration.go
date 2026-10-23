package migrations

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/storage"

	"gorm.io/gorm"
)

// FileURLSignMigration v3.80.0（SaaS 审计 A1 / A12）：把库里存量的"明文租户段文件 URL"
// 重签成新形态。
//
// 背景：on 模式的上传 URL 以前是 `/files/tenant_5/materials/2026/10/<uuid>.pdf` ——
// 租户 id 明文在 URL 里（A12），且无凭证访问一律放行（A1 的残留缺口）。本轮的修法把
// 租户归属与过期时间搬进 `?s=` 的签名 token、URL 路径里不再有租户段，`/files` 在 on 模式下
// **只认签名**。于是存量行里的老 URL 会 404 —— 这正是 fail-closed 的代价，必须由本迁移
// 一次性抹平：就地重签成 `/files/<相对路径>?s=<token>`。
//
// 登记而非扫全库（与 v3.79 的 tenantLeadingIndexTables 同一取舍）：迁移必须对同一份代码
// 给出同一组动作；"扫全库找所有像 URL 的列"会在不同环境做不同事，且不可回滚推定。
// 这里登记的是"`storage.PublicURL` 的输出会被写进去的那几列"——
// 也就是五个上传调用点（controller/upload.go、content/material.go、service/channel_media.go、
// service/obs_config.go、browser_automation/feedback.go）与前端把上传结果存回业务表的那些列。
//
// 幂等：只匹配 `^/files/tenant_<数字>/`，签名后的 URL 不含该段 ⇒ 重跑无动作。
// 无法签名时（on 模式缺 FILE_URL_SECRET/JWT 密钥）**不报错也不改写**，只响亮记一条 ERROR：
// 把"密钥没配"变成迁移失败会卡住整个升级，而数据一行都没坏。
type FileURLSignMigration struct {
	db      *gorm.DB
	targets []fileURLTarget
}

// fileURLTarget 一张表的 URL 列。
type fileURLTarget struct {
	table  string
	column string
}

// fileURLTargets 需要在升级时重签的列（表名、列名，均为真库现名）。
//
// 新增一列存 PublicURL 输出时，请把它加进来：漏掉的表现是"升级后这一列的历史链接全裂"，
// 不会有用例红——所以这张清单本身就是文档。
var fileURLTargets = []fileURLTarget{
	{"materials", "url"},
	{"message_hub", "media_url"},
	{"session_messages", "media_url"},
	{"unified_messages", "media_url"},
	{"unified_replies", "media_url"},
	{"knowledge_documents", "file_url"},
	{"asset_bundles", "cover_image"},
	{"browser_sessions", "final_screenshot_url"},
	{"browser_outreach_receipts", "screenshot_url"},
	{"live_codes", "image_url"},
	{"live_code_qrs", "image_url"},
	{"douyin_cards", "image_url"},
	{"kuaishou_cards", "image_url"},
	{"tiktok_cards", "image_url"},
	{"xianyu_cards", "image_url"},
	{"xiaohongshu_cards", "image_url"},
	{"system_config", "logo_url"},
}

var _ migration.Migration = (*FileURLSignMigration)(nil)

func NewFileURLSignMigration(db *gorm.DB) *FileURLSignMigration {
	return &FileURLSignMigration{db: db, targets: fileURLTargets}
}

// withTargets 仅测试用：把清单换成探针表，验证同一套判据/改写行为。
func (m *FileURLSignMigration) withTargets(targets ...fileURLTarget) *FileURLSignMigration { //nolint:unused //// 仅被 *_test.go 引用
	m.targets = targets
	return m
}

func (m *FileURLSignMigration) Version() string { return "v3.80.0" }

func (m *FileURLSignMigration) Name() string { return "文件 URL 重签（租户段移入签名）" }

func (m *FileURLSignMigration) Description() string {
	return "把存量 /files/tenant_<id>/... 形式的上传 URL 就地重签为 /files/<相对路径>?s=<token>（幂等；off 模式无动作）"
}

// Up 执行重签。返回 nil 不代表"一行都没动"—— 具体动了几行记在日志里。
func (m *FileURLSignMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !tenant.IsMultiTenant() {
		// off 模式：历史上不可能写出带租户段的 URL（TenantPathPrefix 恒为空串）。
		// 不做任何扫描 —— 离线单机的升级路径必须逐字节无副作用。
		return nil
	}
	db := m.db.WithContext(ctx)

	rewritten, skipped, tooLong := 0, 0, 0
	for _, t := range m.targets {
		if !db.Migrator().HasTable(t.table) || !db.Migrator().HasColumn(t.table, t.column) {
			continue // 全新部署尚未建表 / 列已改名：与本迁移无关
		}
		n, s, l, err := m.rewriteColumn(ctx, t)
		if err != nil {
			return err
		}
		rewritten += n
		skipped += s
		tooLong += l
	}
	logger.Infof("[Migration] v3.80.0 文件 URL 重签：改写 %d 行，跳过 %d 行（无签名器/形状不符），跳过 %d 行（超列宽）",
		rewritten, skipped, tooLong)
	if skipped > 0 {
		logger.Warnf("[Migration] v3.80.0 有 %d 行存量 URL 未能重签：这些链接在 on 模式下会 404。"+
			"常见原因：FILE_URL_SECRET（或 JWT 密钥）未配置 ⇒ 签不出凭证。配好后重跑本迁移即可", skipped)
	}
	m.reportUnregisteredLegacyColumns(ctx)
	return nil
}

// reportUnregisteredLegacyColumns 只读体检：登记清单之外，还有没有列存着老的租户段 URL。
//
// 为什么必须做这一步：登记清单是**人工审过的单值 URL 列**。而通用上传接口
// （`controller/upload.go`）的返回值会被前端存进任意头像/封面/图标列 —— 漏登记的表现是
// "升级后那一列的历史链接全裂"，而且没有任何东西会响。这里把漏登记的事实**说出来**。
//
// 为什么只报告、不就地改：多值列（JSON 数组、逗号分隔的 `images`）用"整列换成一条 URL"去改
// 就是**数据销毁**，比裂链更糟。所以这一档永远不写库，只列出"列名 + 命中行数"，交人判断。
//
// 代价可接受：候选列按列名形态收窄（url/url 类字段），命中判据是 `LIKE '/files/tenant_%'`，
// 且只探一行（`LIMIT 1`）—— 空列的表在 Postgres 上是最便宜的路径。
func (m *FileURLSignMigration) reportUnregisteredLegacyColumns(ctx context.Context) {
	registered := map[string]bool{}
	for _, t := range m.targets {
		registered[t.table+"."+t.column] = true
	}
	for _, c := range m.candidateURLLikeColumns(ctx) {
		key := c.table + "." + c.column
		if registered[key] {
			continue
		}
		n := m.countLegacyRows(ctx, c)
		if n == 0 {
			continue
		}
		logger.Warnf("[Migration] v3.80.0 发现未登记的 URL 列 %s 仍有 %d 行老形态（/files/tenant_<id>/…）："+
			"这些链接在 on 模式下会 404。若它是单值 URL 列，请加进 fileURLTargets 后重跑；"+
			"多值列（JSON/逗号分隔）不能就地整列替换，需按业务迁移", key, n)
	}
}

// candidateURLLikeColumns 按列名形态找出"可能存文件 URL"的文本列（只读用）。
func (m *FileURLSignMigration) candidateURLLikeColumns(ctx context.Context) []fileURLTarget {
	type col struct {
		TableName  string
		ColumnName string
	}
	var cols []col
	err := m.db.WithContext(ctx).Table("information_schema.columns").
		Select("table_name, column_name").
		Where("table_schema = ? AND data_type IN ('text', 'character varying')", "public").
		Where("column_name ~* ?", "(url|avatar|image|cover|logo|media|icon|thumb|photo|banner|qrcode|path)").
		Order("table_name, column_name").
		Scan(&cols).Error
	if err != nil {
		logger.Warnf("[Migration] v3.80.0 未登记 URL 列体检跳过（读取 information_schema 失败）: %v", err)
		return nil
	}
	out := make([]fileURLTarget, 0, len(cols))
	for _, c := range cols {
		out = append(out, fileURLTarget{table: c.TableName, column: c.ColumnName})
	}
	return out
}

// countLegacyRows 数一列里还有多少行是老形态（探到即止的上界口径不追求精确值小，只求"有没有"）。
func (m *FileURLSignMigration) countLegacyRows(ctx context.Context, t fileURLTarget) int64 {
	if !m.db.Migrator().HasTable(t.table) {
		return 0
	}
	var n int64
	if err := m.db.WithContext(tenant.WithSystemScope(ctx)).
		Table(t.table).
		Where(quoteIdent(t.column)+" LIKE ?", legacyFileURLPattern).
		Count(&n).Error; err != nil {
		return 0 // 体检失败不阻断迁移：它是"多说一句"，不是判据
	}
	return n
}

// rewriteColumn 重写一张表的一列，返回（改写数, 跳过数）。
//
// 走 GORM API 而不是 Raw/Exec（specs/001-saas-multi-tenant D2 + scripts/check-gorm-only.sh 门禁：
// 预构建 SQL 引用租户表会被 tenancyidx 直接拒掉）。ctx 用 **SystemScope**：数据迁移天然跨租户
// ——要让一行不落地，迁移得扫所有租户的行；SystemScope 是"明确声明我知道自己在越租户"的
// 唯一合法出口，与 A16 真扫里模拟绕过时用的是同一个开关。
func (m *FileURLSignMigration) rewriteColumn(ctx context.Context, t fileURLTarget) (rewritten, skipped, tooLong int, err error) {
	db := m.db.WithContext(tenant.WithSystemScope(ctx))

	var rows []map[string]any
	sel := "CAST(id AS text) AS row_id, " + quoteIdent(t.column) + " AS row_val"
	if err := db.Table(t.table).
		Select(sel).
		Where(quoteIdent(t.column)+" LIKE ?", legacyFileURLPattern).
		Scan(&rows).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("读取 %s.%s 的存量 URL 失败: %w", t.table, t.column, err)
	}

	maxLen := m.columnMaxLen(t)
	rewritten, skipped, tooLong = 0, 0, 0
	for _, row := range rows {
		rowID := asString(row["row_id"])
		raw := asString(row["row_val"])
		signed, ok := resignLegacyFileURL(raw)
		if !ok || rowID == "" {
			skipped++
			continue
		}
		// 列宽守卫：签名形态比明文长 ~150 字符（nonce + 密文 + 认证标签 + base64）。
		// live_code_qrs.image_url 只有 varchar(255)，某条格外长的路径就会让它装不下 ——
		// 那种行若照写，UPDATE 会因超长报错，**整个升级被一行数据卡住**。这里跳过并出声，
		// 与"缺密钥就不改写、只 WARN"同一取向：迁移不许把可修的数据问题放大成升级失败。
		if maxLen > 0 && len(signed) > maxLen {
			tooLong++
			logger.Warnf("[Migration] v3.80.0 跳过 %s.%s id=%s：签名形态 %d 字符超出列宽 %d。"+
				"该链接在 on 模式下会 404，需人工加宽该列后重跑;orig=%q",
				t.table, t.column, rowID, len(signed), maxLen, raw)
			continue
		}
		if err := db.Table(t.table).
			Where("id = ?", rowID).
			Update(t.column, signed).Error; err != nil {
			return rewritten, skipped, tooLong, fmt.Errorf("回写 %s.%s id=%s 失败: %w", t.table, t.column, rowID, err)
		}
		rewritten++
	}
	return rewritten, skipped, tooLong, nil
}

// columnMaxLen 读列声明的最大长度；不限长或读不到 ⇒ 0（= 不做列宽守卫）。
//
// 读 information_schema 而不是写死一张宽度表：宽度是库的事实，不是本文件的常量
// （写死会在某张表被加宽/收窄之后悄悄失效）。它也不是租户表，隔离 callback 不会注入条件。
func (m *FileURLSignMigration) columnMaxLen(t fileURLTarget) int {
	var widths []int
	err := m.db.Table("information_schema.columns").
		Select("COALESCE(character_maximum_length, 0)").
		Where("table_schema = ? AND table_name = ? AND column_name = ?", "public", t.table, t.column).
		Scan(&widths).Error
	if err != nil || len(widths) == 0 {
		return 0
	}
	return widths[0]
}

// asString 把 Scan 出来的 any 收敛成字符串（驱动可能给 []byte、string 或 nil）。
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

// legacyFileURLPattern 存量形态：`/files/tenant_<数字>/...`（`\_` 是 LIKE 的转义下划线）。
const legacyFileURLPattern = `/files/tenant\_%`

// resignLegacyFileURL 把一条存量 URL 变成签名形态；形状不符或签不出来时 ok=false。
//
// 只认 `/files/tenant_<数字>/<相对路径>`：别的值（云存储绝对地址、外站地址、纯文件名）
// 原样不动 —— 本迁移的职责边界就是"本地驱动写出来的那种形态"。
func resignLegacyFileURL(raw string) (string, bool) {
	const prefix = "/files/"
	v := strings.TrimSpace(raw)
	// 允许带 query（历史上不会有，但别把带参数的存量值当成形状不符而漏掉路径部分）
	if i := strings.IndexByte(v, '?'); i >= 0 {
		v = v[:i]
	}
	if !strings.HasPrefix(v, prefix) {
		return "", false
	}
	storagePath := strings.TrimPrefix(v, prefix)
	rel, sig, ok := storage.SignStoredPath(storagePath)
	if !ok {
		return "", false
	}
	return prefix + rel + "?s=" + sig, true
}

// Down 走 decline 口径：降级不销毁数据。
//
// 重签是"就地覆盖一列的值"，回退等于把 URL 改回明文租户段 —— 那会让新版本签出来的
// 行（不含 tenant 段）与旧版本混在一起，且旧版本在 on 模式下本来就不做租户校验
// （见 files_guard.go 的退役记录），改回去只会把 A1 的洞重新打开。签名 URL 在旧版本下
// 也能被当作普通静态路径请求（旧版本不认 ?s=，会 404）—— 所以正确做法是让运维决定，
// 本迁移只把决定写进日志。
func (m *FileURLSignMigration) Down(ctx context.Context) error {
	logger.Infof("[Migration] v3.80.0 无 Down 动作：重签是把列值换成签名形态，改回明文租户段会重新打开 A1 的缺口；" +
		"如需回滚请按运维决策单独处理（本迁移不销毁任何对象、不改任何形状）")
	return nil
}

// quoteIdent 给标识符加双引号（表名/列名来自本文件的常量，不接受外部输入；
// 加引号只是为了不让 `user`/`order` 这类保留字把 SQL 打歪）。
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// sortedFileURLTargets 供用例断言清单本身（顺序稳定，便于对比）。
func sortedFileURLTargets(targets []fileURLTarget) []string { //nolint:unused //// 仅被 *_test.go 引用，生产路径未用
	out := make([]string, 0, len(targets))
	for _, t := range targets {
		out = append(out, t.table+"."+t.column)
	}
	sort.Strings(out)
	return out
}

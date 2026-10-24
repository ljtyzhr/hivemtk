package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// 这两个号写在迁移里是历史记录，不是从代码常量取的：日后 ClueType 再改号，
// 这一版仍然按"当时那两个号"跑（迁移的语义是发生过什么，不是现在叫什么）。
const (
	// legacyGeoCaptureType 撞号期间 GEO 决策链捕获线索落库的号，与抖音同值。
	legacyGeoCaptureType = 9
	// geoCaptureType 拆分后 GEO 决策链捕获线索的号（1-17 已连续占满，99 留给自定义）。
	geoCaptureType = 18
)

// geoTypeSplitBatch 单批行数：与 v3.44.0 的回填同理由——首次上线时待改判的行数
// 等于 GEO 捕获线索的全量，一条语句吃完就是长事务（锁表）。
const geoTypeSplitBatch = 5000

// ClueGeoCaptureTypeSplitMigration v3.56.0：把 GEO 决策链捕获的线索从「与抖音共用的 9」
// 改判到独立类型号 18，代码侧常量 service.ClueTypeGeoCapture 同批改值。
//
// 为什么必须连存量一起改：只改常量影响的是新写入的行，老库里 GEO 捕获的行仍写着 9，
// 同一个来源被拆成两类 —— 列表页按类型筛选时旧行归抖音、新行归 GEO，比统一错标更难读。
//
// 撞号在三处是实打实的错账，不是显示瑕疵（都按 (type, account) 或 type 取数）：
//  1. 展示：defaultClueTypes 里 9 的标签是「抖音」，GEO 捕获的线索在列表页被叫成抖音；
//  2. 评分：clue_score.go 的 scoreChannel 按 type 取档，GEO 捕获一直吃的是抖音档；
//  3. 去重：repository/clue.go 的判重键是 (type, account)，同一个人被抖音与 GEO 各抓一次时，
//     第二条会被当成重复丢掉 —— 拆分后这两条各自成立。
//
// 判据是 source_id 命中 geo_query_chains.chain_id（chain_id 形如 "verify:<品牌>:<uuid>"
// 或 "probe:..."，与抖音侧写的群号/lead_mining 字面量不同形）。`source_id <> ”` 是刻意
// 加的第二道闸：判据一旦退化成"空串也匹配"，开发库那 162 条 source_id 为空的抖音线索会
// 被整体改判。现测那 58 条链里没有空 chain_id，这一条今天不改变结果，它守的是明天。
//
// 软删行（deleted_at 非空）一起改判：这一行"当初是谁"与它是否已删除无关，只改未删的行
// 会让恢复出来的行显示成抖音。
//
// 改号不改排序：18 的评分档与拆分前吃的抖音档同为 65，所以本迁移不会让任何一条线索的
// 分数或温度等级发生变化（level 由评分链路写回，本迁移不碰那两列）。
//
// 落地时开发库的读数是 0 行待改判（type=9 的 162 行 source_id 全空，source_id LIKE 'verify:%'
// 的线索 0 条），也就是说这趟在开发库上是个空跑 —— 它的可判面全在同目录的 _test.go 夹具里，
// 那里同时造了"该改判/不该改判"两侧的行。
type ClueGeoCaptureTypeSplitMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*ClueGeoCaptureTypeSplitMigration)(nil)

func NewClueGeoCaptureTypeSplitMigration(db *gorm.DB) *ClueGeoCaptureTypeSplitMigration {
	return &ClueGeoCaptureTypeSplitMigration{db: db}
}

func (m *ClueGeoCaptureTypeSplitMigration) Version() string { return "v3.56.0" }

func (m *ClueGeoCaptureTypeSplitMigration) Name() string {
	return "clue_geo_capture_type_split"
}

func (m *ClueGeoCaptureTypeSplitMigration) Description() string {
	return "v3.56.0：GEO 决策链捕获线索的类型号从与抖音共用的 9 拆到 18，" +
		"把存量 clues 中 source_id 命中 geo_query_chains.chain_id 的 9 改判为 18"
}

func (m *ClueGeoCaptureTypeSplitMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	d := m.db.WithContext(ctx)
	if !d.Migrator().HasTable("clues") {
		return nil // 表还没建（由 allModels 的 AutoMigrate 负责），此处无物可改判
	}
	if !d.Migrator().HasTable("geo_query_chains") {
		// 思维链表都不在，就没有任何一行线索能被认成 GEO 捕获；
		// 少了这道闸，下面的子查询会直接报错把启动流程带崩。
		return nil
	}

	var moved int64
	// 这个循环能停，靠的是谓词里的 `type = 老号` 把改完的行排除在外。
	// 把那个条件放宽成"任何命中链的行"（比如改成 type <> 老号），Postgres 会对已经是
	// 新号的行再计一次 RowsAffected（值没变也算改），RowsAffected 永远不为 0，
	// 启动流程就在这个循环里挂死 —— 变异测试实测过一次：10 分钟测试超时，无一行 FAIL。
	for {
		res := d.Exec(`UPDATE clues SET type = ?
			WHERE id IN (SELECT id FROM clues
				WHERE type = ? AND source_id <> '' AND source_id IN (SELECT chain_id FROM geo_query_chains)
				ORDER BY id LIMIT ?)`,
			geoCaptureType, legacyGeoCaptureType, geoTypeSplitBatch)
		if res.Error != nil {
			return fmt.Errorf("split GEO capture clue type failed: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			break
		}
		moved += res.RowsAffected
	}
	if moved > 0 {
		logger.Infof("[migration %s] GEO 决策链捕获线索改判 %d 行：type %d → %d",
			m.Version(), moved, legacyGeoCaptureType, geoCaptureType)
	}
	return nil
}

// Down 不改判回去：回滚的判据只能与 Up 同形（source_id 命中链），而它分不清
// "本迁移搬动的行"和"迁移之后新代码按 18 写入的行"，回填会把后者一并折回 9 ——
// 那些行本来没被误标过，回滚反而制造新的错账（并重新与抖音共用判重键）。
// 真要把某个来源改回另一个号，正确动作是按 (type, source_id) 明确点名那一批。
func (m *ClueGeoCaptureTypeSplitMigration) Down(ctx context.Context) error {
	return nil
}

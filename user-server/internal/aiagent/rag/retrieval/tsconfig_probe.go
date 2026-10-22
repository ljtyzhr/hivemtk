// tsconfig_probe.go 探测库里真实存在的文本搜索配置。
//
// 背景（2026-10-22 线上）：`zh_rag` 这个配置**只在装了 zhparser 扩展的库上存在**
// —— 建它的迁移（internal/migration/migrations/rag_hybrid_migration.go:98-106）
// 判据就是"zhparser 装了才建"，而线上镜像 pgvector/pgvector:pg15 不含该扩展。
// 读取侧却把它当"一定存在的首选"无条件先试，于是每次词法召回都先撞一次
//
//	ERROR: text search configuration "zh_rag" does not exist (SQLSTATE 42704)
//
// 才降级到 simple 继续（功能结果是对的，噪音是假的）。
//
// 另一个方向的坑（本文件存在的主要理由）：**不要用"建一个 zh_rag"来消错**。
// PG 的默认解析器把 CJK 当 blank 直接丢掉 ——
//
//	to_tsvector('zh_rag','链路自检 测试')   -- PARSER=default ⇒ 空 tsvector
//	to_tsvector('simple','链路自检 测试')   -- ⇒ '测试':2 '链路自检':1
//
// 真建一个 default 解析器的 zh_rag，触发器里 `IF EXISTS(zh_rag)` 分支就会给
// 所有新写入的中文 chunk 写**空 tsv**，比报错更糟。
//
// 所以正确口径是：**用库里有什么用什么**，而不是假设有什么。
package ragretrieval

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

// tsConfigCandidates 候选文本搜索配置，按优先级排列。
//
// zh_rag 在前：它装了 zhparser 时对中文分词更准（迁移刻意为它铺了
// `ADD MAPPING FOR n,v,a,i,e,l WITH simple`）。
// simple 在后：PG 内置、永远存在，是中文场景的可靠兜底
// （simple 不做词典分词，但按空白/标点切词，CJK 连续段会成为一个词元，
//
//	与触发器写 tsv 时用的口径完全一致 ⇒ 索引与查询同源，@@ 能命中）。
var tsConfigCandidates = []string{"zh_rag", "simple"}

// tsConfigProbe 探测结果缓存。
//
// 缓存粒度按 *gorm.DB 指针：不同库（测试库 / 生产库）各自的配置不同，
// 共用一个全局布尔值会让先跑的库把后跑的库"毒"掉（测试里尤其明显）。
// 配置属启动期/安装期事实（迁移在启动期执行），运行期改动需重启进程才生效 ——
// 与"迁移是启动期装配"的事实源口径一致。
var tsConfigProbe sync.Map // *gorm.DB -> []string

// availableTSConfigs 返回该库里真实存在的候选配置（按 tsConfigCandidates 的优先级）。
//
// 探测方式：查系统表 pg_ts_config —— 它列出该库已创建的全部文本搜索配置。
// 只把**真实存在**的候选返回给调用方，调用方因此永远不会去试一个不存在的配置。
//
// 探测失败（DB 未就绪 / 权限不足 / 系统表不可读）时返回 nil：
// 调用方此时应当走各自的兜底链（ilike / 无词法召回），而不是硬猜一个配置名去撞错。
func availableTSConfigs(ctx context.Context, db *gorm.DB) []string {
	if db == nil {
		return nil
	}
	if cached, ok := tsConfigProbe.Load(db); ok {
		return cached.([]string)
	}

	var names []string
	// 链式 API（本仓 gorm-only 门禁禁业务包出现直连式数据访问，见
	// specs/001-saas-multi-tenant/research.md D2）：跨租户写要靠 GORM 的隔离
	// callback 兜底，直连式写法会绕过那层。
	// 候选表是本包内的常量（不含用户输入），Where 的参数绑定不引入注入面。
	err := db.WithContext(ctx).Table("pg_ts_config").
		Select("cfgname").
		Where("cfgname IN ?", tsConfigCandidates).
		Scan(&names).Error
	if err != nil {
		// 探测不到就当"一个都没有"：调用方的兜底链（simple / ilike）会接手，
		// 硬猜一个配置名只会多一条 42704。
		tsConfigProbe.Store(db, []string(nil))
		return nil
	}

	// 按候选表的优先级排序（SQL 的 IN 查询不保证顺序）。
	present := make(map[string]bool, len(names))
	for _, n := range names {
		present[n] = true
	}
	out := make([]string, 0, len(names))
	for _, c := range tsConfigCandidates {
		if present[c] {
			out = append(out, c)
		}
	}

	tsConfigProbe.Store(db, out)
	return out
}

package repository

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type FAQRepository struct {
	db *gorm.DB
}

func NewFAQRepository(db *gorm.DB) *FAQRepository {
	return &FAQRepository{db: db}
}

// Create 新增 FAQ
//
// 注意: GORM v2 对 bool 零值 (false) 不会写入,会使用 column default。
// 此处用 Select("*") 强制全字段 INSERT,确保 false 也能正确落库。
func (r *FAQRepository) Create(ctx context.Context, entry *model.FAQEntry) error {
	return r.db.WithContext(ctx).Select("*").Create(entry).Error
}

// GetByID 按 ID 查询
func (r *FAQRepository) GetByID(ctx context.Context, id uint) (*model.FAQEntry, error) {
	var entry model.FAQEntry
	if err := r.db.WithContext(ctx).First(&entry, id).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

// ListEnabled 查询所有启用的 FAQ
func (r *FAQRepository) ListEnabled(ctx context.Context, limit int) ([]model.FAQEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("hit_count DESC, confidence DESC, id ASC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// MatchByKeyword 关键词匹配 (Layer1 核心 API) — 兼容旧签名 (agentID=0 表示共享/全局)
//
// 加 agentID 过滤
//   - agentID > 0: 仅匹配该智能体私有 (agent_id=X) + 共享 (agent_id IS NULL)
//   - agentID = 0: 全局共享 (向后兼容, 旧代码不传 agentID 也能跑)
//
// 策略:
//  1. SQL 层先按 (agent_id = ? OR agent_id IS NULL) AND enabled 过滤
//  2. 内存打分 + topK 排序
//  3. 中文 bigram 切词 + 关键词包含加权
//  4. 排序: score * (baseConfidence + log(hit_count+1)) 降序
//  5. 返回 top K
//
// 后续可扩展: pgvector 全文检索 / Embedding 向量召回
func (r *FAQRepository) MatchByKeyword(ctx context.Context, msg string, topK int) ([]model.FAQEntry, error) {
	return r.MatchByKeywordForAgent(ctx, msg, 0, topK)
}

// MatchByKeywordForAgent 按智能体隔离的关键词匹配 (隔离架构)
//
// agentID = 0: 走旧路径 (全局共享, 向后兼容)
// agentID > 0: 匹配 (agent_id = agentID OR agent_id IS NULL) AND enabled
func (r *FAQRepository) MatchByKeywordForAgent(ctx context.Context, msg string, agentID uint, topK int) ([]model.FAQEntry, error) {
	if msg == "" {
		return nil, nil
	}
	all, err := r.listEnabledForAgent(ctx, agentID, 5000)
	if err != nil {
		return nil, err
	}
	return r.scoreAndRank(all, msg, topK)
}

// ListCandidates 返回指定 agent 的 FAQ 候选集（已启用），用于 Service 层命中内存缓存后复用，
// 避免每次 FAQ 匹配都全量拉取 candidate 再打分。
//
// 语义与 listEnabledForAgent 完全一致：
//   - agentID == 0: 全部已启用 FAQ（共享池）
//   - agentID > 0: agent_id = agentID OR agent_id IS NULL（该 agent 私有 + 共享）
func (r *FAQRepository) ListCandidates(ctx context.Context, agentID uint, limit int) ([]model.FAQEntry, error) {
	return r.listEnabledForAgent(ctx, agentID, limit)
}

// ScoreCandidates 对已加载的 FAQ 候选集做内存打分排序（不触发 DB 查询）。
// 供 Service 层命中内存缓存后复用候选集、仅按不同 query 重新打分。
func (r *FAQRepository) ScoreCandidates(ctx context.Context, entries []model.FAQEntry, msg string, topK int) ([]model.FAQEntry, error) {
	return r.scoreAndRank(entries, msg, topK)
}

func (r *FAQRepository) listEnabledForAgent(ctx context.Context, agentID uint, limit int) ([]model.FAQEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var entries []model.FAQEntry
	q := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("hit_count DESC, confidence DESC, id ASC").
		Limit(limit)
	if agentID > 0 {
		q = q.Where("agent_id = ? OR agent_id IS NULL", agentID)
	}
	if err := q.Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// ListShared 列出全部共享 FAQ (agent_id IS NULL)
func (r *FAQRepository) ListShared(ctx context.Context, limit int) ([]model.FAQEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("agent_id IS NULL AND enabled = ?", true).
		Order("id DESC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// MatchByAgent 按智能体 ID 匹配 (Task 15: 强 1对1 改造)
//
// 行为:
//   - agentID == 0  -> 无绑定空, 返回 (nil, nil)
//   - 仅匹配 enabled = true AND agent_id = ? 的 FAQ
//   - 移除"空数组=全局"分支: 任何 agent 都必须显式绑定才能匹配
//
// SQL: WHERE enabled = true AND agent_id = ?  (走 idx_faq_agent_id 索引)
func (r *FAQRepository) MatchByAgent(ctx context.Context, agentID uint, msg string, topK int) ([]model.FAQEntry, error) {
	if agentID == 0 || msg == "" {
		return nil, nil
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND agent_id = ?", true, agentID).
		Order("hit_count DESC, confidence DESC, id ASC").
		Limit(5000).
		Find(&entries).Error
	if err != nil {
		return nil, err
	}
	return r.scoreAndRank(entries, msg, topK)
}

// ListByAgent 列出某智能体的 FAQ 集合 (不参与打分, 仅做缓存预热 / 后台同步)
func (r *FAQRepository) ListByAgent(ctx context.Context, agentID uint, limit int) ([]model.FAQEntry, error) {
	if agentID == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND agent_id = ?", true, agentID).
		Order("id ASC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// ScoreFAQCandidates 对已加载的 FAQ 候选集打分排序。
//
// 导出它是为了让 service 层的测试能复用**生产同一份**打分口径，而不是另写一份
// 近似实现 —— 另写一份就等于让用例锁住一份与生产不同的算法。
//
// 刻意**不收 ctx**：打分是纯内存计算，一处都不查库。以前这个函数收下 ctx 又丢掉、
// 转手传 context.Background() 给下游，等于给后来加进来的一条查询埋好"静默绕过租户隔离"
// 的坑（on 模式下无租户 ctx 的查询会被 fail-closed）。既然不查库，就不留这个假入口。
func ScoreFAQCandidates(r *FAQRepository, entries []model.FAQEntry, msg string, topK int) ([]model.FAQEntry, error) {
	return r.scoreAndRank(entries, msg, topK)
}

func (r *FAQRepository) scoreAndRank(all []model.FAQEntry, msg string, topK int) ([]model.FAQEntry, error) {
	msgTokens := tokenize(msg)
	if len(msgTokens) == 0 {
		return nil, nil
	}
	type scored struct {
		entry model.FAQEntry
		score float64
		level float64
	}
	var results []scored
	for i := range all {
		e := all[i]
		if e.Enabled == nil || !*e.Enabled {
			continue
		}
		faqTokens := tokenize(e.Question)
		// 匹配分 = 关键词命中强度（主导）+ 问法相似度（**只做同分决胜**）。
		//
		// 为什么改成这个结构：早先把关键词命中**每条独立加分**、jaccard 只占 0~1，
		// 于是"关键词多但问法不匹配"的 FAQ 靠堆砌压过问法更贴近的条目
		// （线上实测「多少钱」命中「部署需要什么配置」、「支持哪些语言」命中
		// 「支持哪些渠道」）—— 答非所问比匹配不上更糟。
		//
		// 现在关键词命中强度是**整数计数**（命中几个、命中得多"准"），
		// 问法相似度只做同分时的决胜项：命中数是整数、天然有序，
		// 不存在"两个都命中两个关键词、谁的分高"这种靠权重硬凑的模糊地带。
		// 匹配强度**分层**，不混在一张分数上。
		//
		// 一层（精确命中）：关键词原样是用户句的连续子串。这是强证据。
		// 二层（词元命中）：与关键词共用词元（语序变了、字面不连续）。只作兜底。
		//
		// 分层而不是加权混合，是因为两套机制混在一张分数上永远在打架：
		// 词元命中数一多（中文二元组太容易巧合）就能压过精确命中
		// （「不会写代码能用吗」撞上"商用/要钱吗"、"能做GEO优化吗"撞上"智能体/功能"），
		// 而一旦让词元命中也参与同层竞争，精确命中又被稀释。
		//
		// 排序键：(层, 层内强度, 问法相似度) —— 层是**主键**，层内再比强度，
		// 强度相同才看问法相似度。这样"有精确命中的"永远排在"只有词元命中的"前面。
		exactHits := faqExactKeywordHits(msg, e.Keywords)
		var level, strength float64
		if exactHits > 0 {
			level = faqMatchLevelExact
			strength = float64(exactHits)
		} else {
			tokenHits := faqTokenHits(msg, e.Keywords)
			if tokenHits > 0 {
				level = faqMatchLevelToken
				strength = float64(tokenHits)
			} else if len(e.Keywords) == 0 {
				// 这条 FAQ **没有配关键词** ⇒ 只能退回问法本身的词元重合。
				//
				// 语序完全颠倒时 jaccard 无能为力（「优惠价可以」vs「可以优惠价吗」
				// 只有 0.04），但两句话仍共用"优惠""价"这些词元 ——
				// 词元重合是这种情况下唯一能识别出"讲的是同一件事"的信号。
				//
				// 只在**没有关键词**的条目上走这条路：有关键词的条目由上面的
				// 精确/词元命中负责，避免中文二元组的巧合造成答非所问。
				if overlap := faqTokenOverlapCount(msgTokens, faqTokens); overlap > 0 {
					level = faqMatchLevelToken
					strength = float64(overlap)
				}
			}
		}
		similarity := jaccardSimilarity(msgTokens, faqTokens)
		// 分数只反映"这条有多像用户问的那条"（0~1 量级，可与 0.6 门槛比）；
		// 层号只作**排序键**（主键），不进分数 —— 进了会把分数放大到门槛失去意义。
		score := faqStrengthWeight*strength + faqSimilarityTieBreak*similarity
		score = score * (e.Confidence + 0.1*logPlus(e.HitCount))
		if level == 0 {
			// 精确命中与词元命中都没有 ⇒ 这条与用户问法无关。
			continue
		}
		results = append(results, scored{entry: e, score: score, level: level})
	}
	// 层是主键：先按层排，层内按强度排，强度相同才看问法相似度。
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].level != results[j].level {
			return results[i].level > results[j].level
		}
		return results[i].score > results[j].score
	})
	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	out := make([]model.FAQEntry, 0, topK)
	for i := 0; i < topK; i++ {
		results[i].entry.Confidence = results[i].score
		out = append(out, results[i].entry)
	}
	return out, nil
}

// ListWithFilter 分页+过滤 (前端 FAQ 管理页面使用)
func (r *FAQRepository) ListWithFilter(ctx context.Context, filter FAQListParams) ([]model.FAQEntry, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.FAQEntry{})
	if filter.Keyword != "" {
		kw := "%" + filter.Keyword + "%"
		q = q.Where("question LIKE ? OR answer LIKE ?", kw, kw)
	}
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.Intent != "" {
		q = q.Where("intent = ?", filter.Intent)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []model.FAQEntry
	offset := (filter.Page - 1) * filter.PageSize
	if offset < 0 {
		offset = 0
	}
	err := q.Order("hit_count DESC, confidence DESC, id ASC").
		Offset(offset).Limit(filter.PageSize).
		Find(&entries).Error
	return entries, total, err
}

// Update 更新 FAQ
func (r *FAQRepository) Update(ctx context.Context, id uint, entry *model.FAQEntry) error {
	return r.db.WithContext(ctx).Model(&model.FAQEntry{}).
		Where("id = ?", id).
		Select("*").Updates(entry).Error
}

// SetEnabled 仅切换启用状态（列表页开关专用，避免全量 Update 的 required 校验）
func (r *FAQRepository) SetEnabled(ctx context.Context, id uint, enabled bool) error {
	return r.db.WithContext(ctx).Model(&model.FAQEntry{}).
		Where("id = ?", id).
		Update("enabled", enabled).Error
}

// Delete 删除 FAQ
func (r *FAQRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.FAQEntry{}).Error
}

type FAQListParams struct {
	Keyword  string
	Category string
	Intent   string
	Enabled  *bool
	Page     int
	PageSize int
}

// IncrementHitCount 命中次数 +1
func (r *FAQRepository) IncrementHitCount(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).
		Model(&model.FAQEntry{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"hit_count":   gorm.Expr("hit_count + 1"),
			"last_hit_at": gorm.Expr("NOW()"),
		}).
		Error
}

// DecayQuality 衰减指定 FAQ 的 QualityScore (: 质量衰减)
//
// 入参:
//   - id:    要衰减的 FAQ ID
//   - decay: 衰减量, 范围 [0, 1], 调用方负责边界 (repo 不做下限保护以保持纯函数语义)
//
// 行为:
//   - 用 GREATEST(quality_score - decay, 0) 保证不会衰减到负数
//   - 同一事务可多次调用叠加衰减
func (r *FAQRepository) DecayQuality(ctx context.Context, id uint, decay float64) error {
	if id == 0 || decay <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.FAQEntry{}).
		Where("id = ?", id).
		UpdateColumn("quality_score", gorm.Expr("GREATEST(quality_score - ?, 0)", decay)).
		Error
}

// ListDecayCandidates 查询符合衰减条件的 FAQ
//
// 条件:
//   - hit_count < 5 (低命中)
//   - last_hit_at 非空 且 早于 cutoff (超过 7 天未被命中)
//
// cutoff 建议为 now-7d, 调用方注入
func (r *FAQRepository) ListDecayCandidates(ctx context.Context, cutoff time.Time, limit int) ([]model.FAQEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("hit_count < ? AND last_hit_at IS NOT NULL AND last_hit_at < ?", 5, cutoff).
		Order("last_hit_at ASC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// IncrementNegativeHit 用户负反馈 +1 (: 负反馈快速降权)
func (r *FAQRepository) IncrementNegativeHit(ctx context.Context, id uint) error {
	if id == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.FAQEntry{}).
		Where("id = ?", id).
		UpdateColumn("negative_hit_count", gorm.Expr("negative_hit_count + 1")).
		Error
}

func tokenize(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	runes := []rune(s)
	tokens := make([]string, 0, len(runes))
	if !hasChinese(s) {
		for _, w := range strings.Fields(s) {
			if utf8.RuneCountInString(w) >= 2 {
				tokens = append(tokens, w)
			}
		}
		return tokens
	}
	// 英文/数字片段**整体成词**，不切碎片。
	//
	// 早先只把 CJK 二元组和单字入词，夹在中文里的英文被按字符切成了
	// 2 字碎片（"Telegram" → le/te/gr/am…、"SaaS" → aa/s…），
	// 于是英文关键词的"词元级命中"全是碎片乱撞：用户问「能接入Telegram吗」
	// 靠 "le/te" 这种碎片撞上了「支持哪些渠道接入」那条的"接入" ⇒ 答非所问。
	wordBuf := make([]rune, 0, len(runes))
	flushWord := func() {
		if len(wordBuf) >= 2 {
			// 去掉首尾空白再入词：分隔用的空格/标点不该进词元
			// （"能接入 Telegram 吗" 的 "Telegram" 前后各有一个空格）。
			word := strings.TrimSpace(strings.ToLower(string(wordBuf)))
			if len(word) >= 2 {
				tokens = append(tokens, word)
			}
		}
		wordBuf = wordBuf[:0]
	}
	for _, r := range runes {
		if isCJK(r) {
			flushWord()
			continue
		}
		wordBuf = append(wordBuf, r)
	}
	flushWord()
	for i := 0; i < len(runes)-1; i++ {
		if isCJK(runes[i]) && isCJK(runes[i+1]) {
			tokens = append(tokens, string(runes[i:i+2]))
		}
	}
	for _, r := range runes {
		if isCJK(r) {
			tokens = append(tokens, string(r))
		}
	}
	return tokens
}

// FAQ 关键词命中的加分。
//
// 关键词是人工标注的**强证据**（"这条问答讲的就是收费"），比 jaccard 的字面相似度
// 可靠得多。早先固定 +0.3，且只在"关键词原样连续出现在用户句子里"时才给 ——
// 中文里语序一变整句就不含原关键词（关键词「产品介绍」遇到「介绍下项目」），
// FAQ 明明有标准答案却匹配不上，问题只能落到 RAG 弱召回上 → 置信度不足 → 转人工。
const (
	// faqMatchLevelExact 精确命中层的层号（主键，比词元层高）。
	faqMatchLevelExact = 2.0
	// faqMatchLevelToken 词元命中层的层号（兜底）。
	faqMatchLevelToken = 1.0
	// faqStrengthWeight 层内强度（命中个数）的权重。
	faqStrengthWeight = 1.0
	// faqSimilarityTieBreak 问法相似度在总分里的权重 —— **只做同分决胜**。
	//
	// 取 0.3：小于一次精确命中（1.0），所以相似度高**不能**翻盘关键词命中数
	// 不同的两条；但命中数相同时它能把更贴近问法的那条排前面。
	faqSimilarityTieBreak = 0.3
)

// faqExactKeywordHits 统计用户问法**精确命中**（连续子串）的关键词个数。
//
// 为什么只认精确命中、不认"共用词元"：
// 中文二元组太容易巧合 ——「不会写代码能用吗」与「商用需要买授权码吗？」共享
// "能"这个字元，"要钱吗""收费吗"这些关键词也会被算成命中，于是
// 「不会写代码能用吗」被"商用/授权码"那条抢走；「能做GEO优化吗」被
// "智能体/功能/能力"那条抢走。答非所问比匹配不上更糟。
//
// 代价：语序变了、字面不连续的同义问法（关键词「产品介绍」遇到「介绍下项目」）
// 匹配不上 —— 那种情况交给问法相似度与知识库 RAG 层兜底，不在这里硬凑。
func faqExactKeywordHits(msg string, keywords []string) int {
	lowerMsg := strings.ToLower(msg)
	hits := 0
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(lowerMsg, strings.ToLower(kw)) {
			hits++
		}
	}
	return hits
}

// faqTokenHits 统计用户问法与关键词**共用词元**的关键词个数（兜底用）。
//
// 只在"精确命中数为 0"时参与打分，见 scoreAndRank 里的说明。
func faqTokenHits(msg string, keywords []string) int {
	msgTokens := tokenize(msg)
	if len(msgTokens) == 0 {
		return 0
	}
	hits := 0
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if keywordTokenOverlap(msg, kw) {
			hits++
		}
	}
	return hits
}

// faqTokenOverlapCount 算用户问法与 FAQ 文本的**词元重合个数**。
//
// 与 jaccard 的区别：jaccard 是"重合占比"（语序颠倒时趋近 0），这里只数"重合了几个"
// —— 「优惠价可以」与「可以优惠价吗」jaccard 只有 0.04，但共用"优惠""价"等词元，
// 重合个数能识别出"讲的是同一件事"。
//
// 只用于**没有配关键词**的 FAQ（见 scoreAndRank 里的说明）。
func faqTokenOverlapCount(msgTokens, faqTokens []string) int {
	if len(msgTokens) == 0 || len(faqTokens) == 0 {
		return 0
	}
	setB := make(map[string]struct{}, len(faqTokens))
	for _, t := range faqTokens {
		setB[t] = struct{}{}
	}
	seen := make(map[string]struct{}, len(msgTokens))
	overlap := 0
	for _, t := range msgTokens {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		if _, ok := setB[t]; ok {
			overlap++
		}
	}
	return overlap
}

// keywordTokenOverlap 判断用户句与关键词是否共用词元。
//
// 关键词「产品介绍」与用户句「介绍下项目」共用「介绍」—— 核心词在，语序不同。
func keywordTokenOverlap(msg, kw string) bool {
	msgTokens := tokenize(msg)
	if len(msgTokens) == 0 {
		return false
	}
	kwTokens := tokenize(kw)
	if len(kwTokens) == 0 {
		return false
	}
	for _, t := range kwTokens {
		for _, mt := range msgTokens {
			if t == mt {
				return true
			}
		}
	}
	return false
}

func hasChinese(s string) bool {
	for _, r := range s {
		if isCJK(r) {
			return true
		}
	}
	return false
}

func isCJK(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF
}

func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, s := range a {
		setA[s] = struct{}{}
	}
	setB := make(map[string]struct{}, len(b))
	for _, s := range b {
		setB[s] = struct{}{}
	}
	inter := 0
	for k := range setA {
		if _, ok := setB[k]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func logPlus(n int64) float64 {
	if n <= 0 {
		return 0
	}
	x := float64(n)
	if x < 1 {
		return 0
	}
	if x < 10 {
		return 0.5
	}
	if x < 100 {
		return 1.0
	}
	return 1.5
}

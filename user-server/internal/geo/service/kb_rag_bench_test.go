package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	// OPT-SEC-06（2026-10-24）：10w+ chunks RAG 压测 harness（规划 §4.3）。
	//
	// 判据（规划口径）：固定 query 集、p50/p95/p99、错误率。本机没有生产分布，
	// 本 harness 只做两件事，**不作为容量结论**：
	//  1. 合成语料自测：向真 PG（testutil）灌 N 篇合成 KB 文档，压 Search 的
	//     关键词检索路径（标题 0.6 + 内容 0.4 打分）——它是当前唯一无外部依赖的
	//     检索实现；embedding/rerank 依赖外部 provider，QPS 压测需外部资源。
	//  2. 固定 query 集跑 b.N 轮，Go bench 输出 ns/op（分布统计交给 benchstat）。
	//
	// 运行：POSTGRES_TEST_* 就绪后
	//   go test ./internal/geo/service -bench BenchKBRagSearch -benchtime 5x -run '^$'
	// 语料规模固定 10000（Go test flag 需 TestMain 注册；规模外推留给外部压测机）。
	//
	// 验收口径（写进规划回写）：压测报告需带硬件/版本/数据集指纹；瓶颈定位到
	// embedding/DB/检索某一层——当前 Search 是全表读进内存再过滤（GetList 上限
	// 1000），可预判瓶颈在 DB 全量读；换 pgvector/tsvector 属后续域工作。
)

// BenchKBRagSearch 压 GEO KB 关键词检索（harness，见文件头注）。
func BenchKBRagSearch(b *testing.B) {
	db := geoTestDB(&testing.T{}) // 复用测试库装配（用完即清）
	repo := repository.NewGeoKnowledgeDocumentRepositoryWithDB(db)
	ctx := context.Background()

	// 合成语料：每篇标题唯一、内容 500 rune，保证关键词命中有区分度
	const corpus = 10000 // 压测语料规模（10w chunks 需外部压测机）
	docs := make([]*model.GeoKnowledgeDocument, 0, corpus)
	for i := 0; i < corpus; i++ {
		docs = append(docs, &model.GeoKnowledgeDocument{
			Title:     "合成文档 " + strconv.Itoa(i) + " crm 对比",
			Content:   fmt.Sprintf("第 %d 篇合成内容：中小企业 CRM 选型要点，包括功能、价格、服务。%s", i, pad500),
			SourceURL: fmt.Sprintf("https://bench.example.com/doc/%d", i),
			DocType:   "reference",
		})
	}
	if err := db.CreateInBatches(docs, 500).Error; err != nil {
		b.Fatalf("灌合成语料失败: %v", err)
	}
	svc := NewKBService(repo, nil)

	// 固定 query 集（3 条，覆盖命中/不命中/高频词）
	queries := []string{"0001", "不存在词zzz", "crm 对比"}
	i := 0
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		if _, err := svc.Search(ctx, queries[i%len(queries)], 10); err != nil {
			b.Fatalf("search 失败: %v", err)
		}
		i++
	}
}

const pad500 = "知识库检索性能测试填充文本。知识库检索性能测试填充文本。知识库检索性能测试填充文本。" +
	"知识库检索性能测试填充文本。知识库检索性能测试填充文本。知识库检索性能测试填充文本。" +
	"知识库检索性能测试填充文本。知识库检索性能测试填充文本。知识库检索性能测试填充文本。" +
	"知识库检索性能测试填充文本。知识库检索性能测试填充文本。知识库检索性能测试填充文本。" +
	"知识库检索性能测试填充文本。知识库检索性能测试填充文本。知识库检索性能测试填充文本。" +
	"知识库检索性能测试填充文本。知识库检索性能测试填充文本。知识库检索性能测试填充文本。" +
	"知识库检索性能测试填充文本。知识库检索性能测试填充文本。知识库检索性能测试填充文本。"

package trace_learning

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeInsightLLM struct {
	content string
	err     error
	calls   int
}

func (f *fakeInsightLLM) Dispatch(ctx context.Context, req llm.DispatchRequest) (*llm.DispatchResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &llm.DispatchResult{Content: f.content}, nil
}

// TestShouldDistillInsight_ThresholdBoundary Bad(<60) 边界
func TestShouldDistillInsight_ThresholdBoundary(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, shouldDistillInsight(cfg, &EvalResult{Score: 60}), "score==60 不算 Bad")
	assert.True(t, shouldDistillInsight(cfg, &EvalResult{Score: 59}), "score<60 应沉淀")
	assert.True(t, shouldDistillInsight(cfg, &EvalResult{Score: 0, Bad: true}))
	assert.False(t, shouldDistillInsight(cfg, nil))

	cfgZero := Config{}
	assert.False(t, shouldDistillInsight(cfgZero, &EvalResult{Score: 60}))
}

func TestNormalizeInsight(t *testing.T) {
	assert.Empty(t, normalizeInsight("   "))
	assert.Equal(t, "报价前未确认预算", normalizeInsight("  \"报价前未确认预算\"\n"))
	long := strings.Repeat("长", 300)
	got := normalizeInsight(long)
	require.Len(t, []rune(got), DefaultInsightMaxLen)

	assert.Equal(t, "a b", normalizeInsight("a\nb"))
}

func TestExtractErrorPattern(t *testing.T) {
	fake := &fakeInsightLLM{content: "未确认预算即报价导致流失"}
	agg := &AggregatedTrace{Query: "多少钱", Reply: "三万"}
	got, err := ExtractErrorPattern(context.Background(), fake, llm.ScenarioHighQuality, agg)
	require.NoError(t, err)
	assert.Equal(t, "未确认预算即报价导致流失", got)
	assert.Equal(t, 1, fake.calls)

	_, err = ExtractErrorPattern(context.Background(), fake, llm.ScenarioHighQuality, &AggregatedTrace{})
	assert.True(t, errors.Is(err, ErrNoEvaluableContent))
	assert.Equal(t, 1, fake.calls)

	fakeErr := &fakeInsightLLM{err: errors.New("timeout")}
	_, err = ExtractErrorPattern(context.Background(), fakeErr, llm.ScenarioHighQuality, agg)
	assert.Error(t, err)
}

// TestSaveInsight_Idempotent 同 source_trace_id 重评覆盖不重复插入
func TestSaveInsight_Idempotent(t *testing.T) {
	db := testutil.NewTestDB(t, &model.LearningInsight{})
	ctx := context.Background()

	repo := repository.NewTraceLearningRepository(db)
	require.NoError(t, repo.SaveInsight(ctx, "medical_beauty", "v1 经验", "trace-1"))
	require.NoError(t, repo.SaveInsight(ctx, "medical_beauty", "v2 经验(覆盖)", "trace-1"))

	var rows []model.LearningInsight
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "v2 经验(覆盖)", rows[0].InsightText)
	assert.Equal(t, "medical_beauty", rows[0].Industry)
}

// TestDistillInsightForTrace_SkipWithoutIndustry 未配置 Industry 时跳过沉淀（不写库）
func TestDistillInsightForTrace_SkipWithoutIndustry(t *testing.T) {
	db := testutil.NewTestDB(t, &model.LearningInsight{})
	svc := &Service{repo: repository.NewTraceLearningRepository(db), cfg: DefaultConfig()}

	svc.distillInsightForTrace(context.Background(),
		&AggregatedTrace{TraceID: "t-x", Query: "q", Reply: "r"},
		&EvalResult{Score: 10, Bad: true})

	var count int64
	require.NoError(t, db.Model(&model.LearningInsight{}).Count(&count).Error)
	assert.Zero(t, count)
}

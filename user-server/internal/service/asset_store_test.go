package service

import (
	"testing"

	"hivemtk-user/internal/model"
)

// TestAssetListingTransitionAllowed 全状态迁移表驱动（T5 §9.1）：
// 合法链路放行；draft→published 等越级必须拒绝；驳回→重提→发布全链路成立。
func TestAssetListingTransitionAllowed(t *testing.T) {
	legal := []struct{ from, to string }{
		{model.AssetListingDraft, model.AssetListingSubmitted},
		{model.AssetListingSubmitted, model.AssetListingAutoCheck},
		{model.AssetListingSubmitted, model.AssetListingRejected},
		{model.AssetListingAutoCheck, model.AssetListingManualReview},
		{model.AssetListingAutoCheck, model.AssetListingRejected},
		{model.AssetListingManualReview, model.AssetListingPublished},
		{model.AssetListingManualReview, model.AssetListingRejected},
		{model.AssetListingPublished, model.AssetListingSuspended},
		{model.AssetListingRejected, model.AssetListingSubmitted},
		{model.AssetListingSuspended, model.AssetListingManualReview},
		{model.AssetListingSuspended, model.AssetListingDraft},
	}
	for _, c := range legal {
		if !model.AssetListingTransitionAllowed(c.from, c.to) {
			t.Errorf("合法跃迁被拒: %s → %s", c.from, c.to)
		}
	}
	illegal := []struct{ from, to string }{
		{model.AssetListingDraft, model.AssetListingPublished},
		{model.AssetListingDraft, model.AssetListingRejected},
		{model.AssetListingDraft, model.AssetListingManualReview},
		{model.AssetListingSubmitted, model.AssetListingPublished},
		{model.AssetListingSubmitted, model.AssetListingDraft},
		{model.AssetListingPublished, model.AssetListingDraft},
		{model.AssetListingPublished, model.AssetListingSubmitted},
		{model.AssetListingRejected, model.AssetListingPublished},
		{model.AssetListingRejected, model.AssetListingAutoCheck},
		{model.AssetListingSuspended, model.AssetListingPublished},
		{"", model.AssetListingSubmitted},
		{model.AssetListingDraft, ""},
	}
	for _, c := range illegal {
		if model.AssetListingTransitionAllowed(c.from, c.to) {
			t.Errorf("非法跃迁被放行: %s → %s", c.from, c.to)
		}
	}
	// 驳回→重提→发布全链路
	chain := []string{model.AssetListingDraft, model.AssetListingSubmitted, model.AssetListingRejected,
		model.AssetListingSubmitted, model.AssetListingAutoCheck, model.AssetListingManualReview, model.AssetListingPublished}
	for i := 0; i < len(chain)-1; i++ {
		if !model.AssetListingTransitionAllowed(chain[i], chain[i+1]) {
			t.Errorf("驳回重提链路断在 %s → %s", chain[i], chain[i+1])
		}
	}
}

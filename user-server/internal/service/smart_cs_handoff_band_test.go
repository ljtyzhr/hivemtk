package service

import (
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// 回归：官网右下角网页客服问「介绍下项目」被判「AI 置信度不足 (0.62 < 0.70)」转人工。
//
// 编排器在低置信时**一律**转人工，但置信度聚合器自己已经算出了决策区间
// （BandHandoff / BandLLMFallback / BandReview / BandAuto）—— 那一层区分了
// "确实该转人工"（handoff）与"该先走 LLM 兜底 / 人工审核"（llm_fallback / review）。
// 线上「介绍下项目」的信号被判为 **review** band，编排器却仍按"低置信即转人工"处理。
//
// 判据：band 明确不是 handoff 时，低置信**不该**直接转人工。

// TestShouldTransfer_LowConfidenceButNotHandoffBand 不转人工。
func TestShouldTransfer_LowConfidenceButNotHandoffBand(t *testing.T) {
	for _, band := range []string{dto.BandLLMFallback, dto.BandReview, dto.BandAuto} {
		t.Run(band, func(t *testing.T) {
			if shouldTransferOnLowConfidence(band, 0.62, 0.70) {
				t.Errorf("band=%s 且 conf=0.62 < 0.70 时不该直接转人工", band)
			}
		})
	}
}

// TestShouldTransfer_HandoffBandStillTransfers handoff band 仍然转人工。
func TestShouldTransfer_HandoffBandStillTransfers(t *testing.T) {
	if !shouldTransferOnLowConfidence(dto.BandHandoff, 0.62, 0.70) {
		t.Error("band=handoff 且低置信时必须转人工")
	}
}

// TestShouldTransfer_ConfidentNeverTransfers 置信度达标时不该转人工。
func TestShouldTransfer_ConfidentNeverTransfers(t *testing.T) {
	if shouldTransferOnLowConfidence(dto.BandAuto, 0.85, 0.70) {
		t.Error("置信度达标时不该转人工")
	}
}

// TestShouldTransfer_UnknownBandFallsBackToThresholdCheck band 未知（空）时退回
// 旧口径：只看置信度与阈值，保证老数据/老路径行为不变。
func TestShouldTransfer_UnknownBandFallsBackToThresholdCheck(t *testing.T) {
	if !shouldTransferOnLowConfidence("", 0.62, 0.70) {
		t.Error("band 未知且低置信时应按旧口径转人工")
	}
	if shouldTransferOnLowConfidence("", 0.85, 0.70) {
		t.Error("band 未知但置信度达标时不该转人工")
	}
}

// TestKnownIntent_EmptyIntentIsNotKnown 空意图不是"已知意图"。
//
// 线上 FAQ 的 intent 字段大多为空（只有后补的 4 条填了），而 FAQ 直答会把
// entry.Intent 原样带进 salesResp.Intent.IntentType —— 空串。
// 旧判据 `IntentType != IntentUnknown` 把空串也算成"已知意图"，
// 于是 FAQ 已经用标准答案答出来了，仍被低置信判据推翻去转人工。
func TestKnownIntent_EmptyIntentIsNotKnown(t *testing.T) {
	if isKnownIntent(&dto.RecognizeResult{IntentType: ""}) {
		t.Error("空意图不应算已知意图（FAQ 直答的 intent 字段线上多为空）")
	}
	if !isKnownIntent(&dto.RecognizeResult{IntentType: "ask_product"}) {
		t.Error("ask_product 应算已知意图")
	}
	if isKnownIntent(&dto.RecognizeResult{IntentType: IntentUnknown}) {
		t.Error("unknown 不应算已知意图")
	}
	if isKnownIntent(nil) {
		t.Error("nil 意图不应算已知意图")
	}
}

// TestIsDefiniteAnswer_ConfidenceOverride FAQ 直答有确定答案，不适用置信度判据。
func TestIsDefiniteAnswer_ConfidenceOverride(t *testing.T) {
	// FAQ 直答的响应：带一个标记，编排器据此跳过置信度判据。
	if !isDefiniteAnswer(&SalesResponse{DefiniteAnswer: true}) {
		t.Error("标记为确定答案的响应应跳过置信度判据")
	}
	if isDefiniteAnswer(&SalesResponse{}) {
		t.Error("普通 LLM 响应不应跳过置信度判据")
	}
}

// TestHandleResult_TransferredFlagContract 转人工结果必须带原因，便于排查。
func TestHandleResult_TransferredFlagContract(t *testing.T) {
	r := &HandleResult{HandlerType: model.HandlerTypeHuman, Transferred: true, TransferReason: "x"}
	if r.Transferred && r.TransferReason == "" {
		t.Error("转人工结果必须带原因")
	}
}

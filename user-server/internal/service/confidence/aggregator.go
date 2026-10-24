package confidence

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

type ConfidenceAggregator struct {
	collector      *SignalCollector
	calibrator     *Calibrator
	platt          *PlattScaling
	conformal      *ConformalPredictor
	conformalCalib *ConformalCalibrator
	aggregator     *WeightedAggregator
	vetoChain      *VetoChain
	calc           *DynamicThresholdCalculator
	signalRepo     *repository.ConfidenceSignalRepository
}

// NewConfidenceAggregator 创建总聚合器
func NewConfidenceAggregator(
	collector *SignalCollector,
	calibrator *Calibrator,
	aggregator *WeightedAggregator,
	vetoChain *VetoChain,
	calc *DynamicThresholdCalculator,
	signalRepo *repository.ConfidenceSignalRepository,
) *ConfidenceAggregator {
	return &ConfidenceAggregator{
		collector:  collector,
		calibrator: calibrator,
		aggregator: aggregator,
		vetoChain:  vetoChain,
		calc:       calc,
		signalRepo: signalRepo,
	}
}

// SetPlatt 注入 Platt 校准器（二分类置信度）
//
// 调用时机：训练完 Platt 校准后；亦可 nil 关闭
func (a *ConfidenceAggregator) SetPlatt(p *PlattScaling) {
	a.platt = p
}

// SetConformal 注入 Conformal 预测器（覆盖率保证）
//
// 调用时机：CalibrateOnline 收集到足够非一致性分数后；亦可 nil 关闭
func (a *ConfidenceAggregator) SetConformal(c *ConformalPredictor) {
	a.conformal = c
}

// GetPlatt 返回当前 Platt（用于运行时查询 / 持久化）
func (a *ConfidenceAggregator) GetPlatt() *PlattScaling {
	return a.platt
}

// GetConformal 返回当前 Conformal
func (a *ConfidenceAggregator) GetConformal() *ConformalPredictor {
	return a.conformal
}

// GetPolicy 返回指定意图的阈值策略（P0-2 G4：sales_engine_transfer 的前置门槛改读
// per-intent policy 单一来源，消除「前置 0.7 / policy 表 / Band 边界」三套数字并存）。
// engine 未装配或查不到意图时回 default 档——与 GetPolicy 内部回退一致。
func (a *ConfidenceAggregator) GetPolicy(ctx context.Context, intentType string) *model.ThresholdPolicy {
	if a == nil || a.calc == nil || a.calc.policyEngine == nil {
		return builtinPolicies()["default"]
	}
	return a.calc.policyEngine.GetPolicy(ctx, intentType)
}

// RefreshThresholdPolicies 使指定租户的阈值策略缓存失效（运营后台写入后调用）。
func (a *ConfidenceAggregator) RefreshThresholdPolicies(tenantID uint) {
	if a == nil || a.calc == nil || a.calc.policyEngine == nil {
		return
	}
	a.calc.policyEngine.Invalidate(tenantID)
}

// Aggregate 主入口：采集信号 → 校准 → 聚合 → 否决 → 阈值 → 决策
//
// 输入：dto.SignalCollectionInput（包含 session/customer/message/intent/raw 信号/last turns）
// 输出：dto.ConfidenceDecision（包含 signal_id/aggregated_conf/dynamic_threshold/decision_band/veto_triggered/signals）
func (a *ConfidenceAggregator) Aggregate(ctx context.Context, in *dto.SignalCollectionInput) (*dto.ConfidenceDecision, error) {
	if a == nil {
		return nil, ErrAggregatorNotInitialized
	}

	signals, err := a.collector.Collect(ctx, in)
	if err != nil {
		return nil, err
	}

	calibratedIntentConf := signals.IntentConf
	if len(in.RawLogits) > 0 && a.calibrator != nil {
		calibrated := a.calibrator.Calibrate(in.RawLogits)
		if calibrated > 0 {
			calibratedIntentConf = calibrated
			signals.IntentConf = calibrated
		}
	}

	if a.platt != nil {
		plattCalibrated := a.platt.Predict(calibratedIntentConf)
		if plattCalibrated > 0 && plattCalibrated <= 1 {

			mixed := 0.5*calibratedIntentConf + 0.5*plattCalibrated
			if mixed > 0 {
				calibratedIntentConf = mixed
				signals.IntentConf = mixed
			}
		}
	}

	vetoCtx := &VetoContext{
		IntentType:        in.IntentType,
		CustomerMessage:   in.Text,
		LastNTurns:        in.LastTurns,
		ExpectedEntities:  in.ExpectedEntities,
		ExtractedEntities: in.ExtractedEntities,
	}
	vetoTriggered, vetoReason := a.vetoChain.Check(signals, vetoCtx)

	aggregatedConf := a.aggregator.Aggregate(signals)
	if vetoTriggered {
		aggregatedConf = 0
	}

	threshold := a.calc.Calculate(ctx, &ThresholdInput{
		IntentType:        in.IntentType,
		CustomerLevel:     inferCustomerLevel(in),
		AgentAvailability: inferAgentAvailability(in),
		Now:               time.Now(),
	})

	policy := a.calc.policyEngine.GetPolicy(ctx, in.IntentType)
	band := a.calc.DetermineBand(aggregatedConf, threshold, policy)
	if vetoTriggered {
		band = dto.BandHandoff
	}

	conformalUncertain := false
	if a.conformal != nil {

		nonConformity := 1.0 - aggregatedConf
		conformalThreshold := a.conformal.Quantile()
		if SelectivePredict(conformalThreshold, nonConformity) {
			conformalUncertain = true

			if !vetoTriggered && (band == dto.BandAuto || band == dto.BandReview) {
				band = dto.BandHandoff
				logger.Ctx(ctx).Info().
					Float64("non_conformity", nonConformity).
					Float64("conformal_threshold", conformalThreshold).
					Float64("conformal_coverage", a.conformal.CoverageGuarantee()).
					Msg("[Confidence] Conformal: high uncertainty, escalate to handoff")
			}
		}
	}

	decision := &dto.ConfidenceDecision{
		SignalID:         uuid.New().String(),
		AggregatedConf:   aggregatedConf,
		DynamicThreshold: threshold,
		DecisionBand:     band,
		VetoTriggered:    vetoReason,
		Signals:          *signals,
		CalculatedAt:     time.Now(),
	}

	if a.conformalCalib != nil {
		a.conformalCalib.AddScore(1.0 - aggregatedConf)
	}

	_ = conformalUncertain

	a.saveSignalAsync(ctx, in, decision, signals, calibratedIntentConf)
	return decision, nil
}

func (a *ConfidenceAggregator) saveSignalAsync(
	ctx context.Context,
	in *dto.SignalCollectionInput,
	dec *dto.ConfidenceDecision,
	sig *dto.FiveSignals,
	calibratedIntentConf float64,
) {
	if a.signalRepo == nil {
		return
	}
	record := &model.ConfidenceSignal{
		SignalID:             dec.SignalID,
		SessionID:            in.SessionID,
		CustomerID:           in.CustomerID,
		MessageID:            in.MessageID,
		IntentType:           in.IntentType,
		IntentConf:           in.RawIntentConf,
		IntentConfCalibrated: calibratedIntentConf,
		EntityComp:           sig.EntityComp,
		CtxRelev:             sig.CtxRelev,
		RAGQual:              sig.RAGQual,
		LLMEntropy:           sig.LLMEntropy,
		AggregatedConf:       dec.AggregatedConf,
		VetoTriggered:        dec.VetoTriggered,
		DynamicThreshold:     dec.DynamicThreshold,
		DecisionBand:         dec.DecisionBand,
		Temperature:          1.0,
	}
	if a.calibrator != nil {
		record.Temperature = a.calibrator.CurrentTemperature()
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("confidence_aggregator: async signalRepo.Create recovered from panic: %v\n%s", r, debug.Stack())
			}
		}()
		if err := a.signalRepo.Create(tenant.BackgroundWithTenant(ctx), record); err != nil {
			logger.Errorf("confidence_aggregator: async signalRepo.Create failed: %v", err)
		}
	}()
}

func inferCustomerLevel(in *dto.SignalCollectionInput) string {
	if in.CustomerLevel != "" {
		return in.CustomerLevel
	}
	return "normal"
}

func inferAgentAvailability(in *dto.SignalCollectionInput) float64 {
	if in.AgentAvailability > 0 {
		return in.AgentAvailability
	}
	return 0.5
}

// ErrAggregatorNotInitialized 聚合器未初始化
var ErrAggregatorNotInitialized = &aggError{"confidence aggregator not initialized"}

type aggError struct{ msg string }

func (e *aggError) Error() string { return e.msg }

// StartConformalBackground D19: 启动 Conformal 在线重校准后台任务（每 recalibrateSec 周期 Recalibrate）。
// 未注入校准器时为 no-op。
func (a *ConfidenceAggregator) StartConformalBackground(ctx context.Context) {
	if a == nil || a.conformalCalib == nil {
		return
	}
	go a.conformalCalib.BackgroundRunner(ctx)
}

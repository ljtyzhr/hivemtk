package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
)

type dashboardSnapshotEntry struct {
	snap *service.DashboardSnapshot
	at   time.Time
}

// DashboardSSEController 实时驾驶舱 SSE 控制器
type DashboardSSEController struct {
	statsSvc service.DashboardStatsService

	// 快照缓存按租户分域：单一 lastSnapshot 会把 A 租户的统计喂给 2 秒内
	// 请求快照的 B 租户（FR-018/FR-010 越权读）。
	cacheMu       sync.RWMutex
	snapshotCache map[uint]dashboardSnapshotEntry
	cacheTTL      time.Duration

	subMu           sync.Mutex
	subscriberByTen map[uint]int64
}

// NewDashboardSSEController 创建实时驾驶舱 SSE 控制器
//
// statsSvc 由 router 注入（router 负责创建 service.NewDashboardStatsService(gormDB)）。
// 传入 nil 时进入"离线模式"：SSE 仍可连接但跳过 DB 采集，返回零值快照。
func NewDashboardSSEController(statsSvc service.DashboardStatsService) *DashboardSSEController {
	return &DashboardSSEController{
		statsSvc:        statsSvc,
		cacheTTL:        2 * time.Second,
		snapshotCache:   make(map[uint]dashboardSnapshotEntry),
		subscriberByTen: make(map[uint]int64),
	}
}

// streamTenant 连接所属租户；off 模式无绑定归默认租户。
// on 模式无绑定返回 false——调用方必须拒绝建立流，绝不跨租户兜底。
func streamTenant(ctx context.Context) (uint, bool) {
	if id, ok := tenant.FromContext(ctx); ok {
		return id, true
	}
	if tenant.IsMultiTenant() {
		return 0, false
	}
	return tenant.DefaultTenantID, true
}

func (c *DashboardSSEController) addSubscriber(tid uint, delta int64) int64 {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	c.subscriberByTen[tid] += delta
	if c.subscriberByTen[tid] <= 0 {
		delete(c.subscriberByTen, tid)
		return 0
	}
	return c.subscriberByTen[tid]
}

func (c *DashboardSSEController) subscriberCount(tid uint) int64 {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	return c.subscriberByTen[tid]
}

// StreamEventStream godoc
// @Summary      实时驾驶舱 SSE 数据流
// @Description  保持长连接，每 2 秒推送 dashboard_update，每 15 秒推送 heartbeat
// @Tags         Dashboard
// @Produce      text/event-stream
// @Security     BearerAuth
// @Success      200  {string}  string  "event: dashboard_update / heartbeat"
// @Router       /api/dashboards/stream [get]
func (c *DashboardSSEController) StreamEventStream(ctx *gin.Context) {
	tenantID, ok := streamTenant(ctx.Request.Context())
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, "tenant context required")
		return
	}
	c.addSubscriber(tenantID, 1)
	defer c.addSubscriber(tenantID, -1)

	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)

	snapshot := c.collectSnapshot(ctx.Request.Context(), tenantID)
	if snapshot != nil {
		if writeErr := writeDashboardEvent(ctx, "dashboard_update", snapshot); writeErr != nil {
			return
		}
	}

	_ = writeDashboardRawEvent(ctx, "connected", map[string]any{
		"client_ip":   ctx.ClientIP(),
		"subscriber":  c.subscriberCount(tenantID),
		"message":     "dashboard event stream connected",
		"refresh_sec": int(c.cacheTTL.Seconds()),
	})

	dataTicker := time.NewTicker(c.cacheTTL)
	defer dataTicker.Stop()
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	clientClosed := ctx.Request.Context().Done()
	for {
		select {
		case <-clientClosed:
			return
		case <-dataTicker.C:
			snapshot := c.collectSnapshot(ctx.Request.Context(), tenantID)
			if snapshot == nil {
				continue
			}
			if writeErr := writeDashboardEvent(ctx, "dashboard_update", snapshot); writeErr != nil {
				return
			}
		case <-heartbeatTicker.C:
			if writeErr := writeDashboardRawEvent(ctx, "heartbeat", map[string]any{
				"ts":         time.Now().UTC().Format(time.RFC3339Nano),
				"subscriber": c.subscriberCount(tenantID),
			}); writeErr != nil {
				return
			}
		}
	}
}

// [AUTO] Snapshot
// @Tags      dashboard-s-s-e
// @Router    /api/dashboards/snapshot [get]
// Snapshot 返回当前快照（一次性 JSON）
func (c *DashboardSSEController) Snapshot(ctx *gin.Context) {
	tenantID, ok := streamTenant(ctx.Request.Context())
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, "tenant context required")
		return
	}
	snapshot := c.collectSnapshot(ctx.Request.Context(), tenantID)
	if snapshot == nil {
		response.Error(ctx, http.StatusInternalServerError, "采集快照失败")
		return
	}
	response.Success(ctx, snapshot, "ok")
}

// [AUTO] Metrics
// @Tags      dashboard-s-s-e
// @Router    /api/dashboards/metrics [get]
// Metrics 完整指标（含历史趋势）
func (c *DashboardSSEController) Metrics(ctx *gin.Context) {
	tenantID, ok := streamTenant(ctx.Request.Context())
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, "tenant context required")
		return
	}
	snapshot := c.collectSnapshot(ctx.Request.Context(), tenantID)
	if snapshot == nil {
		response.Error(ctx, http.StatusInternalServerError, "采集指标失败")
		return
	}
	response.Success(ctx, map[string]any{
		"snapshot":         snapshot,
		"subscriber_count": c.subscriberCount(tenantID),
		"cache_ttl_sec":    int(c.cacheTTL.Seconds()),
		"server_time":      time.Now().UTC().Format(time.RFC3339Nano),
	}, "ok")
}

func (c *DashboardSSEController) collectSnapshot(ctx context.Context, tenantID uint) *service.DashboardSnapshot {
	c.cacheMu.RLock()
	if entry, ok := c.snapshotCache[tenantID]; ok && entry.snap != nil && time.Since(entry.at) < c.cacheTTL {
		snap := entry.snap
		c.cacheMu.RUnlock()
		return snap
	}
	c.cacheMu.RUnlock()

	snap := &service.DashboardSnapshot{
		GeneratedAt: time.Now(),
	}

	if c.statsSvc != nil && c.statsSvc.Available(ctx) {
		c.statsSvc.CollectSessionStats(ctx, snap)
		snap.HumanizeDistribution = c.statsSvc.CollectHumanizeDistribution(ctx)
		snap.Funnel = c.statsSvc.CollectFunnel(ctx)
		snap.MessageVolume = c.statsSvc.CollectMessageVolume(ctx, 24)
	} else {
		snap.HumanizeDistribution = service.HumanizeDistribution{WindowHours: 1}
		snap.Funnel = &service.FunnelProgress{Stages: []service.FunnelStage{}}
		snap.MessageVolume = []service.MessageVolumePoint{}
	}

	snap.LLMMetrics = c.collectLLMMetrics(ctx)

	c.cacheMu.Lock()
	c.snapshotCache[tenantID] = dashboardSnapshotEntry{snap: snap, at: time.Now()}
	c.cacheMu.Unlock()

	return snap
}

func (c *DashboardSSEController) collectLLMMetrics(ctx context.Context) *service.LLMRealTimeMetrics {
	m := &service.LLMRealTimeMetrics{}
	defer func() {
		if r := recover(); r != nil {
			logger.Ctx(ctx).Warn().Interface("panic", r).Str("stack", string(debug.Stack())).Msg("dashboard_sse: LLM metrics collection recovered from panic")
		}
	}()
	return m
}

func writeDashboardEvent(c *gin.Context, eventType string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, string(jsonData))
	if err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func writeDashboardRawEvent(c *gin.Context, eventType string, data map[string]any) error {
	return writeDashboardEvent(c, eventType, data)
}

func roundTo(v float64, n int) float64 { //nolint:unused //// 仅被 *_test.go 引用，生产路径未用
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	mult := math.Pow(10, float64(n))
	return math.Round(v*mult) / mult
}

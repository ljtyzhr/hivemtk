// reach_send_pipeline_compliance.go 合规提醒与审计（R-8）：主动触达发送前的
// 合规提醒 WARN 日志，以及异步批量落库的合规审计记录（表 reach_compliance_log）。
package service

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

const complianceReminderTag = "[COMPLIANCE]"

func init() { _db.RegisterExtraModels(&model.ReachComplianceLog{}) }

// ReachComplianceLog 合规提醒审计日志（别名，模型已收敛到 model 层）
type ReachComplianceLog = model.ReachComplianceLog

const (
	complianceFlushBatchSize = 100
	complianceFlushInterval  = 5 * time.Second
	// complianceFlushTimeout 单租户一次刷盘的上限：与 flushInterval 同量级，
	// 超时只影响这一组（其余租户继续），不会把 ticker 拖成雪崩。
	complianceFlushTimeout = 5 * time.Second
)

// ComplianceAuditLogger R-8：异步批量落库的合规日志器
type ComplianceAuditLogger struct {
	mu      sync.Mutex
	buf     []*ReachComplianceLog
	repo    *repository.ReachComplianceLogRepository
	flushCh chan struct{}
	stop    chan struct{}
	stopped sync.Once
}

var (
	complianceLoggerOnce sync.Once
	complianceLogger     *ComplianceAuditLogger
)

// InitComplianceAuditLogger 初始化全局合规审计落库器（main/router 装配时调用一次）。
// 句柄由 repository 自取全局库；库未就绪时退化为仅 WARN 日志（record* 均 nil-safe）。
// 表结构经 EnsureTable 惰性创建，migrate.go 正式注册另行报告。
func InitComplianceAuditLogger() *ComplianceAuditLogger {
	complianceLoggerOnce.Do(func() {
		complianceLogger = &ComplianceAuditLogger{
			repo:    repository.NewReachComplianceLogRepositoryDefault(),
			flushCh: make(chan struct{}, 1),
			stop:    make(chan struct{}),
		}
		if repository.DBReady() {
			go complianceLogger.flushLoop()
		}
	})
	return complianceLogger
}

// GetComplianceAuditLogger 获取全局实例（未初始化返回 nil）
func GetComplianceAuditLogger() *ComplianceAuditLogger { return complianceLogger }

// record 记录一条「已发生主动触达」的合规提醒行（升级前的既有形态：只有渠道与收件人）。
//
// ctx 是**必须**的：reach_compliance_log 是租户表，而刷盘发生在几秒后的后台
// goroutine 里（那里只有进程级 ctx）。租户必须在入队这一刻抄进行里，否则刷盘时
// 无从知道该绑哪个租户 —— on 模式下隔离 callback 会把整批拒掉（fail-closed，
// 表现为"合规日志从来没落过库"），off 模式下整批落列默认值 1。
func (l *ComplianceAuditLogger) record(ctx context.Context, channel, recipientID string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.buf = append(l.buf, &ReachComplianceLog{
		TenantID:    tenant.WriteTenantID(ctx),
		Channel:     channel,
		RecipientID: recipientID,
	})
	if len(l.buf) > complianceFlushBatchSize*10 {
		l.buf = l.buf[len(l.buf)-complianceFlushBatchSize*10:]
	}
	full := len(l.buf) >= complianceFlushBatchSize
	l.mu.Unlock()
	if full {
		select {
		case l.flushCh <- struct{}{}:
		default:
		}
	}
}

// recordDecision E1：记录一条策略判定留痕（allow/deny/defer）。
//
// 与 record 分开而不是加参数：record 的调用点是「发送前提醒」，它对每次外发都记录；
// 判定留痕只在策略判定器装配时才产生，且要带原因码/地区/行业/规则域。
// 合并成一个带可选参数的方法会让调用方去猜「传空串算不算提醒行」。
func (l *ComplianceAuditLogger) recordDecision(ctx context.Context, entry *ReachComplianceLog) {
	if l == nil || entry == nil {
		return
	}
	// 同理：调用方给的 entry 不带租户（它由策略判定结果拼出来），补齐在入队口。
	if entry.TenantID == 0 {
		entry.TenantID = tenant.WriteTenantID(ctx)
	}
	l.mu.Lock()
	l.buf = append(l.buf, entry)
	if len(l.buf) > complianceFlushBatchSize*10 {
		l.buf = l.buf[len(l.buf)-complianceFlushBatchSize*10:]
	}
	full := len(l.buf) >= complianceFlushBatchSize
	l.mu.Unlock()
	if full {
		select {
		case l.flushCh <- struct{}{}:
		default:
		}
	}
}

// BufferedCount 当前缓冲条数（测试用）
func (l *ComplianceAuditLogger) BufferedCount() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buf)
}

// Flush 将缓冲批量写入 DB（供 flushLoop 与测试调用）；db 未配置时清空缓冲并返回
func (l *ComplianceAuditLogger) Flush() error {
	l.mu.Lock()
	if len(l.buf) == 0 {
		l.mu.Unlock()
		return nil
	}
	batch := l.buf
	l.buf = nil
	l.mu.Unlock()
	if l.repo == nil || len(batch) == 0 {
		return nil
	}
	// 按租户分组刷盘：reach_compliance_log 是租户表，一批缓冲里可能混着多个租户的行
	// （同一个进程同时服务多个租户）。整批用一个 ctx 写只有两种下场：on 模式下被
	// 隔离 callback 整批拒掉（一行都落不了库），off 模式下全部落默认租户。按行里
	// 已抄好的 TenantID 分组、每组用 OnlyTenant 显式绑定，才是这批行各自的真实归属。
	retry, err := l.flushByTenant(batch)
	if len(retry) > 0 {
		// 只把**失败的那些**重新入队：按租户分组后失败是局部现象（可能只有一组超时），
		// 整批回滚会把已经写成功的租户再写一遍。上限仍是 10 批 —— 宁可丢最老的，
		// 也不让缓冲无界增长。
		l.mu.Lock()
		l.buf = append(retry, l.buf...)
		if len(l.buf) > complianceFlushBatchSize*10 {
			l.buf = l.buf[len(l.buf)-complianceFlushBatchSize*10:]
		}
		l.mu.Unlock()
	}
	return err
}

// flushByTenant 逐租户写一批合规行，返回**失败需要回队**的那些行。
//
// TenantID 为 0 的行（on 模式下入队时 ctx 没绑租户）一律丢弃并出声：写哪一行都是
// 错的，落默认租户 1 更是把 A 租户的合规记录混进默认租户的审计表。这类行不回队
// —— 再刷一百次结果一样，占着缓冲只会把真正能写的行挤掉。
func (l *ComplianceAuditLogger) flushByTenant(batch []*ReachComplianceLog) ([]*ReachComplianceLog, error) {
	groups := make(map[uint][]*ReachComplianceLog, 4)
	order := make([]uint, 0, 4)
	unattributed := 0
	for _, row := range batch {
		if row.TenantID == 0 {
			unattributed++
			continue
		}
		if _, ok := groups[row.TenantID]; !ok {
			order = append(order, row.TenantID)
		}
		groups[row.TenantID] = append(groups[row.TenantID], row)
	}
	if unattributed > 0 {
		logger.Errorf("[R-8] %d 条合规日志没有租户归属（入队时 ctx 未绑定租户），已丢弃："+
			"多租户模式下写哪一行都是错的，落默认租户等于串库", unattributed)
	}

	var (
		retry   []*ReachComplianceLog
		lastErr error
	)
	for _, tid := range order {
		group := groups[tid]
		wctx, cancel := context.WithTimeout(tenant.OnlyTenant(context.Background(), tid), complianceFlushTimeout)
		err := l.repo.CreateInBatches(wctx, copyComplianceBatch(group), len(group))
		cancel()
		if err != nil {
			logger.Errorf("[R-8] 合规日志刷盘失败（租户 %d，%d 条）: %v", tid, len(group), err)
			retry = append(retry, group...)
			lastErr = err
		}
	}
	return retry, lastErr
}

// copyComplianceBatch 转换缓冲行为仓储入参
func copyComplianceBatch(batch []*ReachComplianceLog) []*model.ReachComplianceLog {
	out := make([]*model.ReachComplianceLog, 0, len(batch))
	for _, b := range batch {
		out = append(out, &model.ReachComplianceLog{
			TenantID:    b.TenantID,
			Channel:     b.Channel,
			RecipientID: b.RecipientID,
			CreatedAt:   b.CreatedAt,
			// E1 判定语义列：漏拷贝会让审计表只剩 channel/recipient，
			// 事后无法回答「这次为什么被拒」——那正是本功能要补的洞。
			Decision:    b.Decision,
			Reason:      b.Reason,
			Region:      b.Region,
			Industry:    b.Industry,
			PolicyScope: b.PolicyScope,
		})
	}
	return out
}

// recordComplianceDecision E1：把一次策略判定写进合规审计（异步批量落库）。
//
// 只记录**有语义的**判定：策略未启用（policy_disabled）不落库——默认路径不该给
// 审计表灌行；这与「策略启用后每次外发都留痕」的诉求刚好互补。
func recordComplianceDecision(ctx context.Context, req *ReachSendRequest, d ComplianceDecision) {
	if req == nil || d.Reason == ComplianceReasonDisabled || d.Reason == "" {
		return
	}
	recordComplianceDecisionEntry(ctx, &ReachComplianceLog{
		Channel:     req.Channel,
		RecipientID: req.RecipientID,
		Decision:    d.Decision(),
		Reason:      d.Reason,
		Region:      d.Region,
		Industry:    d.Industry,
		PolicyScope: d.PolicyScope,
	})
}

// recordComplianceDecisionEntry 落库入口（proactive 路径也走这里，口径只有一份）。
func recordComplianceDecisionEntry(ctx context.Context, entry *ReachComplianceLog) {
	if audit := GetComplianceAuditLogger(); audit != nil {
		audit.recordDecision(ctx, entry)
	}
}

func (l *ComplianceAuditLogger) flushLoop() {
	ticker := time.NewTicker(complianceFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			_ = l.Flush()
			return
		case <-l.flushCh:
			if err := l.Flush(); err != nil {
				logger.Errorf("[R-8] 合规日志刷盘失败: %v", err)
			}
		case <-ticker.C:
			if err := l.Flush(); err != nil {
				logger.Errorf("[R-8] 合规日志刷盘失败: %v", err)
			}
		}
	}
}

// Stop 停止刷盘协程（落盘剩余缓冲）
func (l *ComplianceAuditLogger) Stop() {
	if l == nil {
		return
	}
	l.stopped.Do(func() { close(l.stop) })
}

func LogComplianceReminder(ctx context.Context, channel, recipientID string) {
	logger.Warnf("%s 主动触达发送已触发：channel=%s, recipient=%s。"+
		"请严格遵守各渠道平台（微信/企业微信/抖音/快手/小红书/Telegram/WhatsApp(Meta)/短信/邮件 等）的"+
		"开发者规范、服务条款及当地法律法规；仅可向已授权、已明确同意接收的联系人发送，"+
		"严格控制发送频率，禁止发送垃圾营销、欺诈、骚扰或违法违规内容。"+
		"因违规发送导致的账号封禁、平台处罚、行政处罚或法律后果由使用者自行承担。",
		complianceReminderTag, channel, recipientID)

	complianceLogger.record(ctx, channel, recipientID)
}

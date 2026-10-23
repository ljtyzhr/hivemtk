package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/repository"
)

// WhatsappService WhatsApp 账号 / 会话 / 群发作业的业务入口。
//
// A2-c 之后它**不再直接持有任何协议客户端**：发送、登录、取二维码全部经
// `WhatsAppProvider`（见 whatsapp_provider.go）。这一层的职责只剩
// 「读账号 → 选实现 → 计费/节奏校验 → 委派」，所以将来删掉个人版实现时，
// 本文件一行都不用改。
type WhatsappService struct {
	repo     repository.WhatsappRepository
	clueRepo repository.ClueRepository

	// legacy 个人版（gowa）实现。**唯一**持有 gowa 连接的地方在其内部；
	// 这里只留一个接口引用，避免 service 层知道 gowa 存在。
	legacy *legacyWhatsAppProvider
	// cloud Cloud API 实现（无状态，按账号凭据临时构造客户端）。
	cloud WhatsAppProvider
}

// NewWhatsappService 构造服务：线索仓储用装配点注入的连接（db 为 nil 时该依赖不可用，
// 与调用方显式传入的语义一致，不回落全局句柄）。
func NewWhatsappService(db *gorm.DB) *WhatsappService {
	repo := repository.NewWhatsappRepository()
	return &WhatsappService{
		repo:     repo,
		clueRepo: repository.NewClueRepositoryWithDB(db),
		legacy:   newLegacyWhatsAppProvider(repo),
		cloud:    cloudWhatsAppProvider{},
	}
}

// providerFor 选账号对应的发送实现。
func (s *WhatsappService) providerFor(acc *model.WhatsappAccount) WhatsAppProvider {
	if acc != nil && acc.Type == "cloud" && acc.PhoneID != "" && acc.Token != "" {
		if s.cloud != nil {
			return s.cloud
		}
		return cloudWhatsAppProvider{}
	}
	return whatsAppProviderFor(acc, s.legacy)
}

// Accounts
func (s *WhatsappService) CreateAccount(ctx context.Context, name, remark string) (*model.WhatsappAccount, error) {
	acc := &model.WhatsappAccount{Name: name, Remark: remark, Status: model.WhatsappStatusPending}
	return acc, s.repo.CreateAccount(ctx, acc)
}

func (s *WhatsappService) ListAccounts(ctx context.Context) ([]*model.WhatsappAccount, error) {
	return s.repo.ListAccounts(ctx)
}

func (s *WhatsappService) UpdateAccount(ctx context.Context, acc *model.WhatsappAccount) error {
	return s.repo.UpdateAccount(ctx, acc)
}

func (s *WhatsappService) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAccount(ctx, id)
}

func (s *WhatsappService) GetAccount(ctx context.Context, id uuid.UUID) (*model.WhatsappAccount, error) {
	return s.repo.GetAccount(ctx, id)
}

// Login 个人版扫码登录。Cloud API 账号没有这一步，返回明确错误而不是空二维码。
func (s *WhatsappService) StartLogin(ctx context.Context, accountID uuid.UUID, timeout time.Duration) (string, error) {
	login, ok := s.loginProvider(accountID)
	if !ok {
		return "", fmt.Errorf("%w: 账号 %s 不是个人版（Cloud API 账号无需扫码登录）",
			ErrWhatsAppProviderUnsupported, accountID)
	}
	return login.StartLogin(ctx, accountID, timeout)
}

func (s *WhatsappService) GetLoginQR(ctx context.Context, accountID uuid.UUID) (string, bool) {
	login, ok := s.loginProvider(accountID)
	if !ok {
		return "", false
	}
	return login.LoginQR(accountID)
}

// loginProvider 取支持扫码登录的实现。
//
// 先按账号形态判：Cloud API 账号**不该**拿到个人版登录能力（否则给它建一条
// gowa 连接、把会话落进 whatsapp_sessions，而那条会话永远不会被用到）。
// 账号查不到时（建号请求与登录请求并发时可能发生）回落到个人版——
// 那正是「刚建号、还没选形态」的默认路径。
func (s *WhatsappService) loginProvider(accountID uuid.UUID) (WhatsAppLoginProvider, bool) {
	acc, err := s.repo.GetAccount(context.Background(), accountID)
	if err == nil && acc != nil && acc.Type == "cloud" && acc.PhoneID != "" && acc.Token != "" {
		return nil, false
	}
	if s.legacy == nil {
		return nil, false
	}
	return s.legacy, true
}

func (s *WhatsappService) LoginStatus(ctx context.Context, accountID uuid.UUID) (bool, error) {
	sess, err := s.repo.GetSession(ctx, accountID)
	if err != nil {
		return false, err
	}
	return sess != nil, nil
}

// SendTextMessage 发送文本消息到指定 JID。
// 兼容个人版（gowa，见 whatsapp_provider.go 的 seam 说明）与 Cloud API 两种账号。
func (s *WhatsappService) SendTextMessage(ctx context.Context, accountID uuid.UUID, toJID, text string) (string, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	if acc == nil {
		return "", errors.New("whatsapp account not found")
	}
	if err := enforceWhatsAppTierPacing(toJID, "", time.Now()); err != nil {
		return "", err
	}
	return s.providerFor(acc).SendText(ctx, acc, toJID, text)
}

// SendTemplateMessage 发送模板消息（客服窗口外必须走模板）。
func (s *WhatsappService) SendTemplateMessage(ctx context.Context, accountID uuid.UUID, to, templateName, language string, params map[string]string) (string, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	if acc == nil {
		return "", errors.New("whatsapp account not found")
	}
	if acc.PhoneID == "" || acc.Token == "" {
		// 模板消息只有 Cloud API 支持；个人版即便有会话也发不了模板。
		// 这条错误串保留原文案：既有前端/文档按它提示「去填 Cloud API 凭据」。
		return "", errors.New("cloud api credentials missing")
	}
	if err := enforceWhatsAppTierPacing(to, templateName, time.Now()); err != nil {
		return "", err
	}
	return s.providerFor(acc).SendTemplate(ctx, acc, to, templateName, language, params)
}

// Drafts
func (s *WhatsappService) CreateDraft(ctx context.Context, title, content string) (*model.WhatsappDraft, error) {
	d := &model.WhatsappDraft{Title: title, Content: content}
	return d, s.repo.CreateDraft(ctx, d)
}
func (s *WhatsappService) ListDrafts(ctx context.Context) ([]*model.WhatsappDraft, error) {
	return s.repo.ListDrafts(ctx)
}
func (s *WhatsappService) UpdateDraft(ctx context.Context, d *model.WhatsappDraft) error {
	return s.repo.UpdateDraft(ctx, d)
}
func (s *WhatsappService) DeleteDraft(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDraft(ctx, id)
}
func (s *WhatsappService) GetDraft(ctx context.Context, id uuid.UUID) (*model.WhatsappDraft, error) {
	return s.repo.GetDraft(ctx, id)
}

func (s *WhatsappService) CreateBulkJob(ctx context.Context, draftID uuid.UUID) (*model.WhatsappJob, error) {
	draft, err := s.repo.GetDraft(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, errors.New("draft not found")
	}
	job := &model.WhatsappJob{DraftID: draft.ID, Status: model.WhatsappJobPending}
	if err := s.repo.CreateJob(ctx, job); err != nil {
		return nil, err
	}

	clues, total, err := s.clueRepo.GetWhatsappClues(ctx)
	if err != nil {
		return nil, err
	}
	job.Total = total
	_ = s.repo.UpdateJob(ctx, job)

	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, errors.New("no accounts available")
	}

	utils.SafeGo(ctx, "whatsapp.SendBulk", func(ctx context.Context) {
		job.Status = model.WhatsappJobRunning
		_ = s.repo.UpdateJob(ctx, job)
		idx := 0
		for _, clue := range clues {
			acc := accounts[idx%len(accounts)]
			idx++
			toJid := fmt.Sprintf("%s@s.whatsapp.net", clue.Account)
			detail := &model.WhatsappJobDetail{JobID: job.ID, AccountID: acc.ID, ToJid: toJid}
			_ = s.repo.CreateJobDetail(ctx, detail)
			// 群发同样走 provider：Cloud API 账号与个人版账号在同一个批次里混排时，
			// 原来那条"先 ensureConn 再 gowa.Send"的路径对 Cloud 账号必然失败。
			if _, err := s.providerFor(acc).SendText(ctx, acc, toJid, draft.Content); err != nil {
				detail.Status = model.WhatsappJobDetailFailed
				detail.ErrorMsg = err.Error()
				_ = s.repo.UpdateJobDetail(ctx, detail)
				job.Failed++
				_ = s.repo.UpdateJob(ctx, job)
				continue
			}
			detail.Status = model.WhatsappJobDetailSuccess
			_ = s.repo.UpdateJobDetail(ctx, detail)
			job.Success++
			_ = s.repo.UpdateJob(ctx, job)
			time.Sleep(500 * time.Millisecond)
		}
		job.Status = model.WhatsappJobFinished
		_ = s.repo.UpdateJob(ctx, job)
	})

	return job, nil
}

// ListJobs lists all bulk send jobs
func (s *WhatsappService) ListJobs(ctx context.Context) ([]*model.WhatsappJob, error) {
	return s.repo.ListJobs(ctx)
}

// GetJob returns a single bulk send job by ID
func (s *WhatsappService) GetJob(ctx context.Context, id uuid.UUID) (*model.WhatsappJob, error) {
	return s.repo.GetJob(ctx, id)
}

// DeleteJob deletes a bulk send job by ID
func (s *WhatsappService) DeleteJob(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteJob(ctx, id)
}

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/dbstate"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/repository"
)

// ChatChannelService 客服 Web Widget 渠道管理服务
//
// 职责：
//   - 渠道（channel）CRUD
//   - AppKey 签发（生成、轮换、吊销）
//   - AppSecret 哈希校验
//   - 渠道使用统计（visitor_count / session_count）
//
// 五层架构：Service 层，依赖 Repository，被 Controller 调用。
// service 层禁止直接持有/调用 *gorm.DB，db 仅在构造期用于绑定 repository。
type ChatChannelService struct {
	repo *repository.ChatChannelRepository
}

// NewChatChannelService 构造 ChatChannelService
//
// 严格模式：repo 为 nil 时返回 error（不再 panic），由调用方决定降级或 fail-fast。
func NewChatChannelService(repo *repository.ChatChannelRepository) (*ChatChannelService, error) {
	if repo == nil {
		return nil, errors.New("ChatChannelService: repo is nil")
	}
	return &ChatChannelService{repo: repo}, nil
}

// MustNewChatChannelService 兼容性构造（repo 为 nil 时 panic）
//
// 适用于 router/启动阶段（已确保 repo 已初始化），保留旧调用形态
func MustNewChatChannelService(repo *repository.ChatChannelRepository) *ChatChannelService {
	svc, err := NewChatChannelService(repo)
	if err != nil {
		panic(err.Error())
	}
	return svc
}

// ChatChannelIsActive 渠道是否启用（从 model.ChatChannel 迁出，五层架构合规）
func ChatChannelIsActive(c *model.ChatChannel) bool {
	return c.Status == model.ChatChannelStatusActive
}

// ChatChannelAllowedOriginsList 解析允许的 origin 列表（从 model.ChatChannel 迁出）
func ChatChannelAllowedOriginsList(c *model.ChatChannel) []string {
	if c.AllowedOrigins == "" {
		return []string{}
	}
	result := []string{}
	current := ""
	for _, ch := range c.AllowedOrigins {
		if ch == ',' || ch == ';' || ch == ' ' || ch == '\n' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// CreateChannelRequest 创建渠道请求
type CreateChannelRequest struct {
	ChannelName         string   `json:"channel_name" binding:"required,min=1,max=100"`
	AllowedOrigins      []string `json:"allowed_origins" binding:"required,min=1"`
	DefaultRAGProductID string   `json:"default_rag_product_id"`
	WelcomeMessage      string   `json:"welcome_message"`
	WidgetColor         string   `json:"widget_color"`
	WidgetPosition      string   `json:"widget_position"`
	WidgetTitle         string   `json:"widget_title"`
	AutoAssign          *bool    `json:"auto_assign"`
	ConfidenceThreshold *float64 `json:"confidence_threshold"`
	TargetLanguage      string   `json:"target_language"`
}

// UpdateChannelRequest 更新渠道请求
type UpdateChannelRequest struct {
	ChannelName         *string   `json:"channel_name"`
	AllowedOrigins      *[]string `json:"allowed_origins"`
	DefaultRAGProductID *string   `json:"default_rag_product_id"`
	WelcomeMessage      *string   `json:"welcome_message"`
	WidgetColor         *string   `json:"widget_color"`
	WidgetPosition      *string   `json:"widget_position"`
	WidgetTitle         *string   `json:"widget_title"`
	Status              *string   `json:"status"`
	AutoAssign          *bool     `json:"auto_assign"`
	ConfidenceThreshold *float64  `json:"confidence_threshold"`
	TargetLanguage      *string   `json:"target_language"`
}

// ChannelCreateResult 渠道创建结果（含一次性返回的明文凭证）
type ChannelCreateResult struct {
	Channel   *model.ChatChannel `json:"channel"`
	AppKey    string             `json:"app_key"`
	AppSecret string             `json:"app_secret"`
}

// Create 创建渠道（生成 AppKey + AppSecret）
func (s *ChatChannelService) Create(ctx context.Context, req *CreateChannelRequest, createdBy uint) (*ChannelCreateResult, error) {
	if req == nil {
		return nil, errors.New("请求不能为空")
	}
	if strings.TrimSpace(req.ChannelName) == "" {
		return nil, errors.New("渠道名称不能为空")
	}
	if len(req.AllowedOrigins) == 0 {
		return nil, errors.New("至少配置一个允许的 origin")
	}

	appKey, err := generateAppKey()
	if err != nil {
		return nil, fmt.Errorf("生成 AppKey 失败: %w", err)
	}
	appSecret, err := generateAppSecret()
	if err != nil {
		return nil, fmt.Errorf("生成 AppSecret 失败: %w", err)
	}

	channel := &model.ChatChannel{
		ChannelID:           uuid.NewString(),
		ChannelName:         req.ChannelName,
		AppKey:              appKey,
		AppSecretHash:       hashAppSecret(appSecret),
		AllowedOrigins:      strings.Join(req.AllowedOrigins, ","),
		DefaultRAGProductID: ragProductIDStringToUint(req.DefaultRAGProductID),
		WelcomeMessage:      req.WelcomeMessage,
		WidgetColor:         defaultIfEmpty(req.WidgetColor, "#1989fa"),
		WidgetPosition:      defaultIfEmpty(req.WidgetPosition, "bottom-right"),
		WidgetTitle:         defaultIfEmpty(req.WidgetTitle, "在线客服"),
		Status:              model.ChatChannelStatusActive,
		AutoAssign:          true,
		ConfidenceThreshold: 0.70,
		TargetLanguage:      req.TargetLanguage,
		CreatedBy:           createdBy,
	}
	if req.AutoAssign != nil {
		channel.AutoAssign = *req.AutoAssign
	}
	if req.ConfidenceThreshold != nil {
		channel.ConfidenceThreshold = *req.ConfidenceThreshold
	}

	if err := s.repo.Create(ctx, channel); err != nil {
		return nil, fmt.Errorf("保存渠道失败: %w", err)
	}

	return &ChannelCreateResult{
		Channel:   channel,
		AppKey:    appKey,
		AppSecret: appSecret,
	}, nil
}

// Update 更新渠道（不重置 AppKey/AppSecret）
func (s *ChatChannelService) Update(ctx context.Context, channelID string, req *UpdateChannelRequest) (*model.ChatChannel, error) {
	channel, err := s.GetByChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if req.ChannelName != nil {
		updates["channel_name"] = *req.ChannelName
	}
	if req.AllowedOrigins != nil {
		updates["allowed_origins"] = strings.Join(*req.AllowedOrigins, ",")
	}
	if req.DefaultRAGProductID != nil {
		updates["default_rag_product_id"] = ragProductIDStringToUint(*req.DefaultRAGProductID)
	}
	if req.WelcomeMessage != nil {
		updates["welcome_message"] = *req.WelcomeMessage
	}
	if req.WidgetColor != nil {
		updates["widget_color"] = *req.WidgetColor
	}
	if req.WidgetPosition != nil {
		updates["widget_position"] = *req.WidgetPosition
	}
	if req.WidgetTitle != nil {
		updates["widget_title"] = *req.WidgetTitle
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.AutoAssign != nil {
		updates["auto_assign"] = *req.AutoAssign
	}
	if req.ConfidenceThreshold != nil {
		updates["confidence_threshold"] = *req.ConfidenceThreshold
	}
	if req.TargetLanguage != nil {
		updates["target_language"] = *req.TargetLanguage
	}

	if len(updates) == 0 {
		return channel, nil
	}

	if err := s.repo.Updates(ctx, channel.ID, updates); err != nil {
		return nil, fmt.Errorf("更新渠道失败: %w", err)
	}
	return s.GetByChannelID(ctx, channelID)
}

// Delete 删除渠道（软删除：实际改为 disabled）
func (s *ChatChannelService) Delete(ctx context.Context, channelID string) error {
	channel, err := s.GetByChannelID(ctx, channelID)
	if err != nil {
		return err
	}
	return s.repo.UpdateField(ctx, channel.ID, "status", model.ChatChannelStatusDisabled)
}

// HardDelete 硬删除（管理后台使用）
func (s *ChatChannelService) HardDelete(ctx context.Context, channelID string) error {
	return s.repo.HardDeleteByChannelID(ctx, channelID)
}

// guardSystemScopeChannelKey 系统作用域下的业务键歧义守卫。
//
// 渠道行是公开访客链唯一的租户线索，所以"按业务键取渠道"这一步必须用系统作用域
// （请求此时还没有租户）。但 T008 把 chat_channels 的业务唯一索引复合化成
// (tenant_id, channel_id) / (tenant_id, app_key) 之后，同一个 ref 可以在 A、B 两个
// 租户各有一行——`default` 是每租户自动建的，`{platform}_card_app_key` 是固定字面量，
// 两者注定重复。此时 `First()` 取到哪一行由数据库返回顺序决定，等于把访客随机指派
// 给某个租户：他的会话、消息和 AI 回复全部落进另一个租户的收件箱。
//
// 判据只取"命中行分属几个租户"，多于一个就拒（ErrAmbiguousTenantKey 系）。
// 没有命中行时放行——让后续的 record-not-found 分支给出它原有的错误。
// 已绑定租户的 ctx 不需要这一道：隔离 callback 会把读取限死在本租户。
func guardSystemScopeChannelKey(ctx context.Context, column, value string) error {
	if !tenant.IsSystemScope(ctx) {
		return nil
	}
	gdb := dbstate.Get()
	if gdb == nil {
		return nil // 无库：随后的读取自己失败，这里不额外造错误
	}
	_, err := tenant.SingleTenantByKey(ctx, gdb, "chat_channels", column+" = ?", value)
	return err
}

// ChannelReadScope 选取读取渠道行的作用域：ctx 已绑定租户就沿用绑定，
// 未绑定才用系统作用域（判据见 guardSystemScopeChannelKey）。
// 渠道解析是建立租户上下文的引导步骤，公开入口在绑定前后都可能读到这里，
// 因此作用域选择只有这一处事实源——调用方不许自己包 WithSystemScope，
// 那会把中间件已经定好的归属重新摊成跨租户并集。
func ChannelReadScope(ctx context.Context) context.Context {
	if tid, bound := tenant.FromContext(ctx); bound && tid != 0 {
		return ctx
	}
	return tenant.WithSystemScope(ctx)
}

// GetByChannelID 根据 channel_id 查询
func (s *ChatChannelService) GetByChannelID(ctx context.Context, channelID string) (*model.ChatChannel, error) {
	if strings.TrimSpace(channelID) == "" {
		return nil, errors.New("channel_id 不能为空")
	}
	if id, err := strconv.ParseUint(channelID, 10, 64); err == nil {
		if ch, err2 := s.repo.GetByID(ctx, uint(id)); err2 == nil && ch != nil {
			return ch, nil
		}
	}
	if err := guardSystemScopeChannelKey(ctx, "channel_id", channelID); err != nil {
		return nil, err
	}
	ch, err := s.repo.GetByChannelID(ctx, channelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("渠道不存在: %s", channelID)
		}
		return nil, err
	}
	return ch, nil
}

// GetByAppKey 根据 AppKey 查询
func (s *ChatChannelService) GetByAppKey(ctx context.Context, appKey string) (*model.ChatChannel, error) {
	if strings.TrimSpace(appKey) == "" {
		return nil, errors.New("app_key 不能为空")
	}
	if err := guardSystemScopeChannelKey(ctx, "app_key", appKey); err != nil {
		return nil, err
	}
	channel, err := s.repo.GetByAppKey(ctx, appKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("AppKey 无效: %s", appKey)
		}
		return nil, err
	}
	return channel, nil
}

// channelOwnerTenantForCreate 自动建渠道前必须有确定的归属租户。
//
// 为什么这条判据必须存在：SystemScope 下隔离 callback 会跳过 Create 的 tenant_id 填充，
// 行于是落到列默认值 1 —— 每个租户首次接入时自动建的 default/卡片渠道全部建到租户 1
// 名下，访客的会话与 AI 回复从此住在别人的租户里。宁可拒绝服务，也不替访客猜一个租户。
//
// off 模式恒返回默认租户 1，与列默认值同值 ⇒ 单租户部署行为零变更。
func channelOwnerTenantForCreate(ctx context.Context) (uint, bool) {
	if !tenant.IsMultiTenant() {
		return tenant.DefaultTenantID, true
	}
	if tid, bound := tenant.FromContext(ctx); bound && tid != 0 {
		return tid, true
	}
	return 0, false
}

// GetOrCreateDefaultChannel 获取或自动创建默认渠道（私域部署用）
//
// 私域部署模式：用户首次嵌入客服 widget 时，如果 DB 没有 default 渠道，
// 自动创建一个（无需任何配置），保证访客首次接入即可工作。
// 后续用户可以在管理后台修改 default 渠道的配置。
//
// 多租户（on 模式）下"自动创建"只有在 ctx 已绑定租户时才成立——渠道行本身就是租户
// 线索，没有线索就没有归属（判据见 channelOwnerTenantForCreate）。
func (s *ChatChannelService) GetOrCreateDefaultChannel(ctx context.Context) (*model.ChatChannel, error) {
	const defaultID = "default"
	ownerTenant, knownTenant := channelOwnerTenantForCreate(ctx)
	channel, err := s.GetByChannelID(ctx, defaultID)
	if err == nil {
		return channel, nil
	}
	if !knownTenant {
		return nil, fmt.Errorf("默认渠道不存在且当前请求未指明租户归属，拒绝自动创建: %w", err)
	}
	channel = &model.ChatChannel{
		TenantID:            ownerTenant,
		ChannelID:           defaultID,
		ChannelName:         "默认渠道",
		AppKey:              "",
		AppSecretHash:       "",
		AllowedOrigins:      "*",
		WelcomeMessage:      "您好，欢迎咨询！请描述您的问题，AI 助手将为您服务。",
		WidgetColor:         "#1989fa",
		WidgetPosition:      "bottom-right",
		WidgetTitle:         "在线客服",
		Status:              model.ChatChannelStatusActive,
		AutoAssign:          true,
		ConfidenceThreshold: 0.70,
	}
	if err := s.repo.Create(ctx, channel); err != nil {
		return nil, fmt.Errorf("创建默认渠道失败: %w", err)
	}
	return channel, nil
}

type cardChannelMeta struct {
	PlatformLabel string
	ThemeColor    string
}

var cardChannelMetas = map[string]cardChannelMeta{
	"douyin":      {PlatformLabel: "抖音", ThemeColor: "#000000"},
	"kuaishou":    {PlatformLabel: "快手", ThemeColor: "#ff5000"},
	"xiaohongshu": {PlatformLabel: "小红书", ThemeColor: "#ff2442"},
	"xianyu":      {PlatformLabel: "闲鱼", ThemeColor: "#ff4400"},
}

// IsCardChannelRef 判断 channel_ref 是否为卡片渠道标识（{platform}_card）
// platform ∈ {douyin, kuaishou, xiaohongshu, xianyu}
func IsCardChannelRef(ref string) (string, bool) {
	for platform := range cardChannelMetas {
		if ref == platform+"_card" {
			return platform, true
		}
	}
	return "", false
}

// GetOrCreateCardChannel 获取或自动创建卡片渠道（4 平台统一）
//
// 私域部署模式：抖音/快手/小红书/闲鱼 4 个平台的卡片分享页
// 通过此渠道接入网页客服。首次访问时自动创建，无需管理员手动配置。
// 后续可在管理后台修改欢迎语、主题色等。
//
// platform: douyin / kuaishou / xiaohongshu / xianyu
func (s *ChatChannelService) GetOrCreateCardChannel(ctx context.Context, platform string) (*model.ChatChannel, error) {
	meta, ok := cardChannelMetas[platform]
	if !ok {
		return nil, fmt.Errorf("不支持的卡片平台: %s", platform)
	}
	channelID := platform + "_card"
	ownerTenant, knownTenant := channelOwnerTenantForCreate(ctx)
	channel, err := s.GetByChannelID(ctx, channelID)
	if err == nil {
		return channel, nil
	}
	if !knownTenant {
		return nil, fmt.Errorf("卡片渠道 %s 不存在且当前请求未指明租户归属，拒绝自动创建: %w", channelID, err)
	}
	appKey := platform + "_card_app_key"
	if tenant.IsMultiTenant() {
		// on 模式下 app_key 只在租户内唯一：保留固定字面量会让 N 个租户的卡片渠道
		// 共用同一枚键，按 app_key 解析（访客链的回退分支）从此无法自证归属。
		appKey = fmt.Sprintf("%s_card_app_key_t%d", platform, ownerTenant)
	}
	channel = &model.ChatChannel{
		TenantID:            ownerTenant,
		ChannelID:           channelID,
		ChannelName:         meta.PlatformLabel + "卡片客服",
		AppKey:              appKey,
		AppSecretHash:       "",
		AllowedOrigins:      "*",
		WelcomeMessage:      "您好，欢迎咨询！请描述您的问题，AI 助手将为您服务。",
		WidgetColor:         meta.ThemeColor,
		WidgetPosition:      "bottom-right",
		WidgetTitle:         meta.PlatformLabel + " · 在线客服",
		Status:              model.ChatChannelStatusActive,
		AutoAssign:          true,
		ConfidenceThreshold: 0.70,
	}
	if err := s.repo.Create(ctx, channel); err != nil {
		return nil, fmt.Errorf("创建卡片渠道失败: %w", err)
	}
	return channel, nil
}

// List 列出渠道
func (s *ChatChannelService) List(ctx context.Context, keyword string, status string, page, pageSize int) ([]model.ChatChannel, int64, error) {
	return s.repo.ListByQuery(ctx, repository.ChatChannelListQuery{
		Keyword:  keyword,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
}

// RotateAppKey 轮换 AppKey（保留 AppSecret 不变）
func (s *ChatChannelService) RotateAppKey(ctx context.Context, channelID string) (string, error) {
	channel, err := s.GetByChannelID(ctx, channelID)
	if err != nil {
		return "", err
	}
	newKey, err := generateAppKey()
	if err != nil {
		return "", fmt.Errorf("生成 AppKey 失败: %w", err)
	}
	if err := s.repo.UpdateField(ctx, channel.ID, "app_key", newKey); err != nil {
		return "", err
	}
	return newKey, nil
}

// ResetAppSecret 重置 AppSecret（返回明文，仅返回一次）
func (s *ChatChannelService) ResetAppSecret(ctx context.Context, channelID string) (string, error) {
	channel, err := s.GetByChannelID(ctx, channelID)
	if err != nil {
		return "", err
	}
	newSecret, err := generateAppSecret()
	if err != nil {
		return "", fmt.Errorf("生成 AppSecret 失败: %w", err)
	}
	if err := s.repo.UpdateField(ctx, channel.ID, "app_secret_hash", hashAppSecret(newSecret)); err != nil {
		return "", err
	}
	return newSecret, nil
}

// IncrementVisitorCount 增加访客计数
func (s *ChatChannelService) IncrementVisitorCount(ctx context.Context, channelID string) error {
	return s.repo.IncrementVisitorCount(ctx, channelID)
}

// IncrementSessionCount 增加会话计数
func (s *ChatChannelService) IncrementSessionCount(ctx context.Context, channelID string) error {
	return s.repo.IncrementSessionCount(ctx, channelID)
}

const appKeyAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

func generateAppKey() (string, error) {
	const length = 32
	result := make([]byte, length)
	alphabetLen := big.NewInt(int64(len(appKeyAlphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		result[i] = appKeyAlphabet[n.Int64()]
	}
	return "ak_" + string(result[:24]), nil
}

func generateAppSecret() (string, error) {
	const length = 48
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk_" + hex.EncodeToString(buf), nil
}

func hashAppSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

func ragProductIDStringToUint(s string) uint {
	if s == "" {
		return 0
	}
	const (
		offset64 = 1469598103934665603
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return uint(h & 0x7FFFFFFFFFFFFFFF)
}

// VerifyAppSecret 校验 AppSecret
func (s *ChatChannelService) VerifyAppSecret(ctx context.Context, channel *model.ChatChannel, secret string) bool {
	if channel == nil || secret == "" {
		return false
	}
	return channel.AppSecretHash == hashAppSecret(secret)
}

func defaultIfEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

var _ = time.Second

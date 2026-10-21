package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// SiteConfigInput 站点配置的可编辑字段。
//
// 指针区分「未传」与「传了零值」（Update 只覆盖传了的字段）；
// id/tenant_id/health_score/last_deploy_at 由系统维护，不接受客户端输入。
type SiteConfigInput struct {
	Domain    *string
	Provider  *string
	HugoPath  *string
	GitRepo   *string
	DeployCmd *string
	Active    *bool
}

// SiteConfigService 站点部署配置读写编排（配置 CRUD；导出/部署见 SiteDeployService）
type SiteConfigService struct {
	repo repository.GeoSiteRepository
}

func NewSiteConfigService(repo repository.GeoSiteRepository) *SiteConfigService {
	return &SiteConfigService{repo: repo}
}

// List 分页查询；limit 上限 200。
func (s *SiteConfigService) List(ctx context.Context, page, limit int) ([]*model.GeoSite, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	return s.repo.List(ctx, page, limit)
}

func (s *SiteConfigService) Get(ctx context.Context, id string) (*model.GeoSite, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *SiteConfigService) Create(ctx context.Context, in SiteConfigInput) (*model.GeoSite, error) {
	domain, err := requiredTrimmed(in.Domain, "domain")
	if err != nil {
		return nil, err
	}
	site := &model.GeoSite{
		Domain:   domain,
		Provider: "cloudflare",
		Active:   true,
	}
	if in.Provider != nil && strings.TrimSpace(*in.Provider) != "" {
		site.Provider = strings.TrimSpace(*in.Provider)
	}
	if in.HugoPath != nil {
		site.HugoPath = strings.TrimSpace(*in.HugoPath)
	}
	if in.GitRepo != nil {
		site.GitRepo = strings.TrimSpace(*in.GitRepo)
	}
	if in.DeployCmd != nil {
		site.DeployCmd = strings.TrimSpace(*in.DeployCmd)
	}
	if in.Active != nil {
		site.Active = *in.Active
	}
	if err := s.repo.Create(ctx, site); err != nil {
		return nil, err
	}
	return site, nil
}

func (s *SiteConfigService) Update(ctx context.Context, id string, in SiteConfigInput) (*model.GeoSite, error) {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Domain != nil {
		domain, err := requiredTrimmed(in.Domain, "domain")
		if err != nil {
			return nil, err
		}
		site.Domain = domain
	}
	if in.Provider != nil {
		v := strings.TrimSpace(*in.Provider)
		if v == "" {
			return nil, errors.New("provider 不能为空")
		}
		site.Provider = v
	}
	if in.HugoPath != nil {
		site.HugoPath = strings.TrimSpace(*in.HugoPath)
	}
	if in.GitRepo != nil {
		site.GitRepo = strings.TrimSpace(*in.GitRepo)
	}
	if in.DeployCmd != nil {
		site.DeployCmd = strings.TrimSpace(*in.DeployCmd)
	}
	if in.Active != nil {
		site.Active = *in.Active
	}
	if err := s.repo.Update(ctx, site); err != nil {
		return nil, err
	}
	site.UpdatedAt = time.Now()
	return site, nil
}

func (s *SiteConfigService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

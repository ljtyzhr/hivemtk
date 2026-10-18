package service

import (
	"context"
	"encoding/json"
	"strings"

	"hivemtk-user/internal/config"
	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/platform"
)

// ---------------------------------------------------------- 付款指引

// RegulatoryNoticeView 监管告知视图
type RegulatoryNoticeView struct {
	Notice           string   `json:"notice"`
	AgreementVersion string   `json:"agreement_version"`
	RequiredFields   []string `json:"required_fields"`
}

// LocalRegulatoryNotice 离线兜底的监管告知文案。
//
// 与平台端 service/tenant_registration_service.go 的 RegulatoryNotice 同义不同源：
// 平台不可达时用户仍必须看到告知并勾选，否则离线部署根本没法开户
// （需求原文明确要求"注意离线独立部署情况"）。两端同义不同源是可接受的 ——
// 这里的代价只是"两端文案可能不一致"，而漏掉勾选的后果是登记缺失。
//
// 版本号字面量只有一个权威出处：service/tenant.go 的 serviceCurrentAgreementVersion。
// 这里只保留别名不复制字面量 —— 同一进程里两处各写一份版本号，
// 改版本时漏改一处就会让「用户同意的是哪一版」在两条链路上对不上。
const LocalAgreementVersion = serviceCurrentAgreementVersion

const LocalRegulatoryNotice = "本平台为软件产品服务方，不参与商户与最终客户之间的具体交易。" +
	"商户应自行确保其经营活动合法合规，并对使用本平台产生的行为承担全部责任。" +
	"平台将按法律法规要求配合监管问询。"

var localRequiredFields = []string{"company_name", "legal_person", "phone", "email"}

// RegulatoryNotice 取监管告知。优先平台（两端同源），失败回落本地常量。
func (s *ModuleCatalogService) RegulatoryNotice(ctx context.Context) *RegulatoryNoticeView {
	fallback := RegulatoryNoticeView{
		Notice:           LocalRegulatoryNotice,
		AgreementVersion: LocalAgreementVersion,
		RequiredFields:   localRequiredFields,
	}
	if !config.PlatformEnabled() {
		return &fallback
	}
	notice, version, required, err := platform.NewModuleClient().RegulatoryNotice(ctx)
	if err != nil || strings.TrimSpace(notice) == "" {
		return &fallback
	}
	if len(required) == 0 {
		required = localRequiredFields
	}
	if strings.TrimSpace(version) == "" {
		version = LocalAgreementVersion
	}
	return &RegulatoryNoticeView{Notice: notice, AgreementVersion: version, RequiredFields: required}
}

// PaymentGuide 取付款指引。平台不可达时给固定兜底，绝不返回空串。
func (s *ModuleCatalogService) PaymentGuide(ctx context.Context) (string, error) {
	const fallback = "订单已提交，请联系平台运营完成付款；付款确认后权益自动开通。"
	if !config.PlatformEnabled() {
		return fallback, nil
	}
	g, err := platform.NewModuleClient().PaymentGuide(ctx)
	if err != nil || strings.TrimSpace(g) == "" {
		return fallback, nil
	}
	return g, nil
}

var _ = json.Marshal
var _ = bizerr.CodeSuccess

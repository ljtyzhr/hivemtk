package repository

import (
	"context"
	"encoding/json"
	"errors"

	"hivemtk-user/internal/model"
)

// ErrSecurityRulesNotFound 尚未写入过配置。
var ErrSecurityRulesNotFound = errors.New("security rules not found")

type SecurityRulesRepository interface {
	Available() bool
	Get(ctx context.Context) (*model.SecurityRules, error)
	Upsert(ctx context.Context, rules *model.SecurityRules) error
}

type securityRulesRepo struct {
	kv PlatformConfigKVRepository
}

func NewSecurityRulesRepository(kv PlatformConfigKVRepository) SecurityRulesRepository {
	if kv == nil {
		return nil
	}
	return &securityRulesRepo{kv: kv}
}

func (r *securityRulesRepo) Available() bool {
	return r != nil && r.kv != nil
}

func (r *securityRulesRepo) Get(ctx context.Context) (*model.SecurityRules, error) {
	if !r.Available() {
		return nil, ErrSecurityRulesNotFound
	}
	raw, err := r.kv.Get(ctx, model.SecurityRulesConfigKey)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, ErrSecurityRulesNotFound
	}
	var rules model.SecurityRules
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, err
	}
	return &rules, nil
}

func (r *securityRulesRepo) Upsert(ctx context.Context, rules *model.SecurityRules) error {
	if !r.Available() || rules == nil {
		return ErrSecurityRulesNotFound
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	_, err = r.kv.Upsert(ctx, model.SecurityRulesConfigKey, string(raw))
	return err
}

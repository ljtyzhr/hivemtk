package dto

import (
	_type "hivemtk-user/internal/pkg/utils/type"
)

type AccountResponse struct {
	ID               string                  `json:"id"`
	TgName           string                  `json:"tg_name"`
	TgBotToken       string                  `json:"tg_bot_token"`
	Price            string                  `json:"price"`
	GroupID          int64                   `json:"group_id"`
	ProxyEnableProxy bool                    `json:"proxy_enable_proxy"`
	ProxyProtoclo    string                  `json:"proxy_protoclo"`
	ProxyHost        string                  `json:"proxy_host"`
	ProxyPort        int                     `json:"proxy_port"`
	Status           _type.AccountStatusType `json:"status"`
	CreateTime       int64                   `json:"create_time"`
	Msg              string                  `json:"msg"`
	URL              string                  `json:"url"`
}

type GetAccountListResponse struct {
	Total int64              `json:"total"`
	List  []*AccountResponse `json:"list"`
}

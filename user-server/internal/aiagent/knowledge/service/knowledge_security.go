// Package service 知识库子域 —— 安全/输入校验
//
// URL 校验(SSRF 防护)+ HTML 标签剥离等安全相关函数；
// 本文件保留为薄包装以维持外部兼容。
package service

import (
	"context"

	"hivemtk-user/internal/pkg/utils/text"
	urlutil "hivemtk-user/internal/pkg/utils/url"
)

func validateURL(rawURL string) error {
	return urlutil.ValidateURL(context.Background(), rawURL)
}

func stripHTML(html string) string {
	return text.StripHTML(html)
}

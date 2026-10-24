// Package service 知识库子域 —— 通用辅助函数
//
// 与 service 业务无关的可复用工具（mime 推断）。
package service

func getMimeType(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".docx", ".doc":
		return "application/msword"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".html":
		return "text/html"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	default:
		return "application/octet-stream"
	}
}

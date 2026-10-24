package etl

import (
	"strings"
	"testing"
)

// TestIsPlainTextExt G13 反向对账：结构化后缀（提取可失败）与纯文本后缀（永不报错）的分界。
func TestIsPlainTextExt(t *testing.T) {
	plain := []string{".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".log", ".xml", ".yml", ".yaml", ".text"}
	structured := []string{".pdf", ".docx", ".doc", ".html", ".htm", ".bin", ""}

	for _, ext := range plain {
		if !IsPlainTextExt("doc" + ext) {
			t.Errorf("IsPlainTextExt(%q)=false, want true", ext)
		}
	}
	for _, ext := range structured {
		if IsPlainTextExt("doc" + ext) {
			t.Errorf("IsPlainTextExt(%q)=true, want false", ext)
		}
	}
	// 大写后缀同口径
	if !IsPlainTextExt("a.TXT") || IsPlainTextExt("a.PDF") {
		t.Error("IsPlainTextExt 大写后缀应等价小写")
	}
}

// TestExtractText_StructuredFailIsError 锁定：结构化文档提取失败返回 error 而非静默原文。
func TestExtractText_StructuredFailIsError(t *testing.T) {
	// 伪 PDF 字节：非 PDF 魔数（G13 场景）
	if _, err := ExtractText("broken.pdf", []byte("this is not a pdf\x00\x01")); err == nil {
		t.Error("伪 PDF 提取应返回 error")
	}
	// 扫描件形态：合法 PDF 结构（手工 xref 表，extractPDF 可解析但零文本页）
	scanPDF := "%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\nxref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \ntrailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n186\n%%EOF"
	if _, err := ExtractText("scan.pdf", []byte(scanPDF)); err == nil || !strings.Contains(err.Error(), "扫描件") {
		t.Errorf("空文本 PDF 应报扫描件错，实际: %v", err)
	}
}

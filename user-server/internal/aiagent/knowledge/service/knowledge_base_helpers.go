// Package service 知识库子域 —— 辅助工具函数
package service

import (
	"os"
)

func getFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

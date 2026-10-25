package service

import (
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// 测试专用句柄注入（生产侧这三个服务的 repository 自取全局库）。
//
// 为什么留在 _test.go：显式注入是"同一用例要多库"时的测试手段，不是 service 的
// 公开能力。放到生产文件里会被当成可用的依赖注入口，反过来鼓励 service 接句柄。
func (s *BridgeOfflineReplayService) WithDB(d *gorm.DB) *BridgeOfflineReplayService {
	s.repo = repository.NewBridgeOfflineReplayRepositoryWithDB(d)
	return s
}

func (s *HandoffChainService) WithDB(d *gorm.DB) *HandoffChainService {
	s.repo = repository.NewHandoffChainRepositoryWithDB(d)
	return s
}

func (s *KnowledgeUpdateService) WithDB(d *gorm.DB) *KnowledgeUpdateService {
	s.repo = repository.NewKBDocumentChunkRepositoryWithDB(d)
	return s
}

package service

// ProvideModelDetectionService 复用项目账号测试请求层，启动持久化检测工作线程。
func ProvideModelDetectionService(repo ModelDetectionRepository, accounts AccountRepository, probe *AccountTestService) *ModelDetectionService {
	s := NewModelDetectionService(repo, accounts, probe)
	s.Start()
	return s
}

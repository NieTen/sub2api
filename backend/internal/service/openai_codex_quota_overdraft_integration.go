package service

func (s *OpenAIGatewayService) SetCodexQuotaOverdraftCoordinator(coordinator *CodexQuotaOverdraftCoordinator) {
	if s != nil {
		s.codexQuotaOverdraft = coordinator
	}
}

func (s *AccountUsageService) SetCodexQuotaOverdraftCoordinator(coordinator *CodexQuotaOverdraftCoordinator) {
	if s != nil {
		s.codexQuotaOverdraft = coordinator
	}
}

// codexQuotaOverdraftCoordinator 返回 OpenAI 网关持有的单例协调器。
// 在这里构造可避免为透支功能额外改动 Wire 生成图。
func (s *OpenAIGatewayService) codexQuotaOverdraftCoordinator(
	tlsFPProfileService *TLSFingerprintProfileService,
) *CodexQuotaOverdraftCoordinator {
	if s == nil {
		return nil
	}
	s.codexQuotaOverdraftOnce.Do(func() {
		if s.codexQuotaOverdraft != nil {
			return
		}
		var tempUnschedCache TempUnschedCache
		if s.rateLimitService != nil {
			tempUnschedCache = s.rateLimitService.tempUnschedCache
		}
		s.codexQuotaOverdraft = NewCodexQuotaOverdraftCoordinator(
			s.accountRepo,
			s.httpUpstream,
			s.openAITokenProvider,
			tlsFPProfileService,
			s.cfg,
			tempUnschedCache,
			s,
			s.rateLimitService,
		)
	})
	return s.codexQuotaOverdraft
}

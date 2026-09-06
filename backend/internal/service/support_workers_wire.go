package service

// ProvideCommunityService 启动持久化社群审批及邀请撤销任务。
func ProvideCommunityService(settings SettingRepository, repo CommunityRepository, delivery *SupportDeliveryService) *CommunityService {
	svc := NewCommunityService(settings, repo, delivery)
	svc.Start()
	return svc
}

// ProvideSupportDeliveryService 由依赖注入启动工单通知任务，应用退出时统一停止。
func ProvideSupportDeliveryService(settings SettingRepository, email *EmailService, repo SupportTicketRepository, tickets *SupportTicketService) *SupportDeliveryService {
	svc := NewSupportDeliveryService(settings, email, repo, tickets)
	svc.Start()
	return svc
}

// ProvideBulkEmailService 批量邮件使用持久化任务，重启后继续处理未完成收件人。
func ProvideBulkEmailService(repo BulkEmailRepository, email *EmailService, attachments SupportTicketRepository) *BulkEmailService {
	svc := NewBulkEmailService(repo, email, attachments)
	svc.Start()
	return svc
}

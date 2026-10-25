package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/textutil"
	"time"
)

func sendCertificateDomainAlertNotification(item models.CertificateDomain, alert *models.AlertInstance, recovery bool, now time.Time) {
	if alert == nil || alert.ID == 0 {
		return
	}
	config := loadAlertRuleNotificationConfig(alert.FaultCenterID)
	if recovery {
		if queueGroupedRecoveryAlert(alert, config, now) {
			dispatchDueAlertGroups(now)
			return
		}
	} else if queueGroupedFiringAlert(alert, config, now) {
		dispatchDueAlertGroups(now)
		return
	}
	if recovery && (!config.RecoveryNotification || alert.LastNotifiedAt == nil) {
		status, reason := resolvedAlertNotificationStatus(alert.LastNotifiedAt), "在首次通知前恢复"
		if !config.RecoveryNotification && alert.LastNotifiedAt != nil {
			status, reason = notificationStatusRecoveryDisabled, "恢复通知未启用"
		}
		updateAlertNotificationState(alert, status, reason, alert.NotificationGroupKey, nil)
		return
	}
	if !recovery {
		if alert.LastNotifiedAt != nil && now.Before(alert.LastNotifiedAt.Add(time.Duration(maxInt(config.RepeatSeconds, 60))*time.Second)) {
			return
		}
		if allowed, _ := alertInstanceNotificationAllowed(alert, alert.LastNotifiedAt != nil, now); !allowed {
			return
		}
	}
	channelIDs := notificationRouteChannelIDs(*alert, config.Routes, config.ChannelIDs)
	if len(channelIDs) == 0 {
		updateAlertNotificationState(alert, notificationStatusNoReceiver, "没有匹配到可用通知对象", "", nil)
		return
	}

	status, eventType, titleColor := "firing", "certificate.domain.firing", alertNotificationTitleColor(alert.Severity, false)
	title := "证书域名告警：" + item.Domain
	content := certificateDomainNotificationContent(item, false)
	if alert.LastNotifiedAt != nil {
		title = "证书域名持续告警：" + item.Domain
	}
	if recovery {
		status, eventType, titleColor = "resolved", "certificate.domain.recovery", alertNotificationTitleColor(alert.Severity, true)
		title = "证书域名已恢复：" + item.Domain
		content = certificateDomainNotificationContent(item, true)
	}
	payload := map[string]interface{}{
		"title": title, "content": content, "title_color": titleColor, "source": "certificate-domain",
		"fault_center_id": alert.FaultCenterID, "certificate_domain_id": item.ID,
		"domain": item.Domain, "port": defaultPort(item.Port), "status": status,
		"certificate_status": item.Status, "days_remaining": item.DaysRemaining,
		"expires_at": item.ObservedExpiresAt, "expiry_alert_days": item.ExpiryAlertDays,
		"severity": alert.Severity, "level": alert.Severity, "fingerprint": alert.Fingerprint,
		"fired_at": alert.FiredAt, "recovered_at": alert.ResolvedAt, "error": item.LastCheckError, "checked_at": now,
	}
	namespaces := alertLabelNamespaces(*alert)
	addNotificationLabelNamespace(payload, "system_labels", namespaces["system_labels"])
	addNotificationLabelNamespace(payload, "labels", namespaces["labels"])
	addNotificationLabelNamespace(payload, "datasource_labels", namespaces["datasource_labels"])
	addNotificationLabelNamespace(payload, "external_labels", namespaces["external_labels"])

	sent, attempted := false, false
	for _, channelID := range channelIDs {
		var channel models.NotificationChannel
		if database.DB.Where("id = ? AND status = 1", channelID).First(&channel).Error != nil {
			continue
		}
		attempted = true
		eventKey := fmt.Sprintf("certificate-domain:%d:%s:%d", item.ID, status, now.Unix())
		if sendNotificationToChannel(channel, eventType, eventKey, title, payload) == nil {
			sent = true
		}
	}
	if sent {
		notificationStatus := notificationStatusSent
		if recovery {
			notificationStatus = notificationStatusRecoverySent
		}
		updateAlertNotificationState(alert, notificationStatus, "通知已发送", "", nil)
		if !recovery {
			database.DB.Model(alert).Update("last_notified_at", now)
			alert.LastNotifiedAt = &now
		}
		_ = recordAlertEvent(database.DB, *alert, "notification_sent", alert.Status, alert.Status, alert.Severity, alert.Severity, status, item.Status, alert.LastProbeStatus, now)
	} else if attempted {
		updateAlertNotificationState(alert, notificationStatusSendFailed, "通知发送失败", "", nil)
		_ = recordAlertEvent(database.DB, *alert, "notification_failed", alert.Status, alert.Status, alert.Severity, alert.Severity, status, item.Status, alert.LastProbeStatus, now)
	}
}

func certificateDomainNotificationContent(item models.CertificateDomain, recovery bool) string {
	target := fmt.Sprintf("%s:%d", item.Domain, defaultPort(item.Port))
	if recovery {
		return fmt.Sprintf("域名：%s\n本次检测成功，证书状态已恢复正常。\n证书到期时间：%s\n剩余天数：%d 天", target, formatCertificateDomainNotificationTime(item.ObservedExpiresAt), item.DaysRemaining)
	}
	if item.Status == "failed" {
		return fmt.Sprintf("域名：%s\n证书检测失败：%s", target, textutil.Ellipsis(item.LastCheckError, 800))
	}
	return fmt.Sprintf("域名：%s\n证书状态：%s\n证书到期时间：%s\n剩余天数：%d 天\n到期提醒：%d 天", target, statusTextForCertificateAlert(item.Status), formatCertificateDomainNotificationTime(item.ObservedExpiresAt), item.DaysRemaining, item.ExpiryAlertDays)
}

func formatCertificateDomainNotificationTime(value *time.Time) string {
	if value == nil {
		return "—"
	}
	return value.Format("2006-01-02 15:04:05")
}

package workers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/logger"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"gorm.io/gorm"
)

const defaultExportSchedulerInterval = 1 * time.Minute

// StartExportSchedulerWorker starts a background worker that generates and delivers
// scheduled transaction exports via email or webhook.
func StartExportSchedulerWorker(ctx context.Context, wg *sync.WaitGroup, db *gorm.DB, cfg *config.Config) {
	emailService := services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled).WithDB(db)
	webhookService := services.NewWebhookDeliveryService(db)
	httpClient := &http.Client{Timeout: 15 * time.Second}

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Log.Info("Export scheduler worker started")

		ticker := time.NewTicker(defaultExportSchedulerInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Log.Info("Export scheduler worker stopped")
				return
			case <-ticker.C:
				runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				if _, err := ProcessDueExportSchedules(runCtx, db, emailService, webhookService, httpClient, time.Now().UTC()); err != nil {
					logger.Log.WithField("error", err).Error("Export scheduler cycle failed")
				}
				cancel()
			}
		}
	}()
}

// ProcessDueExportSchedules finds active export schedules with next_run_at <= now,
// generates the export payload, and delivers it via email or webhook.
func ProcessDueExportSchedules(
	ctx context.Context,
	db *gorm.DB,
	emailService *services.EmailService,
	webhookService *services.WebhookDeliveryService,
	httpClient *http.Client,
	now time.Time,
) (int, error) {
	nowUTC := now.UTC()
	var schedules []models.ExportSchedule
	if err := db.WithContext(ctx).
		Where("is_active = ? AND next_run_at <= ?", true, nowUTC).
		Order("next_run_at ASC").
		Limit(50).
		Find(&schedules).Error; err != nil {
		return 0, err
	}

	processed := 0
	for i := range schedules {
		sched := &schedules[i]
		err := executeExportSchedule(ctx, db, sched, emailService, webhookService, httpClient, nowUTC)

		nextRun := models.ComputeNextExportRun(nowUTC, sched.Frequency)
		updates := map[string]interface{}{
			"last_run_at": nowUTC,
			"next_run_at": nextRun,
		}
		if err != nil {
			updates["last_status"] = "failed"
			updates["last_error"] = err.Error()
		} else {
			updates["last_status"] = "success"
			updates["last_error"] = ""
			processed++
		}
		_ = db.WithContext(ctx).Model(sched).Updates(updates).Error
	}

	return processed, nil
}

func executeExportSchedule(
	ctx context.Context,
	db *gorm.DB,
	sched *models.ExportSchedule,
	emailService *services.EmailService,
	webhookService *services.WebhookDeliveryService,
	httpClient *http.Client,
	now time.Time,
) error {
	var payments []models.Payment
	if err := db.WithContext(ctx).
		Where("sender_id = ? OR recipient_id = ?", sched.UserID, sched.UserID).
		Order("created_at DESC").
		Limit(1000).
		Find(&payments).Error; err != nil {
		return fmt.Errorf("fetch payments: %w", err)
	}

	content, err := renderExportCSV(payments)
	if err != nil {
		return fmt.Errorf("render export: %w", err)
	}

	switch sched.Destination {
	case "email":
		if emailService == nil {
			return fmt.Errorf("email service not configured")
		}
		var user models.User
		if err := db.WithContext(ctx).First(&user, sched.UserID).Error; err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		subject := fmt.Sprintf("Scheduled %s Transaction Export (%s)", sched.Frequency, now.Format("2006-01-02"))
		body := fmt.Sprintf("<p>Hello %s,</p><p>Your scheduled %s transaction export (%s, %d records) is ready.</p><pre>%s</pre>",
			user.Name, sched.Frequency, sched.Format, len(payments), string(content))
		return emailService.SendEmail(user.Email, subject, body)

	case "webhook":
		payload := map[string]interface{}{
			"event":        "export.completed",
			"schedule_id":  sched.ID,
			"user_id":      sched.UserID,
			"frequency":    sched.Frequency,
			"format":       sched.Format,
			"row_count":    len(payments),
			"generated_at": now.Format(time.RFC3339),
			"content":      string(content),
		}
		if sched.WebhookURL != "" {
			if httpClient == nil {
				httpClient = &http.Client{Timeout: 15 * time.Second}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshal webhook payload: %w", err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, sched.WebhookURL, bytes.NewReader(raw))
			if err != nil {
				return fmt.Errorf("create webhook request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("deliver webhook: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("webhook returned status %d", resp.StatusCode)
			}
			return nil
		}
		if webhookService != nil {
			return webhookService.TriggerWebhook("export.completed", payload)
		}
		return fmt.Errorf("no webhook URL or webhook service configured")

	default:
		return fmt.Errorf("unsupported export destination: %s", sched.Destination)
	}
}

func renderExportCSV(payments []models.Payment) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"ID", "Created At", "Amount", "Currency", "Status"}); err != nil {
		return nil, err
	}
	for _, p := range payments {
		if err := writer.Write([]string{
			fmt.Sprintf("%d", p.ID),
			p.CreatedAt.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%.2f", float64(p.Amount)/100.0),
			p.Currency,
			p.Status,
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"time"

	"github.com/yourusername/gpay-remit/models"
)

type EmailService struct {
	smtpHost     string
	smtpPort     string
	smtpUser     string
	smtpPassword string
	fromEmail    string
	enabled      bool
}

type EmailTemplate struct {
	Subject string
	Body    string
}

// NewEmailService creates a new email service
func NewEmailService(host, port, user, password, from string, enabled bool) *EmailService {
	return &EmailService{
		smtpHost:     host,
		smtpPort:     port,
		smtpUser:     user,
		smtpPassword: password,
		fromEmail:    from,
		enabled:      enabled,
	}
}

// SendEmail sends an email using SMTP
func (s *EmailService) SendEmail(to, subject, body string) error {
	if !s.enabled {
		// Email is disabled, skip sending
		return nil
	}

	// Setup headers
	headers := make(map[string]string)
	headers["From"] = s.fromEmail
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=\"utf-8\""

	// Build message
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	// Setup authentication
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPassword, s.smtpHost)

	// Setup TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         s.smtpHost,
	}

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to dial SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.smtpHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Quit()

	// Authenticate
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set sender
	if err := client.Mail(s.fromEmail); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipient
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// Send email body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	_, err = w.Write([]byte(message))
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return nil
}

// SendPaymentCompletedEmail sends notification when payment is completed
func (s *EmailService) SendPaymentCompletedEmail(user *models.User, payment *models.Payment) error {
	if !user.EmailNotifications {
		return nil // User has opted out
	}

	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; }
        .content { padding: 20px; background-color: #f9f9f9; }
        .details { background-color: white; padding: 15px; border-radius: 5px; margin: 15px 0; }
        .detail-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #eee; }
        .label { font-weight: bold; }
        .footer { text-align: center; padding: 20px; font-size: 12px; color: #777; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Payment Completed ✓</h1>
        </div>
        <div class="content">
            <p>Hello {{.UserName}},</p>
            <p>Your payment has been completed successfully!</p>
            
            <div class="details">
                <h3>Payment Details</h3>
                <div class="detail-row">
                    <span class="label">Payment ID:</span>
                    <span>{{.PaymentID}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Amount:</span>
                    <span>{{.Amount}} {{.Currency}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Recipient:</span>
                    <span>{{.RecipientAccount}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Fee:</span>
                    <span>{{.Fee}} {{.Currency}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Status:</span>
                    <span style="color: #4CAF50; font-weight: bold;">{{.Status}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Date:</span>
                    <span>{{.Date}}</span>
                </div>
            </div>
            
            <p>Thank you for using GPay-Remit!</p>
        </div>
        <div class="footer">
            <p>This is an automated email. Please do not reply.</p>
            <p>To manage your email preferences, visit your account settings.</p>
        </div>
    </div>
</body>
</html>
`

	t, err := template.New("payment_completed").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	data := map[string]interface{}{
		"UserName":         user.Name,
		"PaymentID":        payment.ID,
		"Amount":           fmt.Sprintf("%.2f", payment.Amount),
		"Currency":         payment.Currency,
		"RecipientAccount": payment.RecipientAccount,
		"Fee":              fmt.Sprintf("%.4f", payment.Fee),
		"Status":           payment.Status,
		"Date":             payment.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	var body bytes.Buffer
	if err := t.Execute(&body, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	subject := fmt.Sprintf("Payment #%d Completed Successfully", payment.ID)
	return s.SendEmail(user.Email, subject, body.String())
}

// SendEscrowExpirationWarningEmail sends warning when escrow is about to expire
func (s *EmailService) SendEscrowExpirationWarningEmail(user *models.User, payment *models.Payment, hoursRemaining int) error {
	if !user.EmailNotifications {
		return nil // User has opted out
	}

	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #FF9800; color: white; padding: 20px; text-align: center; }
        .content { padding: 20px; background-color: #f9f9f9; }
        .warning { background-color: #fff3cd; border-left: 4px solid #FF9800; padding: 15px; margin: 15px 0; }
        .details { background-color: white; padding: 15px; border-radius: 5px; margin: 15px 0; }
        .detail-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #eee; }
        .label { font-weight: bold; }
        .cta { text-align: center; margin: 20px 0; }
        .button { background-color: #FF9800; color: white; padding: 12px 30px; text-decoration: none; border-radius: 5px; display: inline-block; }
        .footer { text-align: center; padding: 20px; font-size: 12px; color: #777; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>⚠️ Escrow Expiration Warning</h1>
        </div>
        <div class="content">
            <p>Hello {{.UserName}},</p>
            
            <div class="warning">
                <strong>Action Required:</strong> Your escrow payment is set to expire in <strong>{{.HoursRemaining}} hours</strong>.
            </div>
            
            <p>Please take action before the escrow expires to avoid losing your funds.</p>
            
            <div class="details">
                <h3>Payment Details</h3>
                <div class="detail-row">
                    <span class="label">Payment ID:</span>
                    <span>{{.PaymentID}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Amount:</span>
                    <span>{{.Amount}} {{.Currency}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Recipient:</span>
                    <span>{{.RecipientAccount}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Escrow ID:</span>
                    <span>{{.EscrowID}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Time Remaining:</span>
                    <span style="color: #FF9800; font-weight: bold;">{{.HoursRemaining}} hours</span>
                </div>
            </div>
            
            <div class="cta">
                <a href="#" class="button">View Payment Details</a>
            </div>
            
            <p>If you have any questions or need assistance, please contact our support team.</p>
        </div>
        <div class="footer">
            <p>This is an automated email. Please do not reply.</p>
            <p>To manage your email preferences, visit your account settings.</p>
        </div>
    </div>
</body>
</html>
`

	t, err := template.New("escrow_expiration").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	data := map[string]interface{}{
		"UserName":         user.Name,
		"PaymentID":        payment.ID,
		"Amount":           fmt.Sprintf("%.2f", payment.Amount),
		"Currency":         payment.Currency,
		"RecipientAccount": payment.RecipientAccount,
		"EscrowID":         payment.EscrowID,
		"HoursRemaining":   hoursRemaining,
	}

	var body bytes.Buffer
	if err := t.Execute(&body, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	subject := fmt.Sprintf("⚠️ Escrow Expiring Soon - Payment #%d", payment.ID)
	return s.SendEmail(user.Email, subject, body.String())
}

// SendPaymentFailedEmail sends notification when payment fails
func (s *EmailService) SendPaymentFailedEmail(user *models.User, payment *models.Payment, reason string) error {
	if !user.EmailNotifications {
		return nil // User has opted out
	}

	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #f44336; color: white; padding: 20px; text-align: center; }
        .content { padding: 20px; background-color: #f9f9f9; }
        .error { background-color: #ffebee; border-left: 4px solid #f44336; padding: 15px; margin: 15px 0; }
        .details { background-color: white; padding: 15px; border-radius: 5px; margin: 15px 0; }
        .detail-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #eee; }
        .label { font-weight: bold; }
        .footer { text-align: center; padding: 20px; font-size: 12px; color: #777; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Payment Failed ✗</h1>
        </div>
        <div class="content">
            <p>Hello {{.UserName}},</p>
            
            <div class="error">
                <strong>Payment Failed:</strong> Your payment could not be completed.
            </div>
            
            <p><strong>Reason:</strong> {{.Reason}}</p>
            
            <div class="details">
                <h3>Payment Details</h3>
                <div class="detail-row">
                    <span class="label">Payment ID:</span>
                    <span>{{.PaymentID}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Amount:</span>
                    <span>{{.Amount}} {{.Currency}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Recipient:</span>
                    <span>{{.RecipientAccount}}</span>
                </div>
                <div class="detail-row">
                    <span class="label">Date:</span>
                    <span>{{.Date}}</span>
                </div>
            </div>
            
            <p>You can try again or contact support if you need assistance.</p>
        </div>
        <div class="footer">
            <p>This is an automated email. Please do not reply.</p>
            <p>To manage your email preferences, visit your account settings.</p>
        </div>
    </div>
</body>
</html>
`

	t, err := template.New("payment_failed").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	data := map[string]interface{}{
		"UserName":         user.Name,
		"PaymentID":        payment.ID,
		"Amount":           fmt.Sprintf("%.2f", payment.Amount),
		"Currency":         payment.Currency,
		"RecipientAccount": payment.RecipientAccount,
		"Reason":           reason,
		"Date":             time.Now().Format("2006-01-02 15:04:05"),
	}

	var body bytes.Buffer
	if err := t.Execute(&body, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	subject := fmt.Sprintf("Payment #%d Failed", payment.ID)
	return s.SendEmail(user.Email, subject, body.String())
}

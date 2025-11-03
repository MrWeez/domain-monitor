package handlers

import (
	"log"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/nwesterhausen/domain-monitor/service"
)

type MailerHandler struct {
	MailerService *service.MailerService
	Recipient     string
}

func NewMailerHandler(ms *service.MailerService, recipient string) *MailerHandler {
	return &MailerHandler{
		MailerService: ms,
		Recipient:     recipient,
	}
}

func (mh MailerHandler) HandleTestMail(c echo.Context) error {
	if mh.MailerService == nil {
		log.Println("⚠️ Test mail requested but MailerService is nil")
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
		return c.HTML(200, `<span class="text-error">❌ SMTP mailer service is not initialized. Please check server logs for details and ensure SMTP is properly configured.</span>`)
	}
	
	if mh.Recipient == "" {
		log.Println("⚠️ Test mail requested but recipient email is empty")
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
		return c.HTML(200, `<span class="text-error">❌ Admin email is not set. Please configure admin email in Alerts settings.</span>`)
	}
	
	log.Printf("📧 Attempting to send test email to %s (timeout: 35 seconds)", mh.Recipient)
	
	// Run email sending in goroutine to avoid blocking HTTP request
	resultChan := make(chan error, 1)
	go func() {
		resultChan <- mh.MailerService.TestMail(mh.Recipient)
	}()
	
	// Wait for result with timeout
	select {
	case err := <-resultChan:
		if err != nil {
			log.Printf("❌ Failed to send test mail to %s: %s", mh.Recipient, err)
			c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
			errorMsg := err.Error()
			if len(errorMsg) > 200 {
				errorMsg = errorMsg[:200] + "..."
			}
			return c.HTML(200, `<span class="text-error">❌ `+errorMsg+`</span>`)
		}
		log.Printf("✅ Test mail sent successfully to %s", mh.Recipient)
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
		return c.HTML(200, `<span class="text-success">✅ Test email sent successfully to `+mh.Recipient+`!</span>`)
	case <-time.After(35 * time.Second):
		log.Printf("❌ Test mail request timed out after 35 seconds")
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
		return c.HTML(200, `<span class="text-error">❌ Request timed out after 35 seconds. SMTP server is unreachable or not responding.<br/>Please check:<br/>• SMTP host and port are correct<br/>• Server is accessible from this network<br/>• Firewall allows outbound connections on SMTP port</span>`)
	}
}

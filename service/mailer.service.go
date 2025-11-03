package service

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/nwesterhausen/domain-monitor/configuration"
	"github.com/wneessen/go-mail"
)

type MailerService struct {
	client *mail.Client
	from   string
	host   string
	port   int
}

func NewMailerService(config configuration.SMTPConfiguration) *MailerService {
	var client *mail.Client
	var err error

	// check if SMTP is enabled
	if !config.Enabled {
		log.Println("⚠️ SMTP is not enabled in configuration")
		return nil
	}

	// Validate required fields
	if config.Host == "" {
		log.Println("⚠️ SMTP Host is not set")
		return nil
	}
	if config.Port == 0 {
		log.Println("⚠️ SMTP Port is not set")
		return nil
	}
	if config.FromAddress == "" {
		log.Println("⚠️ SMTP FromAddress is not set")
		return nil
	}

	// check if SMTP user and password are set, otherwise use none
	var authStyle = mail.SMTPAuthLogin
	if config.AuthUser == "" || config.AuthPass == "" {
		log.Println("⚠️ SMTP AuthUser or AuthPass is empty, using no authentication")
		// auth type None is empty string
		authStyle = ""
	}

	// Determine encryption type (backward compatible with old Secure field)
	encryptionType := config.EncryptionType
	if encryptionType == "" {
		// Legacy support: migrate from Secure boolean to EncryptionType
		if config.Port == 25 {
			encryptionType = "none"
		} else if config.Port == 465 {
			encryptionType = "ssl"
		} else {
			// Default to STARTTLS for ports 587 and others
			encryptionType = "starttls"
		}
	}

	// Normalize old values to new format
	if encryptionType == "tls" || encryptionType == "starttls-mandatory" || encryptionType == "starttls-opportunistic" {
		encryptionType = "starttls"
	}

	// Build options based on encryption type
	var opts []mail.Option
	switch encryptionType {
	case "ssl":
		log.Printf("📧 Creating SMTP client with SSL (port 465): host=%s, port=%d, auth=%v", config.Host, config.Port, authStyle != "")
		opts = []mail.Option{
			mail.WithPort(config.Port),
			mail.WithSSL(), // Enable SSL for implicit TLS (port 465)
			mail.WithSMTPAuth(authStyle),
			mail.WithUsername(config.AuthUser),
			mail.WithPassword(config.AuthPass),
			mail.WithTimeout(30*time.Second),
		}
	case "starttls":
		log.Printf("📧 Creating SMTP client with STARTTLS (port 587): host=%s, port=%d, auth=%v", config.Host, config.Port, authStyle != "")
		opts = []mail.Option{
			mail.WithTLSPortPolicy(mail.TLSMandatory), // Use mandatory for STARTTLS
			mail.WithPort(config.Port),
			mail.WithSMTPAuth(authStyle),
			mail.WithUsername(config.AuthUser),
			mail.WithPassword(config.AuthPass),
			mail.WithTimeout(30*time.Second),
		}
	case "none":
		log.Printf("📧 Creating SMTP client without encryption (port 25): host=%s, port=%d, auth=%v", config.Host, config.Port, authStyle != "")
		opts = []mail.Option{
			mail.WithPort(config.Port),
			mail.WithSMTPAuth(authStyle),
			mail.WithUsername(config.AuthUser),
			mail.WithPassword(config.AuthPass),
			mail.WithTimeout(30*time.Second),
		}
	default:
		log.Printf("⚠️ Unknown encryption type '%s', defaulting to STARTTLS", encryptionType)
		encryptionType = "starttls"
		opts = []mail.Option{
			mail.WithTLSPortPolicy(mail.TLSMandatory),
			mail.WithPort(config.Port),
			mail.WithSMTPAuth(authStyle),
			mail.WithUsername(config.AuthUser),
			mail.WithPassword(config.AuthPass),
			mail.WithTimeout(30*time.Second),
		}
	}

	// create new mail client (note: this doesn't actually connect, just creates the client object)
	client, err = mail.NewClient(config.Host, opts...)
	if err != nil {
		log.Printf("❌ Failed to create mail client: %s", err)
		return nil
	}

	// combine from name and address
	from := config.FromName
	if from == "" {
		from = config.FromAddress
	}
	from = from + " <" + config.FromAddress + ">"

	log.Printf("✅ SMTP mailer service initialized successfully")
	return &MailerService{
		client: client,
		from:   from,
		host:   config.Host,
		port:   config.Port,
	}
}

func (m *MailerService) TestMail(to string) error {
	log.Printf("📧 Preparing test email to %s", to)
	msg := mail.NewMsg()
	if err := msg.From(m.from); err != nil {
		log.Printf("❌ Failed to set FROM address: %s", err)
		return err
	}
	if err := msg.To(to); err != nil {
		log.Printf("❌ Failed to set TO address: %s", err)
		return err
	}
	msg.Subject("Test E-Mail from Domain Monitor")
	msg.SetBodyString(mail.TypeTextPlain, "This is a test e-mail from the Domain Monitor application. If you received this, it's working! 🎉")

	// Quick connectivity check before attempting full SMTP connection
	log.Printf("📧 Checking SMTP server connectivity to %s:%d...", m.host, m.port)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", m.host, m.port), 5*time.Second)
	if err != nil {
		log.Printf("❌ Cannot reach SMTP server %s:%d - %s", m.host, m.port, err)
		return fmt.Errorf("SMTP server unreachable: %s. Please check host, port, and network connectivity", err)
	}
	conn.Close()
	log.Printf("✅ SMTP server is reachable, attempting to send email...")
	
	// Use goroutine with timeout to avoid blocking
	done := make(chan error, 1)
	timeout := make(chan bool, 1)
	
	go func() {
		err := m.client.DialAndSend(msg)
		select {
		case done <- err:
		default:
		}
	}()
	
	go func() {
		time.Sleep(25 * time.Second) // 25 second timeout (5s already spent on connectivity check)
		select {
		case timeout <- true:
		default:
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("❌ Failed to deliver mail: %s", err)
			return fmt.Errorf("SMTP error: %w", err)
		}
		log.Printf("✅ E-mail message sent successfully to %s", to)
		return nil
	case <-timeout:
		log.Printf("❌ SMTP operation timed out after 25 seconds - authentication or sending may be failing")
		return fmt.Errorf("SMTP operation timeout: connection established but sending failed. Check authentication credentials")
	}
}

func (m *MailerService) SendAlert(to string, fqdn string, alert configuration.Alert) error {
	msg := mail.NewMsg()
	if err := msg.From(m.from); err != nil {
		log.Printf("❌ failed to set FROM address: %s", err)
		return err
	}
	if err := msg.To(to); err != nil {
		log.Printf("❌ failed to set TO address: %s", err)
		return err
	}
	msg.Subject("Domain Expiration Alert: " + fqdn)

	body := fmt.Sprintf("Your domain %s is expiring in %s. Please renew it as soon as possible.", fqdn, alert)

	msg.SetBodyString(mail.TypeTextPlain, body)

	if err := m.client.DialAndSend(msg); err != nil {
		log.Printf("❌ failed to deliver mail: %s", err)
		return err
	}

	log.Printf("📧 E-mail message sent to " + to)

	return nil
}

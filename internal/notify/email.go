package notify

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"github.com/find-assets/scanner/internal/exporter"
)

var ErrMissingPassword = errors.New("smtp password is empty")

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	To       string
}

func BuildReportEmail(cfg Config, rep *exporter.Report) ([]byte, error) {
	if rep == nil {
		return nil, errors.New("report is nil")
	}
	from := strings.TrimSpace(cfg.From)
	to := strings.TrimSpace(cfg.To)
	if from == "" || to == "" {
		return nil, errors.New("mail from/to is required")
	}

	var body bytes.Buffer
	if err := (exporter.Markdown{}).Write(&body, rep); err != nil {
		return nil, err
	}

	subject := fmt.Sprintf("find-assets 命中提醒：%s 命中 %d 个", rep.Mode, rep.Matched)
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	fmt.Fprintln(&msg, "MIME-Version: 1.0")
	fmt.Fprintln(&msg, `Content-Type: text/plain; charset="UTF-8"`)
	fmt.Fprintln(&msg, "Content-Transfer-Encoding: 8bit")
	fmt.Fprintln(&msg)
	msg.Write(body.Bytes())
	return msg.Bytes(), nil
}

func SendReport(cfg Config, rep *exporter.Report) error {
	if strings.TrimSpace(cfg.Password) == "" {
		return ErrMissingPassword
	}
	msg, err := BuildReportEmail(cfg, rep)
	if err != nil {
		return err
	}
	return sendSMTP(cfg, msg)
}

func sendSMTP(cfg Config, msg []byte) error {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return errors.New("smtp host is required")
	}
	port := cfg.Port
	if port == 0 {
		port = 465
	}
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	auth := smtp.PlainAuth("", cfg.User, cfg.Password, host)

	if port == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Quit()
		return sendWithClient(client, auth, cfg, msg)
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Quit()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	return sendWithClient(client, auth, cfg, msg)
}

func sendWithClient(client *smtp.Client, auth smtp.Auth, cfg Config, msg []byte) error {
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(cfg.To); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

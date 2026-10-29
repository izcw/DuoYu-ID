package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

func purposeTitle(p string) string {
	switch p {
	case "reset":
		return "重置你的密码"
	case "email":
		return "确认邮箱变更"
	default:
		return "验证你的邮箱"
	}
}
func (s *Server) localMail() bool {
	host := env("SMTP_HOST", "smtp.qq.com")
	return s.dev && os.Getenv("SMTP_PLAIN_LOCAL") == "true" && (host == "localhost" || host == "127.0.0.1" || host == "mailpit")
}

func (s *Server) mailReceipt() map[string]any {
	return map[string]any{"retry_after": 60, "local_mail": s.localMail()}
}
func (s *Server) emailHTML(code, purpose string) (string, error) {
	var b bytes.Buffer
	e := s.templates.ExecuteTemplate(&b, "email.html", map[string]string{"Code": code, "Title": purposeTitle(purpose), "Logo": s.origin + "/logo.png"})
	return b.String(), e
}
func (s *Server) sendMail(to, code, purpose string) error {
	html, e := s.emailHTML(code, purpose)
	if e != nil {
		return e
	}
	return s.enqueueAuth(MailMessage{To: to, Subject: "【多鱼账号】" + purposeTitle(purpose), Text: fmt.Sprintf("多鱼账号 DuoYu ID\r\n%s\r\n验证码：%s\r\n5 分钟内有效，请勿向他人透露。如果不是你的操作，请忽略。", purposeTitle(purpose), code), HTML: html}, to, purpose, code)
}

type MailMessage struct{ To, Subject, Text, HTML string }

func (s *Server) sendMessage(message MailMessage) (result error) {
	dataStarted := false
	defer func() {
		if result != nil && !dataStarted {
			kind := "failed"
			var smtpErr *textproto.Error
			var netErr net.Error
			if errors.As(result, &smtpErr) && smtpErr.Code >= 400 && smtpErr.Code < 500 {
				kind = "retry"
			} else if errors.As(result, &netErr) {
				kind = "retry"
			}
			result = &deliveryError{kind: kind, cause: result}
		}
	}()
	if s.mailTransport != nil {
		dataStarted = true
		return s.mailTransport(message)
	}
	host := env("SMTP_HOST", "smtp.qq.com")
	port := env("SMTP_PORT", "465")
	from := normalizeEmail(env("SMTP_FROM", os.Getenv("SMTP_USER")))
	if from == "" {
		return errors.New("SMTP sender not configured")
	}
	var e error
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ typ, body string }{{"text/plain", message.Text}, {"text/html", message.HTML}} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", part.typ+"; charset=UTF-8")
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		p, _ := mw.CreatePart(h)
		q := quotedprintable.NewWriter(p)
		q.Write([]byte(part.body))
		q.Close()
	}
	mw.Close()
	headers := []string{"From: " + (&mail.Address{Name: "多鱼账号", Address: from}).String(), "To: " + message.To, "Subject: " + mime.QEncoding.Encode("utf-8", message.Subject), "Date: " + time.Now().Format(time.RFC1123Z), "Message-ID: <" + randomID() + "@" + strings.Split(from, "@")[1] + ">", "MIME-Version: 1.0", "Content-Type: multipart/alternative; boundary=" + mw.Boundary()}
	msg := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body.String())
	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	plain := s.localMail()
	if plain {
		conn, e = dialer.Dial("tcp", addr)
	} else {
		conn, e = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	}
	if e != nil {
		return e
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	c, e := smtp.NewClient(conn, host)
	if e != nil {
		return e
	}
	defer c.Close()
	if !plain {
		if os.Getenv("SMTP_PASSWORD") == "" {
			return errors.New("SMTP password missing")
		}
		if e = c.Auth(smtp.PlainAuth("", os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASSWORD"), host)); e != nil {
			return e
		}
	}
	if e = c.Mail(from); e != nil {
		return e
	}
	if e = c.Rcpt(message.To); e != nil {
		return e
	}
	wr, e := c.Data()
	if e != nil {
		return e
	}
	dataStarted = true
	if _, e = wr.Write(msg); e != nil {
		return e
	}
	if e = wr.Close(); e != nil {
		return e
	}
	// DATA succeeded: the server has accepted the mail even if QUIT fails.
	_ = c.Quit()
	return nil
}

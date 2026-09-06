package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"net/textproto"
	"strings"
)

// EmailInlineImage 使用邮件内嵌附件，避免收件人访问站点鉴权链接。
type EmailInlineImage struct {
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

func buildSMTPMessageWithImages(config *SMTPConfig, to, subject, body string, images []EmailInlineImage) (smtpMessage, error) {
	base, err := buildSMTPMessage(config, to, subject, body)
	if err != nil || len(images) == 0 {
		return base, err
	}
	var related bytes.Buffer
	w := multipart.NewWriter(&related)
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", "text/html; charset=UTF-8")
	header.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := w.CreatePart(header)
	if err != nil {
		return smtpMessage{}, err
	}
	qp := quotedprintable.NewWriter(part)
	if _, err = qp.Write([]byte(body)); err != nil {
		return smtpMessage{}, err
	}
	if err = qp.Close(); err != nil {
		return smtpMessage{}, err
	}
	for i, image := range images {
		if image.MimeType != "image/jpeg" && image.MimeType != "image/png" && image.MimeType != "image/gif" && image.MimeType != "image/webp" {
			return smtpMessage{}, fmt.Errorf("不支持的邮件图片类型")
		}
		header = textproto.MIMEHeader{}
		header.Set("Content-Type", image.MimeType)
		header.Set("Content-Transfer-Encoding", "base64")
		header.Set("Content-Disposition", "inline")
		header.Set("Content-ID", fmt.Sprintf("<image-%d>", i))
		part, err = w.CreatePart(header)
		if err != nil {
			return smtpMessage{}, err
		}
		encoded := base64.StdEncoding.EncodeToString(image.Data)
		for len(encoded) > 0 {
			n := min(76, len(encoded))
			if _, err = fmt.Fprint(part, encoded[:n], "\r\n"); err != nil {
				return smtpMessage{}, err
			}
			encoded = encoded[n:]
		}
	}
	if err = w.Close(); err != nil {
		return smtpMessage{}, err
	}
	head := strings.SplitN(string(base.data), "\r\nMIME-Version:", 2)[0] + "\r\n"
	base.data = []byte(head + "MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=\"" + w.Boundary() + "\"\r\n\r\n" + related.String())
	return base, nil
}

// SendEmailWithImages 复用现有 SMTP 配置、TLS 和连接超时，每个收件人独立发送。
func (s *EmailService) SendEmailWithImages(ctx context.Context, to, subject, body string, images []EmailInlineImage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	config, err := s.GetSMTPConfig(ctx)
	if err != nil {
		return err
	}
	message, err := buildSMTPMessageWithImages(config, to, subject, body, images)
	if err != nil {
		return err
	}
	client, err := s.connectSMTP(config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	// SMTP 方法没有 context 参数，通过关闭连接中断已取消的批次或通知。
	stopCancel := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stopCancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err = client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.Host)); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err = client.Mail(message.envelopeFrom); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err = client.Rcpt(message.envelopeTo); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(message.data); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	_ = client.Quit()
	return nil
}

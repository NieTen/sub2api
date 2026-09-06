package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"testing"

	"github.com/stretchr/testify/require"
)

// 验证四张图片经过 MIME 编码后仍能逐张还原，且正文引用与附件编号一致。
func TestSupportDeliveryFourImagesRoundTrip(t *testing.T) {
	images := make([]EmailInlineImage, SupportTicketMaxAttachments)
	for i := range images {
		images[i] = EmailInlineImage{FileName: fmt.Sprintf("图片%d.png", i), MimeType: "image/png", Data: supportTestPNG(t)}
	}
	body := supportEmailHTML("图片测试", images)
	message, err := buildSMTPMessageWithImages(&SMTPConfig{Host: "smtp.example.com", From: "sender@example.com"}, "recipient@example.com", "工单图片", body, images)
	require.NoError(t, err)
	parsed, err := mail.ReadMessage(bytes.NewReader(message.data))
	require.NoError(t, err)
	_, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	require.NoError(t, err)
	parts := multipart.NewReader(parsed.Body, params["boundary"])
	textPart, err := parts.NextPart()
	require.NoError(t, err)
	text, err := io.ReadAll(textPart)
	require.NoError(t, err)
	for i, expected := range images {
		require.Contains(t, string(text), fmt.Sprintf("cid:image-%d", i))
		part, err := parts.NextPart()
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("<image-%d>", i), part.Header.Get("Content-ID"))
		actual, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		require.NoError(t, err)
		require.Equal(t, expected.Data, actual)
	}
	_, err = parts.NextPart()
	require.ErrorIs(t, err, io.EOF)
}

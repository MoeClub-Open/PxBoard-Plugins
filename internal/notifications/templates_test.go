package notifications

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestTemplatesValidateLinksAndEscapeUserContent(t *testing.T) {
	data := sampleData()
	data.UserName = `<img src=x onerror=alert(1)>`
	for _, item := range templateDefaults {
		out, err := renderMail(item, data)
		if err != nil {
			t.Fatalf("%s: %v", item.ID, err)
		}
		if strings.Contains(out.HTML, data.UserName) {
			t.Fatal("user content became HTML")
		}
		if item.ID == "password-reset" && !strings.Contains(out.Text, data.ResetURL) {
			t.Fatal("reset link lost")
		}
	}
	item, _ := defaultTemplate("password-reset")
	item.Text = "重置密码"
	if _, err := renderMail(item, data); err == nil {
		t.Fatal("unusable password reset template was accepted")
	}
	item, _ = defaultTemplate("general")
	item.Subject = "{{.UnknownVariable}}"
	if _, err := renderMail(item, data); err == nil {
		t.Fatal("unknown variable was accepted")
	}
	item.Subject = "{{.UserName}}"
	data.UserName = "name\r\nBcc: unwanted@example.invalid"
	if _, err := renderMail(item, data); err == nil {
		t.Fatal("header injection was accepted")
	}
}

func TestMultipartMailPreservesChineseTextAndHTML(t *testing.T) {
	message := p.Record{"id": "preview-only", "to": "preview@example.invalid", "subject": "订单已开通", "body": "您好，套餐已开通。", "html": "<p>您好，套餐已开通。</p>"}
	out, err := encodeMail(p.Record{"from": "服务 <sender@example.invalid>"}, message)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatal(err)
	}
	format, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || format != "multipart/alternative" {
		t.Fatal("mail is not multipart", err)
	}
	reader := multipart.NewReader(parsed.Body, params["boundary"])
	for _, expected := range []string{message["body"].(string), message["html"].(string)} {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(part)
		if string(body) != expected {
			t.Fatal("message content changed during encoding")
		}
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatal("unexpected MIME part", err)
	}
}

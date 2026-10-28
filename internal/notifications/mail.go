package notifications

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

func send(config, message p.Record) error {
	payload, err := encodeMail(config, message)
	if err != nil {
		return err
	}
	host := p.Text(config, "host")
	port := int(p.Number(config, "port"))
	if port == 0 {
		port = 587
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if port != 465 {
		if err = client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if p.Text(config, "username") != "" {
		if err = client.Auth(smtp.PlainAuth("", p.Text(config, "username"), p.Text(config, "password"), host)); err != nil {
			return err
		}
	}
	if err = client.Mail(payload.From); err != nil {
		return err
	}
	if err = client.Rcpt(payload.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	_, err = writer.Write(payload.Data)
	if err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	_ = client.Quit()
	return nil
}

type smtpMessage struct {
	From, To string
	Data     []byte
}

func encodeMail(config, message p.Record) (smtpMessage, error) {
	result := smtpMessage{}
	from, to, subject := p.Text(config, "from"), p.Text(message, "to"), p.Text(message, "subject")
	if strings.ContainsAny(from+to+subject, "\r\n") {
		return result, fmt.Errorf("邮件头无效")
	}
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return result, err
	}
	recipient, err := mail.ParseAddress(to)
	if err != nil {
		return result, err
	}
	var body bytes.Buffer
	contentType, encoding := "text/plain; charset=UTF-8", "Content-Transfer-Encoding: quoted-printable\r\n"
	if p.Text(message, "html") == "" {
		writer := quotedprintable.NewWriter(&body)
		if _, err = writer.Write([]byte(p.Text(message, "body"))); err != nil {
			return result, err
		}
		if err = writer.Close(); err != nil {
			return result, err
		}
	} else {
		multi := multipart.NewWriter(&body)
		contentType, encoding = mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": multi.Boundary()}), ""
		for _, part := range []struct{ format, body string }{{"text/plain", p.Text(message, "body")}, {"text/html", p.Text(message, "html")}} {
			writer, err := multi.CreatePart(textproto.MIMEHeader{"Content-Type": {part.format + "; charset=UTF-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
			if err != nil {
				return result, err
			}
			encoded := quotedprintable.NewWriter(writer)
			if _, err = encoded.Write([]byte(part.body)); err != nil {
				return result, err
			}
			if err = encoded.Close(); err != nil {
				return result, err
			}
		}
		if err = multi.Close(); err != nil {
			return result, err
		}
	}
	var output bytes.Buffer
	messageID := base64.RawURLEncoding.EncodeToString([]byte(p.Text(message, "id")))
	fmt.Fprintf(&output, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s@pxboard>\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\n%s\r\n", sender.String(), recipient.String(), mime.QEncoding.Encode("utf-8", subject), messageID, time.Now().Format(time.RFC1123Z), contentType, encoding)
	output.Write(body.Bytes())
	result.From, result.To, result.Data = sender.Address, recipient.Address, output.Bytes()
	return result, nil
}

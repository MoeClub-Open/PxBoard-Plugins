package support

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func send(config, message p.Record) error {
	host := p.Text(config, "host")
	port := int(p.Number(config, "port"))
	if port == 0 {
		port = 587
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	var err error
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
	from := p.Text(config, "from")
	to := p.Text(message, "to")
	if strings.ContainsAny(from+to+p.Text(message, "subject"), "\r\n") {
		return fmt.Errorf("邮件头无效")
	}
	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return err
	}
	toAddress, err := mail.ParseAddress(to)
	if err != nil {
		return err
	}
	if err = client.Mail(fromAddress.Address); err != nil {
		return err
	}
	if err = client.Rcpt(toAddress.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	messageID := base64.RawURLEncoding.EncodeToString([]byte(p.Text(message, "id")))
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s@pxboard>\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", fromAddress.String(), toAddress.String(), mime.QEncoding.Encode("utf-8", p.Text(message, "subject")), messageID, time.Now().Format(time.RFC1123Z), p.Text(message, "body"))
	if err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	_ = client.Quit()
	return nil
}

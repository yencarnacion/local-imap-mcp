package emailparse

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
)

type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size,omitempty"`
}

type Email struct {
	UID         uint32       `json:"uid"`
	Mailbox     string       `json:"mailbox"`
	Subject     string       `json:"subject"`
	From        []string     `json:"from"`
	To          []string     `json:"to"`
	CC          []string     `json:"cc"`
	Date        string       `json:"date"`
	MessageID   string       `json:"message_id"`
	TextBody    string       `json:"text_body"`
	HTMLBody    string       `json:"html_body"`
	Attachments []Attachment `json:"attachments"`
}

func Parse(raw []byte, uid uint32, mailboxName string) (*Email, error) {
	reader, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}
	defer reader.Close()

	email := &Email{
		UID:         uid,
		Mailbox:     mailboxName,
		From:        addresses(&reader.Header, "From"),
		To:          addresses(&reader.Header, "To"),
		CC:          addresses(&reader.Header, "Cc"),
		Attachments: []Attachment{},
	}
	if subject, err := reader.Header.Subject(); err == nil {
		email.Subject = subject
	}
	if messageID, err := reader.Header.MessageID(); err == nil {
		email.MessageID = messageID
	}
	if date, err := reader.Header.Date(); err == nil {
		email.Date = date.Format(time.RFC3339)
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse MIME part: %w", err)
		}

		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			contentType := partContentType(&h.Header)
			body, err := io.ReadAll(part.Body)
			if err != nil {
				return nil, fmt.Errorf("read inline part: %w", err)
			}
			switch {
			case strings.HasPrefix(contentType, "text/plain"):
				email.TextBody += string(body)
			case strings.HasPrefix(contentType, "text/html"):
				email.HTMLBody += string(body)
			}
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			contentType := partContentType(&h.Header)
			size, err := discardCount(part.Body)
			if err != nil {
				return nil, fmt.Errorf("read attachment metadata: %w", err)
			}
			email.Attachments = append(email.Attachments, Attachment{
				Filename:    filename,
				ContentType: contentType,
				Size:        size,
			})
		}
	}

	return email, nil
}

func Headers(raw []byte) (map[string][]string, error) {
	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse headers: %w", err)
	}
	headers := make(map[string][]string)
	fields := entity.Header.Fields()
	for fields.Next() {
		key := fields.Key()
		value, err := fields.Text()
		if err != nil {
			value = fields.Value()
		}
		headers[key] = append(headers[key], value)
	}
	return headers, nil
}

func addresses(h *mail.Header, key string) []string {
	list, err := h.AddressList(key)
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(list))
	for _, addr := range list {
		if addr == nil {
			continue
		}
		if addr.Name != "" {
			out = append(out, fmt.Sprintf("%s <%s>", addr.Name, addr.Address))
		} else {
			out = append(out, addr.Address)
		}
	}
	return out
}

func partContentType(h *message.Header) string {
	contentType, _, err := h.ContentType()
	if err != nil || contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func discardCount(r io.Reader) (int64, error) {
	return io.Copy(io.Discard, r)
}

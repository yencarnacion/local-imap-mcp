package emailparse

import (
	"strings"
	"testing"
)

func TestParseKeepsFullTextBody(t *testing.T) {
	raw := strings.Join([]string{
		"From: Sender <sender@example.com>",
		"To: Recipient <recipient@example.com>",
		"Subject: Full body",
		"Date: Tue, 26 May 2026 12:07:00 -0400",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"First line.",
		"Middle line.",
		"Final line without synthetic ellipsis.",
	}, "\r\n")

	email, err := Parse([]byte(raw), 19260, "INBOX")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !strings.Contains(email.TextBody, "Final line without synthetic ellipsis.") {
		t.Fatalf("text body missing final line: %q", email.TextBody)
	}
	if strings.HasSuffix(email.TextBody, "...") {
		t.Fatalf("text body has synthetic trailing ellipsis: %q", email.TextBody)
	}
}

package imapclient

import (
	"testing"
	"time"
)

func TestSubjectTokens(t *testing.T) {
	got := subjectTokens("Re: Online Reading Summary")
	want := []string{"Online", "Reading", "Summary"}

	if len(got) != len(want) {
		t.Fatalf("subjectTokens length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("subjectTokens[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSubjectCriteriaMatchesTokens(t *testing.T) {
	criteria := subjectCriteria("Online Reading Summary")
	if len(criteria.Header) != 3 {
		t.Fatalf("subject criteria headers = %d, want 3", len(criteria.Header))
	}

	for i, want := range []string{"Online", "Reading", "Summary"} {
		if criteria.Header[i].Key != "Subject" {
			t.Fatalf("criteria.Header[%d].Key = %q, want Subject", i, criteria.Header[i].Key)
		}
		if criteria.Header[i].Value != want {
			t.Fatalf("criteria.Header[%d].Value = %q, want %q", i, criteria.Header[i].Value, want)
		}
	}
}

func TestEnsureAllMessages(t *testing.T) {
	criteria := subjectCriteria("Online Reading Summary")
	ensureAllMessages(criteria)

	if len(criteria.SeqNum) != 1 {
		t.Fatalf("SeqNum length = %d, want 1", len(criteria.SeqNum))
	}
	if got := criteria.SeqNum[0].String(); got != "1:*" {
		t.Fatalf("SeqNum = %q, want 1:*", got)
	}
}

func TestIMAPDateFormat(t *testing.T) {
	date := time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC)
	if got := imapDate(date); got != "20-May-2026" {
		t.Fatalf("imapDate = %q, want 20-May-2026", got)
	}
}

func TestSummaryOnOrAfterUsesInternalDate(t *testing.T) {
	summary := MessageSummary{InternalDate: "2026-05-20T12:00:00Z"}
	cutoff := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	if !summaryOnOrAfter(summary, cutoff) {
		t.Fatal("expected summary to match cutoff via internalDate")
	}
}

func TestSubjectMatchesPrefixedSubject(t *testing.T) {
	if !subjectMatches("Online Reading Summary", "(US) Friday Morning Online Reading Summary") {
		t.Fatal("expected query to match prefixed subject")
	}
}

func TestSubjectMatchesWordsWhenPhraseIsInterrupted(t *testing.T) {
	if !subjectMatches("Online Reading Summary", "Friday Online and Morning Reading Summary") {
		t.Fatal("expected query terms to match when present outside exact phrase")
	}
}

func TestSubjectMatchesRejectsMissingTerm(t *testing.T) {
	if subjectMatches("Online Reading Summary", "(US) Friday Morning Online Reading") {
		t.Fatal("expected missing Summary term not to match")
	}
}

func TestScanCursorRoundTrip(t *testing.T) {
	want := scanCursor{
		Mailbox:     "AllMail",
		UIDValidity: 123,
		BeforeUID:   456,
		AfterUID:    10,
		StartDate:   "2026-05-01",
	}

	got, err := decodeScanCursor(encodeScanCursor(want))
	if err != nil {
		t.Fatalf("decodeScanCursor returned error: %v", err)
	}
	if got != want {
		t.Fatalf("cursor round trip = %#v, want %#v", got, want)
	}
}

func TestHeaderMatchesFiltersSenderDomainAndUnread(t *testing.T) {
	summary := MessageSummary{
		From:         []string{"Example Sender <person@alerts.example.com>"},
		InternalDate: "2026-05-20T12:00:00Z",
	}
	query := HeaderScanQuery{SenderDomain: "example.com", UnreadOnly: true}
	cutoff := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)

	if !headerMatches(summary, query, cutoff) {
		t.Fatal("expected unread message from matching sender domain to pass")
	}

	summary.Flags = []string{"\\Seen"}
	if headerMatches(summary, query, cutoff) {
		t.Fatal("expected seen message to be rejected when unreadOnly is true")
	}
}

func TestApplyThreadHeaders(t *testing.T) {
	raw := []byte("Message-ID: <m1@example.com>\r\nIn-Reply-To: <m0@example.com>\r\nReferences: <m0@example.com> <root@example.com>\r\n\r\n")
	var summary MessageSummary

	applyThreadHeaders(&summary, raw)

	if summary.MessageID != "<m1@example.com>" {
		t.Fatalf("MessageID = %q", summary.MessageID)
	}
	if summary.InReplyTo != "<m0@example.com>" {
		t.Fatalf("InReplyTo = %q", summary.InReplyTo)
	}
	if len(summary.References) != 2 {
		t.Fatalf("References length = %d, want 2: %#v", len(summary.References), summary.References)
	}
}

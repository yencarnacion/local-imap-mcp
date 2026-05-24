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

func TestNextUIDBatchCapsAtLowUIDWithoutUnderflow(t *testing.T) {
	low, span, ok := nextUIDBatch(39, 0, 1000)
	if !ok {
		t.Fatal("expected batch")
	}
	if low != 1 {
		t.Fatalf("low = %d, want 1", low)
	}
	if span != 39 {
		t.Fatalf("span = %d, want 39", span)
	}
}

func TestNextUIDBatchHonorsAfterUID(t *testing.T) {
	low, span, ok := nextUIDBatch(40, 10, 1000)
	if !ok {
		t.Fatal("expected batch")
	}
	if low != 11 {
		t.Fatalf("low = %d, want 11", low)
	}
	if span != 30 {
		t.Fatalf("span = %d, want 30", span)
	}
}

func TestThreadKeyUsesRootReference(t *testing.T) {
	summary := MessageSummary{
		UID:        10,
		Mailbox:    "AllMail",
		MessageID:  "<reply@example.com>",
		InReplyTo:  "<parent@example.com>",
		References: []string{"<root@example.com>", "<parent@example.com>"},
	}

	if got := threadKey(summary); got != "<root@example.com>" {
		t.Fatalf("threadKey = %q, want root reference", got)
	}
}

func TestBodyMatcherLiteralCaseInsensitiveSnippet(t *testing.T) {
	matcher, err := newBodyMatcher("787", false, false)
	if err != nil {
		t.Fatalf("newBodyMatcher returned error: %v", err)
	}

	snippet, ok := matcher.snippet([]byte("Call me at 787-555-1212 about the plan."))
	if !ok {
		t.Fatal("expected body match")
	}
	if snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
}

func TestParseDateWindowRejectsEndBeforeStart(t *testing.T) {
	if _, _, err := parseDateWindow("2026-05-02", "2026-05-01"); err == nil {
		t.Fatal("expected endDate before startDate to fail")
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

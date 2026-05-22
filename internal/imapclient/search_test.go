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

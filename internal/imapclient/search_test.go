package imapclient

import "testing"

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

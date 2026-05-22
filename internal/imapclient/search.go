package imapclient

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/emersion/go-imap/v2"
	clientlib "github.com/emersion/go-imap/v2/imapclient"
)

const headerBatchSize = 200

type SearchQuery struct {
	Mailbox    string
	Subject    string
	From       string
	To         string
	StartDate  string
	Days       int
	MaxResults int
}

func (c *Client) SearchBySubject(q SearchQuery) ([]MessageSummary, error) {
	mailbox := c.mailbox(q.Mailbox)
	log.Printf("search_by_subject mailbox=%s criteria=imap_subject subject=%q startDate=%q", mailbox, q.Subject, q.StartDate)
	results, err := c.search(q, subjectCriteria(q.Subject))
	if err != nil || len(results) > 0 {
		if err == nil {
			log.Printf("search_by_subject mailbox=%s returned=%d fallback=false", mailbox, len(results))
		}
		return results, err
	}
	log.Printf("search_by_subject mailbox=%s imap_returned=0 fallback=local_header_scan", mailbox)
	return c.searchSubjectLocally(q)
}

func (c *Client) SearchFrom(q SearchQuery) ([]MessageSummary, error) {
	criteria := &imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "From", Value: q.From}},
	}
	return c.search(q, criteria)
}

func (c *Client) SearchTo(q SearchQuery) ([]MessageSummary, error) {
	criteria := &imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "To", Value: q.To}},
	}
	return c.search(q, criteria)
}

func (c *Client) SearchSince(q SearchQuery) ([]MessageSummary, error) {
	start, err := parseDate(q.StartDate)
	if err != nil {
		return nil, err
	}
	return c.scanHeaders(q, "search_since", fmt.Sprintf("local_since since=%s imapDate=%s", q.StartDate, imapDate(start)), func(summary MessageSummary) bool {
		return summaryOnOrAfter(summary, start)
	})
}

func (c *Client) SearchRecent(q SearchQuery) ([]MessageSummary, error) {
	if q.Days <= 0 {
		return nil, fmt.Errorf("days must be positive")
	}
	cutoff := time.Now().AddDate(0, 0, -q.Days)
	return c.scanHeaders(q, "search_recent", fmt.Sprintf("local_recent days=%d since=%s imapDate=%s", q.Days, cutoff.Format("2006-01-02"), imapDate(cutoff)), func(summary MessageSummary) bool {
		return summaryOnOrAfter(summary, cutoff)
	})
}

func (c *Client) SampleRecentHeaders(mailbox string, limit int) ([]MessageSummary, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > c.cfg.IMAP.MaxResults {
		limit = c.cfg.IMAP.MaxResults
	}
	q := SearchQuery{Mailbox: mailbox, MaxResults: limit}
	return c.scanHeaders(q, "sample_recent_headers", fmt.Sprintf("sample limit=%d", limit), func(MessageSummary) bool {
		return true
	})
}

func (c *Client) search(q SearchQuery, criteria *imap.SearchCriteria) ([]MessageSummary, error) {
	mailbox := c.mailbox(q.Mailbox)
	maxResults := c.maxResults(q.MaxResults)

	if err := applyStartDate(criteria, q.StartDate); err != nil {
		return nil, err
	}

	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	selectData, err := selectMailbox(ic, mailbox)
	if err != nil {
		return nil, err
	}
	log.Printf("search mailbox=%s message_count=%d criteria=%s", mailbox, selectData.NumMessages, criteriaDescription(criteria))
	if selectData.NumMessages == 0 {
		log.Printf("search mailbox=%s imap_search_uids=0 local_filtered=0", mailbox)
		return []MessageSummary{}, nil
	}
	ensureAllMessages(criteria)

	searchData, err := ic.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, classifyIMAPError(err)
	}
	uids := searchData.AllUIDs()
	log.Printf("search mailbox=%s imap_search_uids=%d", mailbox, len(uids))
	if len(uids) == 0 {
		log.Printf("search mailbox=%s local_filtered=0", mailbox)
		return []MessageSummary{}, nil
	}

	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	if len(uids) > maxResults {
		uids = uids[:maxResults]
	}

	results, err := fetchSummaries(ic, mailbox, uids)
	if err != nil {
		return nil, err
	}
	log.Printf("search mailbox=%s local_filtered=%d", mailbox, len(results))
	return results, nil
}

func (c *Client) searchSubjectLocally(q SearchQuery) ([]MessageSummary, error) {
	var start time.Time
	if q.StartDate != "" {
		parsed, err := parseDate(q.StartDate)
		if err != nil {
			return nil, err
		}
		start = parsed
	}
	return c.scanHeaders(q, "search_by_subject", fmt.Sprintf("local_subject subject=%q startDate=%q", q.Subject, q.StartDate), func(summary MessageSummary) bool {
		if !start.IsZero() && !summaryOnOrAfter(summary, start) {
			return false
		}
		return subjectMatches(q.Subject, summary.Subject)
	})
}

func (c *Client) scanHeaders(q SearchQuery, op string, criteria string, match func(MessageSummary) bool) ([]MessageSummary, error) {
	mailbox := c.mailbox(q.Mailbox)
	maxResults := c.maxResults(q.MaxResults)

	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	selectData, err := selectMailbox(ic, mailbox)
	if err != nil {
		return nil, err
	}
	log.Printf("%s mailbox=%s message_count=%d criteria=%s", op, mailbox, selectData.NumMessages, criteria)

	if selectData.NumMessages == 0 {
		log.Printf("%s mailbox=%s imap_search_uids=0 local_scanned=0 local_filtered=0", op, mailbox)
		return []MessageSummary{}, nil
	}

	matches := make([]MessageSummary, 0, maxResults)
	scanned := 0
	for endSeq := selectData.NumMessages; endSeq > 0 && len(matches) < maxResults; {
		startSeq := uint32(1)
		if endSeq > headerBatchSize {
			startSeq = endSeq - headerBatchSize + 1
		}
		summaries, err := fetchSummariesBySeq(ic, mailbox, startSeq, endSeq)
		if err != nil {
			return nil, err
		}
		scanned += len(summaries)
		for _, summary := range summaries {
			if match(summary) {
				matches = append(matches, summary)
				if len(matches) == maxResults {
					break
				}
			}
		}
		if startSeq == 1 {
			break
		}
		endSeq = startSeq - 1
	}

	log.Printf("%s mailbox=%s imap_search_uids=0 local_scanned=%d local_filtered=%d", op, mailbox, scanned, len(matches))
	return matches, nil
}

func fetchSummaries(c *clientlib.Client, mailbox string, uids []imap.UID) ([]MessageSummary, error) {
	set := imap.UIDSetNum(uids...)
	options := &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		InternalDate: true,
		RFC822Size:   true,
	}
	messages, err := c.Fetch(set, options).Collect()
	if err != nil {
		return nil, classifyIMAPError(err)
	}

	byUID := make(map[imap.UID]MessageSummary, len(messages))
	for _, msg := range messages {
		byUID[msg.UID] = messageSummary(mailbox, msg)
	}

	out := make([]MessageSummary, 0, len(uids))
	for _, uid := range uids {
		if summary, ok := byUID[uid]; ok {
			out = append(out, summary)
		}
	}
	return out, nil
}

func fetchSummariesBySeq(c *clientlib.Client, mailbox string, startSeq, endSeq uint32) ([]MessageSummary, error) {
	var set imap.SeqSet
	set.AddRange(startSeq, endSeq)
	options := &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		InternalDate: true,
		RFC822Size:   true,
	}
	messages, err := c.Fetch(set, options).Collect()
	if err != nil {
		return nil, classifyIMAPError(err)
	}

	out := make([]MessageSummary, 0, len(messages))
	for _, msg := range messages {
		out = append(out, messageSummary(mailbox, msg))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeqNum > out[j].SeqNum })
	return out, nil
}

func messageSummary(mailbox string, msg *clientlib.FetchMessageBuffer) MessageSummary {
	summary := MessageSummary{
		UID:     uint32(msg.UID),
		Mailbox: mailbox,
		Size:    msg.RFC822Size,
		SeqNum:  msg.SeqNum,
	}
	if !msg.InternalDate.IsZero() {
		summary.InternalDate = msg.InternalDate.Format(time.RFC3339)
		summary.Date = summary.InternalDate
	}
	if msg.Envelope != nil {
		summary.Subject = msg.Envelope.Subject
		if !msg.Envelope.Date.IsZero() {
			summary.Date = msg.Envelope.Date.Format(time.RFC3339)
		}
		summary.From = formatAddresses(msg.Envelope.From)
		summary.To = formatAddresses(msg.Envelope.To)
	}
	return summary
}

func (c *Client) mailbox(mailbox string) string {
	if mailbox != "" {
		return mailbox
	}
	return c.cfg.IMAP.DefaultMailbox
}

func (c *Client) maxResults(maxResults int) int {
	if maxResults <= 0 || maxResults > c.cfg.IMAP.MaxResults {
		return c.cfg.IMAP.MaxResults
	}
	return maxResults
}

func parseDate(value string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD")
	}
	return t, nil
}

func applyStartDate(criteria *imap.SearchCriteria, value string) error {
	if value == "" {
		return nil
	}
	start, err := parseDate(value)
	if err != nil {
		return err
	}
	criteria.Since = start
	return nil
}

func ensureAllMessages(criteria *imap.SearchCriteria) {
	if len(criteria.SeqNum) > 0 || len(criteria.UID) > 0 {
		return
	}
	var all imap.SeqSet
	all.AddRange(1, 0)
	criteria.SeqNum = []imap.SeqSet{all}
}

func criteriaDescription(criteria *imap.SearchCriteria) string {
	parts := make([]string, 0, 4)
	if !criteria.Since.IsZero() {
		parts = append(parts, "SINCE "+imapDate(criteria.Since))
	}
	for _, header := range criteria.Header {
		parts = append(parts, fmt.Sprintf("HEADER %s %q", header.Key, header.Value))
	}
	if len(criteria.SeqNum) > 0 {
		sets := make([]string, 0, len(criteria.SeqNum))
		for _, set := range criteria.SeqNum {
			sets = append(sets, set.String())
		}
		parts = append(parts, "SEQ "+strings.Join(sets, ","))
	}
	if len(parts) == 0 {
		return "ALL"
	}
	return strings.Join(parts, " ")
}

func imapDate(t time.Time) string {
	return t.Format("02-Jan-2006")
}

func summaryOnOrAfter(summary MessageSummary, cutoff time.Time) bool {
	for _, value := range []string{summary.InternalDate, summary.Date} {
		if value == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, value)
		if err == nil && !t.Before(cutoff) {
			return true
		}
	}
	return false
}

func subjectCriteria(subject string) *imap.SearchCriteria {
	tokens := subjectTokens(subject)
	if len(tokens) == 0 {
		return &imap.SearchCriteria{
			Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: subject}},
		}
	}

	headers := make([]imap.SearchCriteriaHeaderField, 0, len(tokens))
	for _, token := range tokens {
		headers = append(headers, imap.SearchCriteriaHeaderField{Key: "Subject", Value: token})
	}
	return &imap.SearchCriteria{Header: headers}
}

func subjectMatches(query, subject string) bool {
	normalizedQuery := normalizeSubject(query)
	normalizedSubject := normalizeSubject(subject)
	if normalizedQuery == "" {
		return false
	}
	if strings.Contains(normalizedSubject, normalizedQuery) {
		return true
	}
	tokens := subjectTokens(query)
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		if !strings.Contains(normalizedSubject, strings.ToLower(token)) {
			return false
		}
	}
	return true
}

func normalizeSubject(subject string) string {
	parts := strings.FieldsFunc(subject, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.ToLower(strings.Join(parts, " "))
}

func subjectTokens(subject string) []string {
	stopWords := map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "for": {}, "fwd": {}, "in": {}, "of": {}, "on": {}, "or": {}, "re": {}, "the": {}, "to": {},
	}
	seen := make(map[string]struct{})
	parts := strings.FieldsFunc(subject, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		key := strings.ToLower(token)
		if len(key) < 2 {
			continue
		}
		if _, skip := stopWords[key]; skip {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tokens = append(tokens, token)
	}
	return tokens
}

func formatAddresses(addrs []imap.Address) []string {
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		mailbox := addr.Mailbox
		host := addr.Host
		if mailbox == "" && host == "" {
			continue
		}
		email := mailbox
		if host != "" {
			email = mailbox + "@" + host
		}
		if addr.Name != "" {
			out = append(out, fmt.Sprintf("%s <%s>", addr.Name, email))
		} else {
			out = append(out, email)
		}
	}
	return out
}

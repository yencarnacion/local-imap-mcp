package imapclient

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/emersion/go-imap/v2"
	clientlib "github.com/emersion/go-imap/v2/imapclient"
)

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
	results, err := c.search(q, subjectCriteria(q.Subject))
	if err != nil || len(results) > 0 {
		return results, err
	}
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
	criteria := &imap.SearchCriteria{}
	return c.search(q, criteria)
}

func (c *Client) SearchRecent(q SearchQuery) ([]MessageSummary, error) {
	if q.Days <= 0 {
		return nil, fmt.Errorf("days must be positive")
	}
	criteria := &imap.SearchCriteria{
		Since: time.Now().AddDate(0, 0, -q.Days),
	}
	return c.search(q, criteria)
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
	if selectData.NumMessages == 0 {
		return []MessageSummary{}, nil
	}
	ensureAllMessages(criteria)

	searchData, err := ic.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, classifyIMAPError(err)
	}
	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return []MessageSummary{}, nil
	}

	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	if len(uids) > maxResults {
		uids = uids[:maxResults]
	}

	return fetchSummaries(ic, mailbox, uids)
}

func (c *Client) searchSubjectLocally(q SearchQuery) ([]MessageSummary, error) {
	mailbox := c.mailbox(q.Mailbox)
	maxResults := c.maxResults(q.MaxResults)
	criteria := &imap.SearchCriteria{}
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
	if selectData.NumMessages == 0 {
		return []MessageSummary{}, nil
	}
	ensureAllMessages(criteria)

	searchData, err := ic.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, classifyIMAPError(err)
	}
	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return []MessageSummary{}, nil
	}

	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })

	const chunkSize = 100
	matches := make([]MessageSummary, 0, maxResults)
	for start := 0; start < len(uids) && len(matches) < maxResults; start += chunkSize {
		end := start + chunkSize
		if end > len(uids) {
			end = len(uids)
		}
		summaries, err := fetchSummaries(ic, mailbox, uids[start:end])
		if err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			if subjectMatches(q.Subject, summary.Subject) {
				matches = append(matches, summary)
				if len(matches) == maxResults {
					break
				}
			}
		}
	}

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
		summary := MessageSummary{
			UID:     uint32(msg.UID),
			Mailbox: mailbox,
			Size:    msg.RFC822Size,
			SeqNum:  msg.SeqNum,
		}
		if !msg.InternalDate.IsZero() {
			summary.Date = msg.InternalDate.Format(time.RFC3339)
		}
		if msg.Envelope != nil {
			summary.Subject = msg.Envelope.Subject
			if !msg.Envelope.Date.IsZero() {
				summary.Date = msg.Envelope.Date.Format(time.RFC3339)
			}
			summary.From = formatAddresses(msg.Envelope.From)
			summary.To = formatAddresses(msg.Envelope.To)
		}
		byUID[msg.UID] = summary
	}

	out := make([]MessageSummary, 0, len(uids))
	for _, uid := range uids {
		if summary, ok := byUID[uid]; ok {
			out = append(out, summary)
		}
	}
	return out, nil
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

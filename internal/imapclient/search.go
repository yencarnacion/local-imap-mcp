package imapclient

import (
	"fmt"
	"sort"
	"time"

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
	criteria := &imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: q.Subject}},
	}
	return c.search(q, criteria)
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

	if q.StartDate != "" {
		start, err := parseDate(q.StartDate)
		if err != nil {
			return nil, err
		}
		criteria.Since = start
	}

	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	if err := selectMailbox(ic, mailbox); err != nil {
		return nil, err
	}

	searchData, err := ic.UIDSearch(criteria, &imap.SearchOptions{ReturnAll: true}).Wait()
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

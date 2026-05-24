package imapclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/emersion/go-imap/v2"
	clientlib "github.com/emersion/go-imap/v2/imapclient"

	"local-imap-mcp/internal/emailparse"
)

const headerBatchSize = 200
const defaultHeaderScanLimit = 200
const defaultUIDWindow = 1000

type SearchQuery struct {
	Mailbox    string
	Subject    string
	From       string
	To         string
	StartDate  string
	Days       int
	MaxResults int
}

type SearchResult struct {
	Mailbox         string           `json:"mailbox"`
	Criteria        string           `json:"criteria"`
	MaxResults      int              `json:"maxResults"`
	Returned        int              `json:"returned"`
	CandidateUIDs   int              `json:"candidateUIDs,omitempty"`
	ScannedMessages int              `json:"scannedMessages,omitempty"`
	HasMore         bool             `json:"hasMore"`
	Truncated       bool             `json:"truncated"`
	Warnings        []string         `json:"warnings,omitempty"`
	Results         []MessageSummary `json:"results"`
}

type HeaderScanQuery struct {
	Mailbox             string
	StartDate           string
	BeforeUID           uint32
	AfterUID            uint32
	Cursor              string
	Limit               int
	UIDWindow           int
	From                string
	To                  string
	SenderDomain        string
	UnreadOnly          bool
	HasReplyHeaders     bool
	StopAtDateThreshold bool
}

type HeaderScanResult struct {
	Mailbox         string           `json:"mailbox"`
	UIDValidity     uint32           `json:"uidValidity"`
	UIDNext         uint32           `json:"uidNext"`
	Exists          uint32           `json:"exists"`
	StartDate       string           `json:"startDate,omitempty"`
	BeforeUID       uint32           `json:"beforeUID,omitempty"`
	AfterUID        uint32           `json:"afterUID,omitempty"`
	NextBeforeUID   uint32           `json:"nextBeforeUID,omitempty"`
	Cursor          string           `json:"cursor,omitempty"`
	Limit           int              `json:"limit"`
	UIDWindow       int              `json:"uidWindow"`
	ScannedUIDHigh  uint32           `json:"scannedUIDHigh,omitempty"`
	ScannedUIDLow   uint32           `json:"scannedUIDLow,omitempty"`
	ScannedMessages int              `json:"scannedMessages"`
	Returned        int              `json:"returned"`
	HasMore         bool             `json:"hasMore"`
	Complete        bool             `json:"complete"`
	Truncated       bool             `json:"truncated"`
	StopReason      string           `json:"stopReason"`
	Warnings        []string         `json:"warnings,omitempty"`
	Headers         []MessageSummary `json:"headers"`
}

type scanCursor struct {
	Mailbox     string `json:"mailbox"`
	UIDValidity uint32 `json:"uidValidity"`
	BeforeUID   uint32 `json:"beforeUID"`
	AfterUID    uint32 `json:"afterUID,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
}

func (c *Client) SearchBySubject(q SearchQuery) (*SearchResult, error) {
	mailbox := c.mailbox(q.Mailbox)
	log.Printf("search_by_subject mailbox=%s criteria=imap_subject subject=%q startDate=%q", mailbox, q.Subject, q.StartDate)
	results, err := c.search(q, subjectCriteria(q.Subject))
	if err != nil || results.Returned > 0 {
		if err == nil {
			log.Printf("search_by_subject mailbox=%s returned=%d fallback=false", mailbox, results.Returned)
		}
		return results, err
	}
	log.Printf("search_by_subject mailbox=%s imap_returned=0 fallback=local_header_scan", mailbox)
	return c.searchSubjectLocally(q)
}

func (c *Client) SearchFrom(q SearchQuery) (*SearchResult, error) {
	criteria := &imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "From", Value: q.From}},
	}
	return c.search(q, criteria)
}

func (c *Client) SearchTo(q SearchQuery) (*SearchResult, error) {
	criteria := &imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "To", Value: q.To}},
	}
	return c.search(q, criteria)
}

func (c *Client) SearchSince(q SearchQuery) (*SearchResult, error) {
	start, err := parseDate(q.StartDate)
	if err != nil {
		return nil, err
	}
	return c.scanHeaders(q, "search_since", fmt.Sprintf("local_since since=%s imapDate=%s", q.StartDate, imapDate(start)), func(summary MessageSummary) bool {
		return summaryOnOrAfter(summary, start)
	})
}

func (c *Client) ScanHeadersRange(q HeaderScanQuery) (*HeaderScanResult, error) {
	mailbox := c.mailbox(q.Mailbox)
	if q.Cursor != "" {
		cursor, err := decodeScanCursor(q.Cursor)
		if err != nil {
			return nil, err
		}
		if cursor.Mailbox != "" {
			mailbox = cursor.Mailbox
		}
		q.BeforeUID = cursor.BeforeUID
		q.AfterUID = cursor.AfterUID
		if q.StartDate == "" {
			q.StartDate = cursor.StartDate
		}
	}

	limit := q.Limit
	if limit <= 0 {
		limit = defaultHeaderScanLimit
	}
	uidWindow := q.UIDWindow
	if uidWindow <= 0 {
		uidWindow = defaultUIDWindow
	}

	var start time.Time
	if q.StartDate != "" {
		parsed, err := parseDate(q.StartDate)
		if err != nil {
			return nil, err
		}
		start = parsed
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

	result := &HeaderScanResult{
		Mailbox:     mailbox,
		UIDValidity: selectData.UIDValidity,
		UIDNext:     uint32(selectData.UIDNext),
		Exists:      selectData.NumMessages,
		StartDate:   q.StartDate,
		BeforeUID:   q.BeforeUID,
		AfterUID:    q.AfterUID,
		Limit:       limit,
		UIDWindow:   uidWindow,
		Headers:     []MessageSummary{},
	}
	if q.Cursor != "" {
		cursor, _ := decodeScanCursor(q.Cursor)
		if cursor.UIDValidity != 0 && cursor.UIDValidity != result.UIDValidity {
			return nil, fmt.Errorf("cursor uidValidity %d no longer matches mailbox uidValidity %d", cursor.UIDValidity, result.UIDValidity)
		}
	}
	if selectData.NumMessages == 0 || selectData.UIDNext <= 1 {
		result.Complete = true
		result.StopReason = "empty_mailbox"
		return result, nil
	}

	high := uint32(selectData.UIDNext) - 1
	if q.BeforeUID > 0 && q.BeforeUID <= high+1 {
		high = q.BeforeUID - 1
	}
	if high <= q.AfterUID {
		result.Complete = true
		result.StopReason = "uid_range_exhausted"
		return result, nil
	}
	result.ScannedUIDHigh = high

	currentHigh := high
	remainingUIDs := uidWindow
	lowestProcessed := uint32(0)
	stopReason := "uid_range_exhausted"

scanLoop:
	for currentHigh > q.AfterUID && remainingUIDs > 0 {
		span := headerBatchSize
		if remainingUIDs < span {
			span = remainingUIDs
		}
		batchLow := currentHigh - uint32(span) + 1
		if batchLow <= q.AfterUID {
			batchLow = q.AfterUID + 1
		}
		actualSpan := int(currentHigh - batchLow + 1)
		summaries, err := fetchHeaderSummariesByUIDRange(ic, mailbox, imap.UID(batchLow), imap.UID(currentHigh))
		if err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			lowestProcessed = summary.UID
			result.ScannedMessages++
			if q.StopAtDateThreshold && !start.IsZero() && !summaryOnOrAfter(summary, start) {
				stopReason = "stopped_at_date_threshold"
				break scanLoop
			}
			if headerMatches(summary, q, start) {
				result.Headers = append(result.Headers, summary)
				if len(result.Headers) >= limit {
					stopReason = "limit_reached"
					break scanLoop
				}
			}
		}
		if lowestProcessed == 0 || lowestProcessed > batchLow {
			lowestProcessed = batchLow
		}
		currentHigh = batchLow - 1
		remainingUIDs -= actualSpan
	}

	if lowestProcessed == 0 {
		lowestProcessed = currentHigh
	}
	if stopReason == "uid_range_exhausted" && currentHigh > q.AfterUID && remainingUIDs == 0 {
		stopReason = "uid_window_exhausted"
	}
	result.ScannedUIDLow = lowestProcessed
	result.Returned = len(result.Headers)
	result.StopReason = stopReason

	switch stopReason {
	case "limit_reached", "stopped_at_date_threshold", "uid_window_exhausted":
		result.NextBeforeUID = lowestProcessed
	case "uid_range_exhausted":
		result.NextBeforeUID = currentHigh + 1
	default:
		result.NextBeforeUID = lowestProcessed
	}

	if stopReason == "stopped_at_date_threshold" {
		result.Complete = true
		result.Warnings = append(result.Warnings, "scan stopped early at date threshold; use without stopAtDateThreshold for exhaustive UID traversal")
	} else if result.NextBeforeUID > q.AfterUID+1 {
		result.HasMore = true
		result.Truncated = true
		result.StopReason = stopReason
	} else {
		result.Complete = true
		result.StopReason = "uid_range_exhausted"
	}

	if result.HasMore {
		cursor := scanCursor{
			Mailbox:     mailbox,
			UIDValidity: result.UIDValidity,
			BeforeUID:   result.NextBeforeUID,
			AfterUID:    q.AfterUID,
			StartDate:   q.StartDate,
		}
		result.Cursor = encodeScanCursor(cursor)
		result.Warnings = append(result.Warnings, "result is a page, not a complete audit; resume with cursor or nextBeforeUID")
	}

	log.Printf("scan_headers_range mailbox=%s uid_high=%d uid_low=%d scanned_messages=%d returned=%d has_more=%t",
		mailbox, result.ScannedUIDHigh, result.ScannedUIDLow, result.ScannedMessages, result.Returned, result.HasMore)
	return result, nil
}

func (c *Client) SearchRecent(q SearchQuery) (*SearchResult, error) {
	if q.Days <= 0 {
		return nil, fmt.Errorf("days must be positive")
	}
	cutoff := time.Now().AddDate(0, 0, -q.Days)
	return c.scanHeaders(q, "search_recent", fmt.Sprintf("local_recent days=%d since=%s imapDate=%s", q.Days, cutoff.Format("2006-01-02"), imapDate(cutoff)), func(summary MessageSummary) bool {
		return summaryOnOrAfter(summary, cutoff)
	})
}

func (c *Client) SampleRecentHeaders(mailbox string, limit int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	q := SearchQuery{Mailbox: mailbox, MaxResults: limit}
	return c.scanHeaders(q, "sample_recent_headers", fmt.Sprintf("sample limit=%d", limit), func(MessageSummary) bool {
		return true
	})
}

func (c *Client) search(q SearchQuery, criteria *imap.SearchCriteria) (*SearchResult, error) {
	mailbox := c.mailbox(q.Mailbox)
	maxResults := c.maxResults(q.MaxResults)
	description := criteriaDescription(criteria)
	result := &SearchResult{
		Mailbox:    mailbox,
		Criteria:   description,
		MaxResults: maxResults,
		Results:    []MessageSummary{},
	}

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
	description = criteriaDescription(criteria)
	result.Criteria = description
	log.Printf("search mailbox=%s message_count=%d criteria=%s", mailbox, selectData.NumMessages, description)
	if selectData.NumMessages == 0 {
		log.Printf("search mailbox=%s imap_search_uids=0 local_filtered=0", mailbox)
		return result, nil
	}
	ensureAllMessages(criteria)

	searchData, err := ic.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, classifyIMAPError(err)
	}
	uids := searchData.AllUIDs()
	log.Printf("search mailbox=%s imap_search_uids=%d", mailbox, len(uids))
	result.CandidateUIDs = len(uids)
	if len(uids) == 0 {
		log.Printf("search mailbox=%s local_filtered=0", mailbox)
		return result, nil
	}

	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	if len(uids) > maxResults {
		result.HasMore = true
		result.Truncated = true
		result.Warnings = append(result.Warnings, fmt.Sprintf("search matched %d UIDs; returned newest %d only", len(uids), maxResults))
		uids = uids[:maxResults]
	}

	summaries, err := fetchSummaries(ic, mailbox, uids)
	if err != nil {
		return nil, err
	}
	result.Results = summaries
	result.Returned = len(summaries)
	log.Printf("search mailbox=%s local_filtered=%d", mailbox, result.Returned)
	return result, nil
}

func (c *Client) searchSubjectLocally(q SearchQuery) (*SearchResult, error) {
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

func (c *Client) scanHeaders(q SearchQuery, op string, criteria string, match func(MessageSummary) bool) (*SearchResult, error) {
	mailbox := c.mailbox(q.Mailbox)
	maxResults := c.maxResults(q.MaxResults)
	result := &SearchResult{
		Mailbox:    mailbox,
		Criteria:   criteria,
		MaxResults: maxResults,
		Results:    []MessageSummary{},
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
	log.Printf("%s mailbox=%s message_count=%d criteria=%s", op, mailbox, selectData.NumMessages, criteria)

	if selectData.NumMessages == 0 {
		log.Printf("%s mailbox=%s imap_search_uids=0 local_scanned=0 local_filtered=0", op, mailbox)
		return result, nil
	}

	matches := make([]MessageSummary, 0, maxResults)
	scanned := 0
	lastStartSeq := uint32(0)
	for endSeq := selectData.NumMessages; endSeq > 0 && len(matches) < maxResults; {
		startSeq := uint32(1)
		if endSeq > headerBatchSize {
			startSeq = endSeq - headerBatchSize + 1
		}
		lastStartSeq = startSeq
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

	result.Results = matches
	result.Returned = len(matches)
	result.ScannedMessages = scanned
	if len(matches) == maxResults && lastStartSeq > 1 {
		result.HasMore = true
		result.Truncated = true
		result.Warnings = append(result.Warnings, fmt.Sprintf("header scan stopped at maxResults=%d before reaching the oldest message", maxResults))
	}
	log.Printf("%s mailbox=%s imap_search_uids=0 local_scanned=%d local_filtered=%d", op, mailbox, scanned, len(matches))
	return result, nil
}

func fetchSummaries(c *clientlib.Client, mailbox string, uids []imap.UID) ([]MessageSummary, error) {
	set := imap.UIDSetNum(uids...)
	options := &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		Flags:        true,
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
		Flags:        true,
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

func fetchHeaderSummariesByUIDRange(c *clientlib.Client, mailbox string, startUID, endUID imap.UID) ([]MessageSummary, error) {
	var set imap.UIDSet
	set.AddRange(startUID, endUID)
	headerSection := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"Message-ID", "In-Reply-To", "References"},
		Peek:         true,
	}
	options := &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
		RFC822Size:   true,
		BodySection:  []*imap.FetchItemBodySection{headerSection},
	}
	messages, err := c.Fetch(set, options).Collect()
	if err != nil {
		return nil, classifyIMAPError(err)
	}

	out := make([]MessageSummary, 0, len(messages))
	for _, msg := range messages {
		summary := messageSummary(mailbox, msg)
		if raw := msg.FindBodySection(headerSection); raw != nil {
			applyThreadHeaders(&summary, raw)
		}
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID > out[j].UID })
	return out, nil
}

func messageSummary(mailbox string, msg *clientlib.FetchMessageBuffer) MessageSummary {
	summary := MessageSummary{
		UID:     uint32(msg.UID),
		Mailbox: mailbox,
		Size:    msg.RFC822Size,
		SeqNum:  msg.SeqNum,
		Flags:   flagsToStrings(msg.Flags),
	}
	if !msg.InternalDate.IsZero() {
		summary.InternalDate = msg.InternalDate.Format(time.RFC3339)
		summary.Date = summary.InternalDate
	}
	if msg.Envelope != nil {
		summary.Subject = msg.Envelope.Subject
		summary.MessageID = msg.Envelope.MessageID
		if len(msg.Envelope.InReplyTo) > 0 {
			summary.InReplyTo = msg.Envelope.InReplyTo[0]
		}
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
	if maxResults <= 0 {
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

func headerMatches(summary MessageSummary, q HeaderScanQuery, start time.Time) bool {
	if !start.IsZero() && !summaryOnOrAfter(summary, start) {
		return false
	}
	if q.From != "" && !addressesContain(summary.From, q.From) {
		return false
	}
	if q.To != "" && !addressesContain(summary.To, q.To) {
		return false
	}
	if q.SenderDomain != "" && !addressesContainDomain(summary.From, q.SenderDomain) {
		return false
	}
	if q.UnreadOnly && hasFlag(summary.Flags, string(imap.FlagSeen)) {
		return false
	}
	if q.HasReplyHeaders && summary.InReplyTo == "" && len(summary.References) == 0 {
		return false
	}
	return true
}

func addressesContain(addresses []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return true
	}
	for _, address := range addresses {
		if strings.Contains(strings.ToLower(address), needle) {
			return true
		}
	}
	return false
}

func addressesContainDomain(addresses []string, domain string) bool {
	domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
	if domain == "" {
		return true
	}
	for _, address := range addresses {
		value := strings.ToLower(address)
		if strings.Contains(value, "@"+domain) || strings.Contains(value, "."+domain) {
			return true
		}
	}
	return false
}

func hasFlag(flags []string, flag string) bool {
	for _, value := range flags {
		if strings.EqualFold(value, flag) {
			return true
		}
	}
	return false
}

func applyThreadHeaders(summary *MessageSummary, raw []byte) {
	headers, err := emailparse.Headers(raw)
	if err != nil {
		return
	}
	if values := headerValues(headers, "Message-ID"); len(values) > 0 && summary.MessageID == "" {
		summary.MessageID = strings.TrimSpace(values[0])
	}
	if values := headerValues(headers, "In-Reply-To"); len(values) > 0 && summary.InReplyTo == "" {
		summary.InReplyTo = strings.TrimSpace(values[0])
	}
	if values := headerValues(headers, "References"); len(values) > 0 {
		summary.References = splitReferences(values)
	}
}

func headerValues(headers map[string][]string, key string) []string {
	for candidate, values := range headers {
		if strings.EqualFold(candidate, key) {
			return values
		}
	}
	return nil
}

func splitReferences(values []string) []string {
	out := make([]string, 0)
	for _, value := range values {
		for _, part := range strings.Fields(value) {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func encodeScanCursor(cursor scanCursor) string {
	data, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeScanCursor(value string) (scanCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return scanCursor{}, fmt.Errorf("invalid cursor: %w", err)
	}
	var cursor scanCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return scanCursor{}, fmt.Errorf("invalid cursor: %w", err)
	}
	return cursor, nil
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

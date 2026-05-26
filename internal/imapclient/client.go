package imapclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"strings"

	"github.com/emersion/go-imap/v2"
	clientlib "github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"

	"local-imap-mcp/internal/config"
	"local-imap-mcp/internal/emailparse"
)

var (
	ErrAuthFailed      = errors.New("IMAP authentication failed")
	ErrMailboxNotFound = errors.New("mailbox not found")
	ErrMessageNotFound = errors.New("message not found")
)

type Client struct {
	cfg *config.Config
}

type Mailbox struct {
	Name      string   `json:"name"`
	Delimiter string   `json:"delimiter,omitempty"`
	Attrs     []string `json:"attrs,omitempty"`
}

type MailboxCount struct {
	Mailbox     string `json:"mailbox"`
	Exists      uint32 `json:"exists"`
	Messages    uint32 `json:"messages"`
	Recent      uint32 `json:"recent"`
	UIDNext     uint32 `json:"uidNext"`
	UIDValidity uint32 `json:"uidValidity"`
}

type MailboxDiagnostics struct {
	Mailbox          string   `json:"mailbox"`
	Listed           bool     `json:"listed"`
	Delimiter        string   `json:"delimiter,omitempty"`
	Attrs            []string `json:"attrs,omitempty"`
	StatusOK         bool     `json:"statusOK"`
	StatusError      string   `json:"statusError,omitempty"`
	SelectOK         bool     `json:"selectOK"`
	SelectError      string   `json:"selectError,omitempty"`
	Exists           uint32   `json:"exists"`
	Messages         uint32   `json:"messages"`
	Recent           uint32   `json:"recent"`
	Unseen           *uint32  `json:"unseen,omitempty"`
	UIDNext          uint32   `json:"uidNext"`
	UIDValidity      uint32   `json:"uidValidity"`
	HighestModSeq    uint64   `json:"highestModSeq,omitempty"`
	Size             *int64   `json:"size,omitempty"`
	Flags            []string `json:"flags,omitempty"`
	PermanentFlags   []string `json:"permanentFlags,omitempty"`
	FetchByUIDOK     bool     `json:"fetchByUIDOK"`
	FetchByUID       uint32   `json:"fetchByUID,omitempty"`
	FetchByUIDError  string   `json:"fetchByUIDError,omitempty"`
	Empty            bool     `json:"empty"`
	UsableForTriage  bool     `json:"usableForTriage"`
	Healthy          bool     `json:"healthy"`
	DiagnosticStatus string   `json:"diagnosticStatus"`
	Warnings         []string `json:"warnings,omitempty"`
}

type MailboxSyncHealth struct {
	Mailbox            string   `json:"mailbox"`
	Exists             uint32   `json:"exists"`
	Messages           uint32   `json:"messages"`
	UIDNext            uint32   `json:"uidNext"`
	UIDValidity        uint32   `json:"uidValidity"`
	LatestUID          uint32   `json:"latestUID,omitempty"`
	LatestDate         string   `json:"latestDate,omitempty"`
	LatestInternalDate string   `json:"latestInternalDate,omitempty"`
	TargetMessages     int      `json:"targetMessages,omitempty"`
	PercentOfTarget    float64  `json:"percentOfTarget,omitempty"`
	Healthy            bool     `json:"healthy"`
	UsableForTriage    bool     `json:"usableForTriage"`
	DiagnosticStatus   string   `json:"diagnosticStatus"`
	Warnings           []string `json:"warnings,omitempty"`
}

type MessageSummary struct {
	UID          uint32   `json:"uid"`
	Mailbox      string   `json:"mailbox"`
	Subject      string   `json:"subject"`
	From         []string `json:"from"`
	To           []string `json:"to"`
	Date         string   `json:"date"`
	InternalDate string   `json:"internalDate,omitempty"`
	Size         int64    `json:"size,omitempty"`
	SeqNum       uint32   `json:"seqNum,omitempty"`
	Flags        []string `json:"flags,omitempty"`
	MessageID    string   `json:"messageId,omitempty"`
	InReplyTo    string   `json:"inReplyTo,omitempty"`
	References   []string `json:"references,omitempty"`
}

type fetchedBody struct {
	Body       []byte
	RFC822Size int64
}

func New(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) ListMailboxes() ([]Mailbox, error) {
	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	data, err := ic.List("", "*", nil).Collect()
	if err != nil {
		return nil, classifyIMAPError(err)
	}
	mailboxes := make([]Mailbox, 0, len(data))
	for _, item := range data {
		attrs := make([]string, 0, len(item.Attrs))
		for _, attr := range item.Attrs {
			attrs = append(attrs, string(attr))
		}
		delimiter := ""
		if item.Delim != 0 {
			delimiter = string(item.Delim)
		}
		mailboxes = append(mailboxes, Mailbox{
			Name:      item.Mailbox,
			Delimiter: delimiter,
			Attrs:     attrs,
		})
	}
	return mailboxes, nil
}

func (c *Client) CountMessages(mailbox string) (*MailboxCount, error) {
	mailbox = c.mailbox(mailbox)

	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	data, err := selectMailbox(ic, mailbox)
	if err != nil {
		return nil, err
	}

	return &MailboxCount{
		Mailbox:     mailbox,
		Exists:      data.NumMessages,
		Messages:    data.NumMessages,
		Recent:      data.NumRecent,
		UIDNext:     uint32(data.UIDNext),
		UIDValidity: data.UIDValidity,
	}, nil
}

func (c *Client) MailboxDiagnostics(mailbox string) (*MailboxDiagnostics, error) {
	mailbox = c.mailbox(mailbox)
	diag := &MailboxDiagnostics{Mailbox: mailbox}

	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	if listData, err := ic.List("", "*", nil).Collect(); err == nil {
		for _, item := range listData {
			if item.Mailbox != mailbox {
				continue
			}
			diag.Listed = true
			if item.Delim != 0 {
				diag.Delimiter = string(item.Delim)
			}
			for _, attr := range item.Attrs {
				diag.Attrs = append(diag.Attrs, string(attr))
			}
			break
		}
	} else {
		diag.Warnings = append(diag.Warnings, "LIST failed: "+classifyIMAPError(err).Error())
	}
	if !diag.Listed {
		diag.Warnings = append(diag.Warnings, "mailbox was not returned by LIST")
	}

	status, err := ic.Status(mailbox, &imap.StatusOptions{
		NumMessages: true,
		UIDNext:     true,
		UIDValidity: true,
		NumUnseen:   true,
	}).Wait()
	if err != nil {
		diag.StatusError = classifyIMAPError(err).Error()
		diag.Warnings = append(diag.Warnings, "STATUS failed: "+diag.StatusError)
	} else {
		diag.StatusOK = true
		if status.NumMessages != nil {
			diag.Messages = *status.NumMessages
		}
		diag.Unseen = status.NumUnseen
		diag.UIDNext = uint32(status.UIDNext)
		diag.UIDValidity = status.UIDValidity
		diag.Size = status.Size
	}

	selectData, err := selectMailbox(ic, mailbox)
	if err != nil {
		diag.SelectError = classifyIMAPError(err).Error()
		diag.Warnings = append(diag.Warnings, "SELECT failed: "+diag.SelectError)
		finalizeDiagnostics(diag)
		return diag, nil
	}
	diag.SelectOK = true
	diag.Exists = selectData.NumMessages
	diag.Messages = selectData.NumMessages
	diag.Recent = selectData.NumRecent
	diag.UIDNext = uint32(selectData.UIDNext)
	diag.UIDValidity = selectData.UIDValidity
	diag.HighestModSeq = selectData.HighestModSeq
	diag.Flags = flagsToStrings(selectData.Flags)
	diag.PermanentFlags = flagsToStrings(selectData.PermanentFlags)
	diag.Empty = selectData.NumMessages == 0

	if selectData.NumMessages == 0 {
		diag.Warnings = append(diag.Warnings, "mailbox is selectable but empty")
		finalizeDiagnostics(diag)
		return diag, nil
	}

	sample, err := fetchSummariesBySeq(ic, mailbox, selectData.NumMessages, selectData.NumMessages)
	if err != nil {
		diag.FetchByUIDError = "sample latest message by sequence failed: " + err.Error()
		diag.Warnings = append(diag.Warnings, diag.FetchByUIDError)
		finalizeDiagnostics(diag)
		return diag, nil
	}
	if len(sample) == 0 || sample[0].UID == 0 {
		diag.FetchByUIDError = "latest sequence returned no UID"
		diag.Warnings = append(diag.Warnings, diag.FetchByUIDError)
		finalizeDiagnostics(diag)
		return diag, nil
	}

	diag.FetchByUID = sample[0].UID
	byUID, err := fetchSummaries(ic, mailbox, []imap.UID{imap.UID(sample[0].UID)})
	if err != nil {
		diag.FetchByUIDError = err.Error()
		diag.Warnings = append(diag.Warnings, "UID fetch failed: "+diag.FetchByUIDError)
		finalizeDiagnostics(diag)
		return diag, nil
	}
	if len(byUID) == 0 {
		diag.FetchByUIDError = "message not found when fetched by UID"
		diag.Warnings = append(diag.Warnings, diag.FetchByUIDError)
		finalizeDiagnostics(diag)
		return diag, nil
	}
	diag.FetchByUIDOK = true
	finalizeDiagnostics(diag)
	return diag, nil
}

func (c *Client) MailboxSyncHealth(mailbox string, targetMessages int) (*MailboxSyncHealth, error) {
	diag, err := c.MailboxDiagnostics(mailbox)
	if err != nil {
		return nil, err
	}
	health := &MailboxSyncHealth{
		Mailbox:          diag.Mailbox,
		Exists:           diag.Exists,
		Messages:         diag.Messages,
		UIDNext:          diag.UIDNext,
		UIDValidity:      diag.UIDValidity,
		TargetMessages:   targetMessages,
		Healthy:          diag.Healthy,
		UsableForTriage:  diag.UsableForTriage,
		DiagnosticStatus: diag.DiagnosticStatus,
		Warnings:         append([]string{}, diag.Warnings...),
	}
	if targetMessages > 0 {
		health.PercentOfTarget = float64(diag.Messages) / float64(targetMessages) * 100
		if diag.Messages < uint32(targetMessages) {
			health.Warnings = append(health.Warnings, "mailbox message count is below targetMessages")
		}
	}
	if diag.SelectOK && diag.Exists > 0 {
		sample, err := c.SampleRecentHeaders(diag.Mailbox, 1)
		if err != nil {
			health.Warnings = append(health.Warnings, "latest header sample failed: "+err.Error())
			return health, nil
		}
		if len(sample.Results) > 0 {
			latest := sample.Results[0]
			health.LatestUID = latest.UID
			health.LatestDate = latest.Date
			health.LatestInternalDate = latest.InternalDate
		}
	}
	return health, nil
}

func (c *Client) FetchEmail(mailbox string, uid uint32) (*emailparse.Email, error) {
	raw, err := c.fetchBody(mailbox, uid, imap.FetchItemBodySection{Peek: true})
	if err != nil {
		return nil, err
	}
	email, err := emailparse.Parse(raw.Body, uid, mailbox)
	if err != nil {
		return nil, err
	}
	email.RawSize = int64(len(raw.Body))
	email.RFC822Size = raw.RFC822Size
	email.FetchComplete = raw.RFC822Size == 0 || int64(len(raw.Body)) >= raw.RFC822Size
	email.BodyTruncated = raw.RFC822Size > 0 && int64(len(raw.Body)) < raw.RFC822Size
	email.TextBodyBytes = len(email.TextBody)
	email.HTMLBodyBytes = len(email.HTMLBody)
	return email, nil
}

func (c *Client) GetHeaders(mailbox string, uid uint32) (map[string][]string, error) {
	raw, err := c.fetchBody(mailbox, uid, imap.FetchItemBodySection{
		Specifier: imap.PartSpecifierHeader,
		Peek:      true,
	})
	if err != nil {
		return nil, err
	}
	return emailparse.Headers(raw.Body)
}

func (c *Client) fetchBody(mailbox string, uid uint32, section imap.FetchItemBodySection) (*fetchedBody, error) {
	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	if _, err := selectMailbox(ic, mailbox); err != nil {
		return nil, err
	}
	options := &imap.FetchOptions{
		UID:         true,
		RFC822Size:  true,
		BodySection: []*imap.FetchItemBodySection{&section},
	}
	messages, err := ic.Fetch(imap.UIDSetNum(imap.UID(uid)), options).Collect()
	if err != nil {
		return nil, classifyIMAPError(err)
	}
	if len(messages) == 0 {
		return nil, ErrMessageNotFound
	}
	body := messages[0].FindBodySection(&section)
	if body == nil {
		return nil, ErrMessageNotFound
	}
	return &fetchedBody{Body: body, RFC822Size: messages[0].RFC822Size}, nil
}

func (c *Client) connect() (*clientlib.Client, error) {
	dialer := &net.Dialer{Timeout: c.cfg.IMAPTimeout()}
	options := &clientlib.Options{
		Dialer:      dialer,
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}

	var (
		ic  *clientlib.Client
		err error
	)
	if c.cfg.IMAP.Secure {
		options.TLSConfig = &tls.Config{ServerName: c.cfg.IMAP.Host, MinVersion: tls.VersionTLS12}
		ic, err = clientlib.DialTLS(c.cfg.IMAPAddr(), options)
	} else {
		ic, err = clientlib.DialInsecure(c.cfg.IMAPAddr(), options)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to IMAP %s: %w", c.cfg.IMAPAddr(), err)
	}

	if err := ic.Login(c.cfg.IMAP.User, c.cfg.IMAP.Pass).Wait(); err != nil {
		closeClient(ic)
		return nil, ErrAuthFailed
	}
	return ic, nil
}

func selectMailbox(c *clientlib.Client, mailbox string) (*imap.SelectData, error) {
	if mailbox == "" {
		return nil, ErrMailboxNotFound
	}
	data, err := c.Select(mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return nil, classifyIMAPError(err)
	}
	return data, nil
}

func closeClient(c *clientlib.Client) {
	if c == nil {
		return
	}
	_ = c.Logout().Wait()
	_ = c.Close()
}

func finalizeDiagnostics(diag *MailboxDiagnostics) {
	diag.Empty = diag.SelectOK && diag.Exists == 0
	diag.UsableForTriage = diag.SelectOK && !diag.Empty && diag.FetchByUIDOK
	diag.Healthy = diag.StatusOK && diag.SelectOK && (diag.Empty || diag.FetchByUIDOK)

	switch {
	case !diag.SelectOK:
		diag.DiagnosticStatus = "unusable: SELECT failed"
	case diag.Empty:
		diag.DiagnosticStatus = "empty: selectable mailbox has zero messages"
	case !diag.FetchByUIDOK:
		diag.DiagnosticStatus = "unusable: sample fetch by UID failed"
	case !diag.StatusOK:
		diag.DiagnosticStatus = "partial: SELECT and UID fetch work, STATUS failed"
	default:
		diag.DiagnosticStatus = "healthy"
	}
}

func flagsToStrings(flags []imap.Flag) []string {
	out := make([]string, 0, len(flags))
	for _, flag := range flags {
		out = append(out, string(flag))
	}
	return out
}

func classifyIMAPError(err error) error {
	if err == nil {
		return nil
	}
	var imapErr *imap.Error
	if errors.As(err, &imapErr) {
		status := (*imap.StatusResponse)(imapErr)
		text := strings.ToLower(status.Text)
		if strings.Contains(text, "mailbox") || strings.Contains(text, "folder") || strings.Contains(text, "doesn't exist") || strings.Contains(text, "not exist") {
			return ErrMailboxNotFound
		}
	}
	return err
}

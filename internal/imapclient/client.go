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

type MessageSummary struct {
	UID     uint32   `json:"uid"`
	Mailbox string   `json:"mailbox"`
	Subject string   `json:"subject"`
	From    []string `json:"from"`
	To      []string `json:"to"`
	Date    string   `json:"date"`
	Size    int64    `json:"size,omitempty"`
	SeqNum  uint32   `json:"seq_num,omitempty"`
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

func (c *Client) FetchEmail(mailbox string, uid uint32) (*emailparse.Email, error) {
	raw, err := c.fetchBody(mailbox, uid, imap.FetchItemBodySection{Peek: true})
	if err != nil {
		return nil, err
	}
	return emailparse.Parse(raw, uid, mailbox)
}

func (c *Client) GetHeaders(mailbox string, uid uint32) (map[string][]string, error) {
	raw, err := c.fetchBody(mailbox, uid, imap.FetchItemBodySection{
		Specifier: imap.PartSpecifierHeader,
		Peek:      true,
	})
	if err != nil {
		return nil, err
	}
	return emailparse.Headers(raw)
}

func (c *Client) fetchBody(mailbox string, uid uint32, section imap.FetchItemBodySection) ([]byte, error) {
	ic, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer closeClient(ic)

	if err := selectMailbox(ic, mailbox); err != nil {
		return nil, err
	}
	options := &imap.FetchOptions{
		UID:         true,
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
	return body, nil
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

func selectMailbox(c *clientlib.Client, mailbox string) error {
	if mailbox == "" {
		return ErrMailboxNotFound
	}
	if _, err := c.Select(mailbox, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return classifyIMAPError(err)
	}
	return nil
}

func closeClient(c *clientlib.Client) {
	if c == nil {
		return
	}
	_ = c.Logout().Wait()
	_ = c.Close()
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

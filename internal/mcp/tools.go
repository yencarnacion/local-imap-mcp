package mcp

import (
	"encoding/json"
	"fmt"

	"local-imap-mcp/internal/imapclient"
)

type ToolRunner struct {
	imap *imapclient.Client
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func NewToolRunner(imap *imapclient.Client) *ToolRunner {
	return &ToolRunner{imap: imap}
}

func Tools() []Tool {
	return []Tool{
		{
			Name:        "list_mailboxes",
			Description: "List all IMAP mailboxes available from the local Dovecot server.",
			InputSchema: objectSchema(map[string]any{}),
		},
		{
			Name:        "count_messages",
			Description: "Return read-only SELECT counts and UID metadata for a mailbox.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":   stringSchema(),
				"inboxOnly": boolSchema(),
			}),
		},
		{
			Name:        "mailbox_diagnostics",
			Description: "Run mailbox preflight diagnostics: LIST, STATUS, SELECT, flags, UID metadata, and sample fetch-by-UID health.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":   stringSchema(),
				"inboxOnly": boolSchema(),
			}),
		},
		{
			Name:        "sample_recent_headers",
			Description: "Fetch the newest message headers by sequence number for a mailbox.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":   stringSchema(),
				"limit":     intSchema(),
				"inboxOnly": boolSchema(),
			}, "mailbox"),
		},
		{
			Name:        "search_by_subject",
			Description: "Search messages by subject text.",
			InputSchema: objectSchema(map[string]any{
				"subject":    stringSchema(),
				"mailbox":    stringSchema(),
				"startDate":  stringSchema(),
				"maxResults": intSchema(),
				"inboxOnly":  boolSchema(),
			}, "subject"),
		},
		{
			Name:        "search_recent",
			Description: "Return messages newer than the supplied number of days.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":    stringSchema(),
				"days":       intSchema(),
				"maxResults": intSchema(),
				"inboxOnly":  boolSchema(),
			}, "days"),
		},
		{
			Name:        "fetch_email",
			Description: "Fetch and parse a full email by mailbox and UID. Attachments are returned as metadata only.",
			InputSchema: objectSchema(map[string]any{
				"mailbox": stringSchema(),
				"uid":     intSchema(),
			}, "mailbox", "uid"),
		},
		{
			Name:        "get_email_headers",
			Description: "Fetch only the headers for an email by mailbox and UID.",
			InputSchema: objectSchema(map[string]any{
				"mailbox": stringSchema(),
				"uid":     intSchema(),
			}, "mailbox", "uid"),
		},
		{
			Name:        "search_from",
			Description: "Search messages by From header text.",
			InputSchema: objectSchema(map[string]any{
				"from":       stringSchema(),
				"mailbox":    stringSchema(),
				"startDate":  stringSchema(),
				"maxResults": intSchema(),
				"inboxOnly":  boolSchema(),
			}, "from"),
		},
		{
			Name:        "search_to",
			Description: "Search messages by To header text.",
			InputSchema: objectSchema(map[string]any{
				"to":         stringSchema(),
				"mailbox":    stringSchema(),
				"startDate":  stringSchema(),
				"maxResults": intSchema(),
				"inboxOnly":  boolSchema(),
			}, "to"),
		},
		{
			Name:        "search_since",
			Description: "Search messages since a YYYY-MM-DD date.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":    stringSchema(),
				"startDate":  stringSchema(),
				"maxResults": intSchema(),
				"inboxOnly":  boolSchema(),
			}, "startDate"),
		},
		{
			Name:        "scan_headers_range",
			Description: "Page compact headers newest-to-oldest by stable UID windows. Returns hasMore, nextBeforeUID, cursor, UIDVALIDITY, flags, and thread headers for exhaustive audits.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":             stringSchema(),
				"startDate":           stringSchema(),
				"beforeUID":           intSchema(),
				"afterUID":            intSchema(),
				"cursor":              stringSchema(),
				"limit":               intSchema(),
				"uidWindow":           intSchema(),
				"from":                stringSchema(),
				"to":                  stringSchema(),
				"senderDomain":        stringSchema(),
				"unreadOnly":          boolSchema(),
				"hasReplyHeaders":     boolSchema(),
				"collapseThreads":     boolSchema(),
				"stopAtDateThreshold": boolSchema(),
				"inboxOnly":           boolSchema(),
			}),
		},
		{
			Name:        "count_date_window",
			Description: "Exhaustively count headers in a date window, with optional address filters and thread collapse.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":         stringSchema(),
				"startDate":       stringSchema(),
				"endDate":         stringSchema(),
				"beforeUID":       intSchema(),
				"afterUID":        intSchema(),
				"uidWindow":       intSchema(),
				"from":            stringSchema(),
				"to":              stringSchema(),
				"senderDomain":    stringSchema(),
				"unreadOnly":      boolSchema(),
				"hasReplyHeaders": boolSchema(),
				"collapseThreads": boolSchema(),
				"inboxOnly":       boolSchema(),
			}, "startDate"),
		},
		{
			Name:        "search_body",
			Description: "Search full message bodies by literal text or regex over a bounded UID page. Returns matching headers and snippets.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":       stringSchema(),
				"pattern":       stringSchema(),
				"regex":         boolSchema(),
				"caseSensitive": boolSchema(),
				"startDate":     stringSchema(),
				"endDate":       stringSchema(),
				"beforeUID":     intSchema(),
				"afterUID":      intSchema(),
				"cursor":        stringSchema(),
				"limit":         intSchema(),
				"uidWindow":     intSchema(),
				"inboxOnly":     boolSchema(),
			}, "pattern"),
		},
		{
			Name:        "mailbox_sync_health",
			Description: "Expose mailbox import/sync health, UID metadata, latest sampled message, and optional target progress.",
			InputSchema: objectSchema(map[string]any{
				"mailbox":        stringSchema(),
				"targetMessages": intSchema(),
				"inboxOnly":      boolSchema(),
			}),
		},
	}
}

func (r *ToolRunner) Call(name string, args json.RawMessage) (any, error) {
	switch name {
	case "list_mailboxes":
		return r.imap.ListMailboxes()
	case "count_messages":
		var req mailboxRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return r.imap.CountMessages(mailboxArg(req.Mailbox, req.InboxOnly))
	case "mailbox_diagnostics":
		var req mailboxRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return r.imap.MailboxDiagnostics(mailboxArg(req.Mailbox, req.InboxOnly))
	case "sample_recent_headers":
		var req sampleHeadersRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return r.imap.SampleRecentHeaders(mailboxArg(req.Mailbox, req.InboxOnly), req.Limit)
	case "search_by_subject":
		var req subjectRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.Subject == "" {
			return nil, fmt.Errorf("subject is required")
		}
		return r.imap.SearchBySubject(imapclient.SearchQuery{
			Mailbox: mailboxArg(req.Mailbox, req.InboxOnly), Subject: req.Subject, StartDate: req.StartDate, MaxResults: req.MaxResults,
		})
	case "search_recent":
		var req recentRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return r.imap.SearchRecent(imapclient.SearchQuery{
			Mailbox: mailboxArg(req.Mailbox, req.InboxOnly), Days: req.Days, MaxResults: req.MaxResults,
		})
	case "fetch_email":
		var req uidRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.Mailbox == "" || req.UID == 0 {
			return nil, fmt.Errorf("mailbox and uid are required")
		}
		return r.imap.FetchEmail(req.Mailbox, req.UID)
	case "get_email_headers":
		var req uidRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.Mailbox == "" || req.UID == 0 {
			return nil, fmt.Errorf("mailbox and uid are required")
		}
		return r.imap.GetHeaders(req.Mailbox, req.UID)
	case "search_from":
		var req fromRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.From == "" {
			return nil, fmt.Errorf("from is required")
		}
		return r.imap.SearchFrom(imapclient.SearchQuery{
			Mailbox: mailboxArg(req.Mailbox, req.InboxOnly), From: req.From, StartDate: req.StartDate, MaxResults: req.MaxResults,
		})
	case "search_to":
		var req toRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.To == "" {
			return nil, fmt.Errorf("to is required")
		}
		return r.imap.SearchTo(imapclient.SearchQuery{
			Mailbox: mailboxArg(req.Mailbox, req.InboxOnly), To: req.To, StartDate: req.StartDate, MaxResults: req.MaxResults,
		})
	case "search_since":
		var req sinceRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.StartDate == "" {
			return nil, fmt.Errorf("startDate is required")
		}
		return r.imap.SearchSince(imapclient.SearchQuery{
			Mailbox: mailboxArg(req.Mailbox, req.InboxOnly), StartDate: req.StartDate, MaxResults: req.MaxResults,
		})
	case "scan_headers_range":
		var req scanHeadersRangeRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return r.imap.ScanHeadersRange(imapclient.HeaderScanQuery{
			Mailbox:             mailboxArg(req.Mailbox, req.InboxOnly),
			StartDate:           req.StartDate,
			BeforeUID:           req.BeforeUID,
			AfterUID:            req.AfterUID,
			Cursor:              req.Cursor,
			Limit:               req.Limit,
			UIDWindow:           req.UIDWindow,
			From:                req.From,
			To:                  req.To,
			SenderDomain:        req.SenderDomain,
			UnreadOnly:          req.UnreadOnly,
			HasReplyHeaders:     req.HasReplyHeaders,
			CollapseThreads:     req.CollapseThreads,
			StopAtDateThreshold: req.StopAtDateThreshold,
		})
	case "count_date_window":
		var req countDateWindowRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.StartDate == "" {
			return nil, fmt.Errorf("startDate is required")
		}
		return r.imap.CountDateWindow(imapclient.DateWindowCountQuery{
			Mailbox:         mailboxArg(req.Mailbox, req.InboxOnly),
			StartDate:       req.StartDate,
			EndDate:         req.EndDate,
			BeforeUID:       req.BeforeUID,
			AfterUID:        req.AfterUID,
			UIDWindow:       req.UIDWindow,
			From:            req.From,
			To:              req.To,
			SenderDomain:    req.SenderDomain,
			UnreadOnly:      req.UnreadOnly,
			HasReplyHeaders: req.HasReplyHeaders,
			CollapseThreads: req.CollapseThreads,
		})
	case "search_body":
		var req bodySearchRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		if req.Pattern == "" {
			return nil, fmt.Errorf("pattern is required")
		}
		return r.imap.SearchBody(imapclient.BodySearchQuery{
			Mailbox:       mailboxArg(req.Mailbox, req.InboxOnly),
			Pattern:       req.Pattern,
			Regex:         req.Regex,
			CaseSensitive: req.CaseSensitive,
			StartDate:     req.StartDate,
			EndDate:       req.EndDate,
			BeforeUID:     req.BeforeUID,
			AfterUID:      req.AfterUID,
			Cursor:        req.Cursor,
			Limit:         req.Limit,
			UIDWindow:     req.UIDWindow,
		})
	case "mailbox_sync_health":
		var req syncHealthRequest
		if err := decodeArgs(args, &req); err != nil {
			return nil, err
		}
		return r.imap.MailboxSyncHealth(mailboxArg(req.Mailbox, req.InboxOnly), req.TargetMessages)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

type mailboxRequest struct {
	Mailbox   string `json:"mailbox"`
	InboxOnly bool   `json:"inboxOnly"`
}

type sampleHeadersRequest struct {
	Mailbox   string `json:"mailbox"`
	Limit     int    `json:"limit"`
	InboxOnly bool   `json:"inboxOnly"`
}

type subjectRequest struct {
	Subject    string `json:"subject"`
	Mailbox    string `json:"mailbox"`
	StartDate  string `json:"startDate"`
	MaxResults int    `json:"maxResults"`
	InboxOnly  bool   `json:"inboxOnly"`
}

type recentRequest struct {
	Mailbox    string `json:"mailbox"`
	Days       int    `json:"days"`
	MaxResults int    `json:"maxResults"`
	InboxOnly  bool   `json:"inboxOnly"`
}

type uidRequest struct {
	Mailbox string `json:"mailbox"`
	UID     uint32 `json:"uid"`
}

type fromRequest struct {
	From       string `json:"from"`
	Mailbox    string `json:"mailbox"`
	StartDate  string `json:"startDate"`
	MaxResults int    `json:"maxResults"`
	InboxOnly  bool   `json:"inboxOnly"`
}

type toRequest struct {
	To         string `json:"to"`
	Mailbox    string `json:"mailbox"`
	StartDate  string `json:"startDate"`
	MaxResults int    `json:"maxResults"`
	InboxOnly  bool   `json:"inboxOnly"`
}

type sinceRequest struct {
	Mailbox    string `json:"mailbox"`
	StartDate  string `json:"startDate"`
	MaxResults int    `json:"maxResults"`
	InboxOnly  bool   `json:"inboxOnly"`
}

type scanHeadersRangeRequest struct {
	Mailbox             string `json:"mailbox"`
	StartDate           string `json:"startDate"`
	BeforeUID           uint32 `json:"beforeUID"`
	AfterUID            uint32 `json:"afterUID"`
	Cursor              string `json:"cursor"`
	Limit               int    `json:"limit"`
	UIDWindow           int    `json:"uidWindow"`
	From                string `json:"from"`
	To                  string `json:"to"`
	SenderDomain        string `json:"senderDomain"`
	UnreadOnly          bool   `json:"unreadOnly"`
	HasReplyHeaders     bool   `json:"hasReplyHeaders"`
	CollapseThreads     bool   `json:"collapseThreads"`
	StopAtDateThreshold bool   `json:"stopAtDateThreshold"`
	InboxOnly           bool   `json:"inboxOnly"`
}

type countDateWindowRequest struct {
	Mailbox         string `json:"mailbox"`
	StartDate       string `json:"startDate"`
	EndDate         string `json:"endDate"`
	BeforeUID       uint32 `json:"beforeUID"`
	AfterUID        uint32 `json:"afterUID"`
	UIDWindow       int    `json:"uidWindow"`
	From            string `json:"from"`
	To              string `json:"to"`
	SenderDomain    string `json:"senderDomain"`
	UnreadOnly      bool   `json:"unreadOnly"`
	HasReplyHeaders bool   `json:"hasReplyHeaders"`
	CollapseThreads bool   `json:"collapseThreads"`
	InboxOnly       bool   `json:"inboxOnly"`
}

type bodySearchRequest struct {
	Mailbox       string `json:"mailbox"`
	Pattern       string `json:"pattern"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	StartDate     string `json:"startDate"`
	EndDate       string `json:"endDate"`
	BeforeUID     uint32 `json:"beforeUID"`
	AfterUID      uint32 `json:"afterUID"`
	Cursor        string `json:"cursor"`
	Limit         int    `json:"limit"`
	UIDWindow     int    `json:"uidWindow"`
	InboxOnly     bool   `json:"inboxOnly"`
}

type syncHealthRequest struct {
	Mailbox        string `json:"mailbox"`
	TargetMessages int    `json:"targetMessages"`
	InboxOnly      bool   `json:"inboxOnly"`
}

func mailboxArg(mailbox string, inboxOnly bool) string {
	if mailbox != "" {
		return mailbox
	}
	if inboxOnly {
		return "INBOX"
	}
	return ""
}

func decodeArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringSchema() map[string]string {
	return map[string]string{"type": "string"}
}

func intSchema() map[string]string {
	return map[string]string{"type": "integer"}
}

func boolSchema() map[string]string {
	return map[string]string{"type": "boolean"}
}

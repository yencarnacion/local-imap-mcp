# local-imap-mcp

A small Go HTTP server that exposes a read-only MCP-style JSON-RPC endpoint for email cached locally with `mbsync`/`isync` and served by Dovecot IMAP on `127.0.0.1:143`.

The server listens at:

```text
http://host:port/mcp
```

By default, this project is read-only. It does not delete, move, expunge, mark read, mark unread, or send mail.

## Tools

- `list_mailboxes`
- `count_messages`
- `mailbox_diagnostics`
- `sample_recent_headers`
- `search_by_subject`
- `search_recent`
- `fetch_email`
- `get_email_headers`
- `search_from`
- `search_to`
- `search_since`
- `scan_headers_range`
- `count_date_window`
- `search_body`
- `mailbox_sync_health`

`fetch_email` requests the full IMAP `BODY[]` payload and returns parsed text and HTML bodies plus attachment metadata only. Attachments are not saved. It also returns `raw_size`, `rfc822_size`, `fetch_complete`, `body_truncated`, `text_body_bytes`, and `html_body_bytes` so callers can distinguish a complete fetch from client-side display truncation. Search snippets may contain leading or trailing `...`; `fetch_email` does not add ellipses to bodies.

Search-style tools return an object with `results`, `returned`, `hasMore`, `truncated`, and `warnings`. If `maxResults` is omitted, the configured `imap.max_results` default is used. If `maxResults` is provided, that explicit value is honored for admin/reporting workflows.

For large mailbox audits, prefer `mailbox_diagnostics`, `count_date_window`, and paged `scan_headers_range` over one-shot search calls. `scan_headers_range` pages compact header records by UID window and returns `hasMore`, `nextBeforeUID`, `cursor`, `uidValidity`, `uidNext`, `scannedMessages`, and `warnings` so clients can resume safely without assuming a small response is complete.

## Ubuntu 22.04 Setup

```bash
sudo apt install golang-go
cp .env.example .env
edit .env
go mod tidy
go run ./cmd/local-imap-mcp
```

Example `.env`:

```bash
IMAP_HOST=127.0.0.1
IMAP_PORT=143
IMAP_SECURE=false
IMAP_USER=yamir
IMAP_PASS=change-me
```

Example `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8095
  mcp_path: "/mcp"

imap:
  default_mailbox: "AllMail"
  max_results: 50
  timeout_seconds: 30

safety:
  read_only: true
  allow_delete: false
  allow_move: false
  allow_send: false
```

## Dovecot Check

For STARTTLS-capable local IMAP, this command should connect:

```bash
openssl s_client -connect 127.0.0.1:143 -starttls imap
```

Then try:

```text
a login yamir password
b list "" "*"
```

This server supports normal mailbox names such as `INBOX` and `AllMail`.

## JSON-RPC / MCP Requests

Initialize:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | jq
```

List tools:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | jq
```

List mailboxes:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_mailboxes","arguments":{}}}' | jq
```

Count messages in `AllMail`:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"count_messages","arguments":{"mailbox":"AllMail"}}}' | jq
```

Run mailbox health diagnostics before triage:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"mailbox_diagnostics","arguments":{"mailbox":"AllMail"}}}' | jq
```

Sample recent headers from `AllMail`:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"sample_recent_headers","arguments":{"mailbox":"AllMail","limit":10}}}' | jq
```

Search recent mail in the default `AllMail` mailbox:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"search_recent","arguments":{"mailbox":"AllMail","days":7,"maxResults":10}}}' | jq
```

Search `AllMail` since a date:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"search_since","arguments":{"mailbox":"AllMail","startDate":"2026-05-20","maxResults":20}}}' | jq
```

Search subject:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"search_by_subject","arguments":{"mailbox":"AllMail","subject":"Online Reading Summary","maxResults":20}}}' | jq
```

Fetch one message:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"fetch_email","arguments":{"mailbox":"AllMail","uid":123}}}' | jq
```

Get headers only:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"get_email_headers","arguments":{"mailbox":"AllMail","uid":123}}}' | jq
```

Page compact headers by UID window:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"scan_headers_range","arguments":{"mailbox":"AllMail","startDate":"2026-05-01","limit":200,"uidWindow":2000}}}' | jq
```

Resume the scan using the returned `cursor`:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"scan_headers_range","arguments":{"cursor":"PASTE_RETURNED_CURSOR_HERE","limit":200,"uidWindow":2000}}}' | jq
```

## Exhaustive Mailbox Audits

Recommended response-needed audit workflow for large mailboxes:

1. Run `mailbox_diagnostics` for the target mailbox. Continue only if `usableForTriage` is true. If `diagnosticStatus` says the mailbox is empty, unselectable, or UID fetch failed, fix that mailbox before auditing it.
2. Call `count_date_window` first when you need an exact count for a date range. It scans by UID without assuming dates are monotonic, and supports `collapseThreads` for thread-level counts.
3. Call `scan_headers_range` with a modest `limit`, such as `100` to `300`, and a UID window sized for the mailbox, such as `1000` to `5000`. Include `startDate` for the audit date and optionally filters like `senderDomain`, `from`, `to`, `unreadOnly`, `hasReplyHeaders`, or `collapseThreads`.
4. Give only the returned `headers` array to the LLM for triage. These records include UID, mailbox, date, from, to, subject, message ID, in-reply-to, references, flags, and size without full bodies.
5. If `hasMore` is true, call `scan_headers_range` again with `cursor`. Repeat until `complete` is true.
6. Shortlist likely response-needed messages by UID. Fetch full content only for those candidates with `fetch_email`, or use `search_body` for targeted literal or regex body searches.

For a "May 1 forward" audit with limited LLM context, use chunks:

```json
{
  "name": "scan_headers_range",
  "arguments": {
    "mailbox": "AllMail",
    "startDate": "2026-05-01",
    "limit": 150,
    "uidWindow": 3000,
    "unreadOnly": false
  }
}
```

After each page, ask the LLM to return only a compact shortlist such as:

```json
[
  {"mailbox": "AllMail", "uid": 12345, "reason": "direct question from client"},
  {"mailbox": "AllMail", "uid": 12312, "reason": "unanswered scheduling request"}
]
```

Then fetch only shortlisted UIDs:

```json
{
  "name": "fetch_email",
  "arguments": {"mailbox": "AllMail", "uid": 12345}
}
```

`scan_headers_range` is intentionally page-oriented. A response with `truncated: true` is not complete; resume with `cursor` or `nextBeforeUID`.

Exact date-window count:

```json
{
  "name": "count_date_window",
  "arguments": {
    "mailbox": "AllMail",
    "startDate": "2026-05-01",
    "endDate": "2026-05-31",
    "collapseThreads": true
  }
}
```

Targeted body regex search:

```json
{
  "name": "search_body",
  "arguments": {
    "mailbox": "AllMail",
    "pattern": "\\b787[-. ]?\\d{3}[-. ]?\\d{4}\\b",
    "regex": true,
    "startDate": "2026-05-01",
    "limit": 20,
    "uidWindow": 500
  }
}
```

Sync health and optional import progress:

```json
{
  "name": "mailbox_sync_health",
  "arguments": {"mailbox": "AllMail", "targetMessages": 250000}
}
```

## Python Test Client

```bash
python3 examples/test_client.py
```

Or point it at another URL:

```bash
python3 examples/test_client.py http://127.0.0.1:8095/mcp
```

For a fuller manual MCP diagnostic against the default local test URL:

```bash
python3 examples/manual_mcp_test.py
```

## Import Progress

Run this on the machine that can connect to your local Dovecot IMAP server. It reads the same `.env` and `config.yaml` as the MCP server.

One-shot status:

```bash
go run ./cmd/local-imap-progress -mailbox AllMail
```

Watch progress every 10 seconds:

```bash
go run ./cmd/local-imap-progress -mailbox AllMail -watch -interval 10s
```

Or build a reusable binary:

```bash
go build -o local-imap-progress ./cmd/local-imap-progress
./local-imap-progress -mailbox AllMail -watch -interval 10s
```

If you know the remote/source mailbox total, pass it as `-target` to get percent and ETA:

```bash
go run ./cmd/local-imap-progress -mailbox AllMail -watch -interval 10s -target 250000
```

Example output:

```text
local-imap-mcp import progress
===============================
time:      2026-05-22T14:10:00-04:00
mailbox:   AllMail
messages:  105075
uidNext:   105076
uidValid:  1779458646
recent:    0
delta:     +314 in 10s (1884.0 msg/min)
target:    250000 (42.03%, 144925 remaining)
eta:       1h16m55s at current rate

latest exposed message
----------------------
seq/uid:   105075 / 105075
date:      2015-01-20T20:00:49Z
internal:  2015-01-20T20:01:17Z
from:      Jennie (Udacity Team) <support@udacity.com>
subject:   Help us design a new Android course for beginners
```

## systemd

Example unit file is in `systemd/local-imap-mcp.service`.

Install it as:

```bash
sudo cp systemd/local-imap-mcp.service /etc/systemd/system/local-imap-mcp.service
sudo systemctl daemon-reload
sudo systemctl enable --now local-imap-mcp
sudo systemctl status local-imap-mcp
```

The unit runs as `yamir`, uses `/home/yamir/Documents/local-imap-mcp` as its working directory, and listens on port `8095`.

## Notes

- `.env` contains secrets and should not be committed.
- IMAP passwords are never logged.
- Each tool call logs the tool name and duration.
- Empty searches return an empty result, not an error.

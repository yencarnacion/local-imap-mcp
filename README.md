# local-imap-mcp

A small Go HTTP server that exposes a read-only MCP-style JSON-RPC endpoint for email cached locally with `mbsync`/`isync` and served by Dovecot IMAP on `127.0.0.1:143`.

The server listens at:

```text
http://host:port/mcp
```

By default, this project is read-only. It does not delete, move, expunge, mark read, mark unread, or send mail.

## Tools

- `list_mailboxes`
- `search_by_subject`
- `search_recent`
- `fetch_email`
- `get_email_headers`
- `search_from`
- `search_to`
- `search_since`

`fetch_email` returns parsed text and HTML bodies plus attachment metadata only. Attachments are not saved.

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
  default_mailbox: "INBOX"
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

Search recent mail:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"search_recent","arguments":{"mailbox":"INBOX","days":7,"maxResults":10}}}' | jq
```

Search subject:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_by_subject","arguments":{"mailbox":"AllMail","subject":"invoice","startDate":"2026-01-01","maxResults":10}}}' | jq
```

Fetch one message:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"fetch_email","arguments":{"mailbox":"INBOX","uid":123}}}' | jq
```

Get headers only:

```bash
curl -s http://127.0.0.1:8095/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_email_headers","arguments":{"mailbox":"INBOX","uid":123}}}' | jq
```

## Python Test Client

```bash
python3 examples/test_client.py
```

Or point it at another URL:

```bash
python3 examples/test_client.py http://127.0.0.1:8095/mcp
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

#!/usr/bin/env python3

import json
import urllib.error
import urllib.request

URL = "http://10.17.17.90:8095/mcp"
PROTOCOL_VERSION = "2025-06-18"

HEADERS = {
    "Content-Type": "application/json",
    "Accept": "application/json",
    "MCP-Protocol-Version": PROTOCOL_VERSION,
}

next_id = 1


def post(payload):
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(URL, data=data, headers=HEADERS, method="POST")

    with urllib.request.urlopen(req, timeout=60) as resp:
        raw = resp.read().decode("utf-8", errors="replace")
        return json.loads(raw) if raw else None


def rpc(method, params=None):
    global next_id

    payload = {
        "jsonrpc": "2.0",
        "id": next_id,
        "method": method,
        "params": params or {},
    }
    next_id += 1

    print(f"\n=== {method} REQUEST ===")
    print(json.dumps(payload, indent=2))

    try:
        result = post(payload)
        print(f"\n=== {method} RESPONSE ===")
        print(json.dumps(result, indent=2))
        return result
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        print(f"\nHTTP ERROR {e.code}")
        print(body)
    except Exception as e:
        print(f"\nERROR: {e!r}")


def notify(method, params=None):
    payload = {
        "jsonrpc": "2.0",
        "method": method,
        "params": params or {},
    }

    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(URL, data=data, headers=HEADERS, method="POST")

    print(f"\n=== {method} NOTIFY ===")

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            print(f"status={resp.status}")
    except Exception as e:
        print(f"notify error: {e!r}")


def call_tool(name, arguments):
    return rpc(
        "tools/call",
        {
            "name": name,
            "arguments": arguments,
        },
    )


def main():
    rpc(
        "initialize",
        {
            "protocolVersion": PROTOCOL_VERSION,
            "capabilities": {},
            "clientInfo": {
                "name": "manual-python-mcp-test",
                "version": "0.1.0",
            },
        },
    )

    notify("notifications/initialized")

    rpc("tools/list")

    call_tool("list_mailboxes", {})

    call_tool("count_messages", {"mailbox": "AllMail"})
    call_tool("count_messages", {"mailbox": "INBOX"})
    call_tool("sample_recent_headers", {"mailbox": "AllMail", "limit": 10})

    call_tool(
        "search_since",
        {
            "mailbox": "AllMail",
            "startDate": "2026-05-20",
            "maxResults": 50,
        },
    )

    # Sanity check for currently imported historical mail. If this returns rows
    # while 2026 returns [], the server search path is working and AllMail has
    # not yet exposed 2026-dated messages through Dovecot.
    call_tool(
        "search_since",
        {
            "mailbox": "AllMail",
            "startDate": "2014-09-25",
            "maxResults": 5,
        },
    )

    call_tool(
        "search_recent",
        {
            "mailbox": "AllMail",
            "days": 7,
            "maxResults": 50,
        },
    )

    call_tool(
        "search_by_subject",
        {
            "mailbox": "AllMail",
            "subject": "(US) Friday Morning Online Reading Summary",
            "maxResults": 20,
        },
    )

    call_tool(
        "search_by_subject",
        {
            "mailbox": "AllMail",
            "subject": "Friday Morning Online Reading Summary",
            "maxResults": 20,
        },
    )

    call_tool(
        "search_by_subject",
        {
            "mailbox": "AllMail",
            "subject": "Online Reading Summary",
            "maxResults": 20,
        },
    )


if __name__ == "__main__":
    main()

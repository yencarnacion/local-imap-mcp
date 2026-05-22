#!/usr/bin/env python3
import json
import sys
import urllib.request


URL = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8095/mcp"


def rpc(method, params=None, request_id=1):
    payload = {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": method,
    }
    if params is not None:
        payload["params"] = params
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        URL,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


def tool(name, arguments=None, request_id=10):
    return rpc(
        "tools/call",
        {"name": name, "arguments": arguments or {}},
        request_id=request_id,
    )


def main():
    print(json.dumps(rpc("initialize", request_id=1), indent=2))
    print(json.dumps(rpc("tools/list", request_id=2), indent=2))
    print(json.dumps(tool("list_mailboxes", request_id=3), indent=2))
    print(json.dumps(tool("count_messages", {"mailbox": "AllMail"}, request_id=4), indent=2))
    print(json.dumps(tool("search_recent", {"mailbox": "AllMail", "days": 7, "maxResults": 5}, request_id=5), indent=2))


if __name__ == "__main__":
    main()

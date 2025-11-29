# MCP Server - go-mycli

This is a tiny MCP (Model Context Protocol) HTTP proxy intended to be used as a local middleware to forward requests to a GitHub Copilot MCP endpoint or another MCP-capable endpoint.

## Run

```bash
# Local copilot MCP server (requires GitHub Copilot and gh plugin):
# 1) Authenticate copilot via gh
gh auth copilot login
# 2) Launch the local copilot MCP proxy
gh copilot mcp serve &

# Start this tiny proxy and forward to the local copilot server
# (this simply provides a stable access point, you can also call Copilot's local MCP directly)
cd mcp-server
go run . --listen :8800 --upstream http://127.0.0.1:44044/mcp
```

## TLS / Upstream

The proxy supports forwarding to any upstream MCP server; change `--upstream` to your provider if needed.

## Use with go-mycli

Point `go-mycli` at the proxy:

```bash
./bin/go-mycli --config ./.my.cnf --ai-server-url http://127.0.0.1:8800/mcp --ai-server-mode copilot_mcp_http -e "EXPLAIN SELECT * FROM actor\G"
```

The `ai-server-mode` selects the exact client behavior; `copilot_mcp_http` is the default and sends a plain HTTP request using the standard OpenAI-like chat body.

## License

MIT/Apache

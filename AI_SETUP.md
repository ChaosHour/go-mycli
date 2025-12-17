# AI Setup Guide

go-mycli supports AI-powered EXPLAIN analysis through multiple backends.
Choose the one that fits your needs.

## Quick Comparison

| Backend | Cost | Speed | Privacy | Setup |
|---------|------|-------|---------|-------|
| **GitHub Copilot** | Subscription | Fast | Cloud | Medium |
| **Ollama (Local)** | Free | Varies | Local | Easy |
| **OpenAI API** | Pay-per-use | Fast | Cloud | Easy |

---

## Option 1: GitHub Copilot (Recommended)

Use your existing GitHub Copilot subscription via the integrated sqlbot MCP server.

### Setup

1. **Build everything**

   ```bash
   make build
   ```

2. **Start the MCP server with Docker**

   ```bash
   make docker-up
   ```

3. **Run go-mycli**

   ```bash
   ./bin/go-mycli --config .my.cnf \
     --ai-server-url http://127.0.0.1:8800/mcp \
     --ai-server-mode copilot_mcp_http
   ```

### Architecture

```text
go-mycli → HTTP → mcp-server → sqlbot (Docker) → AI Analysis
```

### Docker Environment Variables

Set in `docker-compose.yml` or environment:

| Variable | Default | Description |
|----------|---------|-------------|
| `MYSQL_USER` | `root` | MySQL username |
| `MYSQL_PASS` | `s3cr3t` | MySQL password |
| `MYSQL_HOST` | `host.docker.internal:3306` | MySQL host:port |
| `MYSQL_DATABASE` | `sakila` | Database name |

---

## VS Code Integration

For seamless AI-powered EXPLAIN analysis directly in VS Code:

1. **Install GitHub Copilot Chat** extension
2. **Add to VS Code settings** (`settings.json`):

```json
{
  "github.copilot.chat.mcp": {
    "sqlbot": {
      "command": "docker",
      "args": ["run", "--rm", "-i", "sqlbot-mcp"],
      "env": {
        "MYSQL_USER": "root",
        "MYSQL_PASS": "s3cr3t", 
        "MYSQL_HOST": "host.docker.internal:3306",
        "MYSQL_DATABASE": "sakila"
      }
    }
  }
}
```

1. **Build and run**: `make docker-up`
2. **Use in Copilot Chat**: Ask "Explain this SQL query" with your query

This integrates your local sqlbot-mcp with GitHub Copilot for instant query analysis.

---

## Option 2: Ollama (Free & Local)

Run AI analysis entirely on your machine with Ollama.

### Ollama Setup

1. **Install Ollama**

   ```bash
   brew install ollama
   ```

2. **Start Ollama and pull a model**

   ```bash
   ollama serve &
   ollama pull llama3.1:latest
   ```

3. **Start the MCP proxy**

   ```bash
   ./bin/mcp-server --listen :8800 --upstream http://localhost:11434/v1/chat/completions
   ```

4. **Run go-mycli**

   ```bash
   ./bin/go-mycli --config .my.cnf \
     --ai-server-url http://127.0.0.1:8800/mcp \
     --ai-server-mode copilot_mcp_http
   ```

### Notes

- Speed depends on your hardware (GPU recommended)
- Models: `llama3.1`, `codellama`, `mistral` work well
- Completely offline after model download

---

## Option 3: OpenAI API

Use OpenAI's GPT models for fast, accurate analysis.

### OpenAI Setup

```bash
# 1. Get API key from https://platform.openai.com/api-keys
export OPENAI_API_KEY='sk-...'

# 2. Run go-mycli with OpenAI mode
./bin/go-mycli --config .my.cnf --ai-server-mode openai
```

### Cost

- ~$0.002 per EXPLAIN query (GPT-3.5-turbo)
- Responses are cached locally to avoid duplicate costs

---

## Configuration

### Command-Line Flags

```bash
./bin/go-mycli \
  --ai-server-url http://127.0.0.1:8800/mcp \
  --ai-server-mode copilot_mcp_http \
  --ai-cache-path ~/.go-mycli/ai_cache.db \
  --ai-detail-level expert
```

### Config File (`~/.go-myclirc`)

```ini
[main]
ai_analysis = true
visual_explain = true
json_export = false

[ai]
ai_server_url = http://127.0.0.1:8800/mcp
ai_server_mode = copilot_mcp_http
ai_cache_path = ~/.go-mycli/ai_cache.db
```

### Detail Levels

| Level | Description |
|-------|-------------|
| `basic` | Quick overview with cost and access types |
| `detailed` | Cost breakdown, grouping strategy, execution plan |
| `expert` | Deep analysis with index utilization metrics |

```bash
./bin/go-mycli --ai-detail-level expert
```

---

## Interactive Toggle

```sql
-- Enable/disable AI analysis on the fly
\ai on
\ai off
\ai toggle

-- Enable visual explain tree
\visual on

-- Enable JSON export for external tools
\json on
```

---

## Caching

AI responses are cached locally to:

- Avoid repeated API calls for identical queries
- Provide instant results for previously analyzed queries
- Reduce costs (OpenAI) or processing time (Ollama)

Cache location: `~/.go-mycli/ai_cache.db` (BoltDB)

```bash
# Clear cache
rm ~/.go-mycli/ai_cache.db
```

---

## Troubleshooting

### "AI analysis failed: connection refused"

```bash
# Check if MCP server is running
curl http://127.0.0.1:8800/health

# Start it
make docker-up
# or
./bin/mcp-server --listen :8800 --mcp-command "./bin/sqlbot"
```

### "No response from AI"

- Ollama: Ensure model is downloaded (`ollama list`)
- OpenAI: Check API key is set and has credits
- Copilot: Ensure Docker container is running (`docker ps`)

### Slow responses with Ollama

- Try a smaller model: `ollama pull mistral:7b`
- Use GPU acceleration if available
- Consider using OpenAI or Copilot for faster results

---

## MCP Server Details

The `mcp-server` is an HTTP-to-stdio bridge that:

1. Receives HTTP requests from go-mycli
2. Forwards them to the sqlbot MCP server (stdio)
3. Returns AI analysis results

See [mcp-server/README.md](mcp-server/README.md) for more details.

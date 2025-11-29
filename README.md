# go-mycli

A command-line MySQL client with AI-powered query analysis, written in Go.

Inspired by [mycli](https://github.com/dbcli/mycli) from the dbcli team.

## Features

- 🤖 **AI-Powered EXPLAIN Analysis** - Get optimization suggestions for your queries
- 🌳 **Visual Query Plans** - ASCII tree visualization of execution plans  
- ✨ **Syntax Highlighting** - Colorized SQL output with customizable themes
- 🎯 **Smart Auto-Completion** - Context-aware suggestions for tables, columns, and keywords
- 📦 **Zstd Compression** - Network and file compression support
- ⚙️ **Configurable** - Customize via `~/.go-myclirc`

## Quick Start

```bash
# Build
make build

# Connect to MySQL
./bin/go-mycli -u root -p password -h localhost database_name

# Or use a config file
./bin/go-mycli --config ~/.my.cnf
```

## Installation

```bash
# Build from source
make build
make install

# Docker (for AI server)
make docker-up
```

## Usage

### Basic Connection

```bash
# Direct connection
go-mycli -u username -p password -h localhost -P 3306 database

# With config file  
go-mycli --config ~/.my.cnf

# With compression (MySQL 8.0.18+)
go-mycli --zstd-compression-level=3 -h remote-server database
```

### Interactive Commands

| Command | Description |
|---------|-------------|
| `\q` | Quit |
| `\h` | Help |
| `\s` | Server status |
| `\u <db>` | Switch database |
| `\. <file>` | Execute SQL file (supports .zst) |
| `\! <cmd>` | Run shell command |
| `\ai on/off` | Toggle AI analysis |
| `\visual on/off` | Toggle visual explain |
| `\json on/off` | Toggle JSON export |

### AI-Powered Analysis

```sql
-- Enable AI analysis
\ai on

-- Run EXPLAIN query - AI automatically analyzes it
EXPLAIN FORMAT=JSON SELECT * FROM users WHERE email = 'test@example.com';

-- Output includes:
-- 1. Standard EXPLAIN output
-- 2. 🤖 AI Performance Analysis with optimization suggestions
-- 3. 🌳 Visual execution plan tree
```

**See [AI_SETUP.md](AI_SETUP.md) for AI backend configuration options.**

### Auto-Completion

go-mycli provides **context-aware** auto-completion:

- After `FROM`/`JOIN` → suggests only table names
- After `WHERE`/`SELECT` → suggests columns from tables in your query  
- After `table.` → suggests columns from that specific table
- Supports table aliases (`u.` after `FROM users u`)

Press **Tab** to trigger suggestions.

## Configuration

Create `~/.go-myclirc`:

```ini
[main]
syntax_style = monokai
suggestions = true
ai_analysis = true
visual_explain = true

[ai]
ai_server_url = http://127.0.0.1:8800/mcp
ai_server_mode = copilot_mcp_http
ai_cache_path = ~/.go-mycli/ai_cache.db

[colors]
keyword = #66D9EF
string = #E6DB74
number = #AE81FF
```

**Available themes:** `monokai`, `dracula`, `github`, `vim`, `nord`, `solarized-dark`, `solarized-light`, `gruvbox`

See [CONFIG.md](CONFIG.md) for all options.

## Documentation

| Document | Description |
|----------|-------------|
| [CONFIG.md](CONFIG.md) | All configuration options |
| [AI_SETUP.md](AI_SETUP.md) | AI backend setup (Copilot, Ollama, OpenAI) |
| [mcp-server/README.md](mcp-server/README.md) | MCP proxy server details |

## Examples

```bash
# Execute single query
go-mycli --config ~/.my.cnf -e "SHOW DATABASES"

# Pipe SQL
echo "SELECT COUNT(*) FROM users" | go-mycli --config ~/.my.cnf

# Execute compressed SQL file
go-mycli -e "\. large_dump.sql.zst"

# AI analysis with expert detail level
go-mycli --ai-detail-level expert --config ~/.my.cnf
```

## Example Session

```bash
# Start Docker and connect with AI analysis
make docker-up && ./bin/go-mycli --ai-detail-level expert \
  --ai-server-url http://127.0.0.1:8800/mcp \
  --ai-server-mode copilot_mcp_http sakila
```

```text
Connected to MySQL
MySQL version: 8.0.43
go-mycli 0.1.0
MySQL root@localhost:3306(sakila)> select * from actor where actor_id = 1;
→ select * from actor where actor_id = 1

+----------+------------+-----------+---------------------+
| actor_id | first_name | last_name | last_update         |
+----------+------------+-----------+---------------------+
| 1        | PENELOPE   | GUINESS   | 2006-02-15 04:34:33 |
+----------+------------+-----------+---------------------+
1 row in set (0.010s)

MySQL root@localhost:3306(sakila)> EXPLAIN FORMAT=JSON SELECT * FROM actor WHERE first_name = 'PENELOPE'\G
→ EXPLAIN FORMAT=JSON SELECT * FROM actor WHERE first_name = 'PENELOPE'

*************************** 1. row ***************************
EXPLAIN: {
  "query_block": {
    "select_id": 1,
    "cost_info": { "query_cost": "1.15" },
    "table": {
      "table_name": "actor",
      "access_type": "ref",
      "key": "idx_actor_first_name",
      "rows_examined_per_scan": 4,
      "filtered": "100.00"
    }
  }
}

1 row in set (0.003s)

🤖 AI Performance Analysis:
==========================
(AI backend: copilot_mcp_http http://127.0.0.1:8800/mcp)
🔬 Expert MySQL EXPLAIN Analysis
═══════════════════════════════════

📝 Query: SELECT * FROM actor WHERE first_name = 'PENELOPE'

🔬 Cost-Benefit Analysis:
  • Outer Query Cost: 1.15 units
  💰 Total Query Cost: 1.15 units

🗂️ Index Utilization:
📊 Table: actor
  🔍 Access Method: ref
  🗂️ Index Used: idx_actor_first_name
    Key Length: 137 bytes
  📊 Row Estimates: 4 rows
  🎯 Selectivity: 100.00% (excellent)

💡 Optimization Recommendations:
Query at theoretical optimum - cost minimization achieved
Index utilization good - single key lookup effective
Exceptional selectivity - query highly optimized

🌳 Built-in Visual Explain:
===========================
Query Execution Plan:
└──  [cost=1.15]
    └── actor (ref) using idx_actor_first_name [filtered=100.00%]
```

## License

BSD 3-Clause License - see [LICENSE.txt](LICENSE.txt)

## Acknowledgments

Thanks to the [mycli](https://github.com/dbcli/mycli) team for the inspiration.

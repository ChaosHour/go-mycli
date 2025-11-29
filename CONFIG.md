# Configuration Guide

## Overview

go-mycli now supports user configuration through the `~/.go-myclirc` file. This allows you to customize syntax highlighting colors and styles without modifying the code.

## Configuration File Location

The configuration file is located at:

- **macOS/Linux**: `~/.go-myclirc`
- **Windows**: `%USERPROFILE%\.go-myclirc`

## Default Configuration

When you first run go-mycli, it automatically creates a default configuration file with these settings:

```ini
[main]
syntax_style = monokai
use_custom_colors = false
suggestions = true
ai_analysis = false
json_export = false
visual_explain = false
ai_server_url = http://127.0.0.1:44044/mcp
ai_server_mode = copilot_mcp_http
ai_cache_path = ~/.go-mycli/ai_cache.db

[colors]
keyword = #66D9EF
name = #A6E22E
builtin = #FD971F
string = #E6DB74
number = #AE81FF
operator = #F92672
comment = #75715E
punctuation = #F8F8F2
```

## Customization Options

### Syntax Styles

You can use any of the built-in Chroma styles by setting `syntax_style` in the `[main]` section:

**Popular Styles:**

- `native` - Default terminal colors
- `monokai` - Monokai theme (dark)
- `dracula` - Dracula theme (dark)
- `github` - GitHub style (light)
- `vim` - Vim default colors
- `solarized-dark` - Solarized dark
- `solarized-light` - Solarized light
- `nord` - Nord theme
- `gruvbox` - Gruvbox theme

**Example:**

```ini
[main]
syntax_style = dracula
```

### Custom Colors

You can override individual token colors in the `[colors]` section. Colors are specified in hex format (`#RRGGBB`).

**Available Tokens:**

- `keyword` - SQL keywords (SELECT, FROM, WHERE, etc.)
- `name` - Identifiers (table names, column names, etc.)
- `string` - String literals ('text', "text")
- `number` - Numeric literals (123, 45.67)
- `operator` - Operators (=, >, <, +, -, etc.)
- `comment` - SQL comments (`-- comment`, `/* comment */`)

**Example Custom Theme:**

```ini
[colors]
keyword = #FF69B4  ; Pink keywords
name = #98FB98     ; Pale green identifiers
string = #FFD700   ; Gold strings
number = #87CEEB   ; Sky blue numbers
operator = #FF6347 ; Tomato red operators
comment = #808080  ; Gray comments
```

## Features

### 1. Post-Input Syntax Highlighting

SQL statements are highlighted **after** you press Enter, showing:

- Keywords in your configured color
- Table/column names highlighted
- Strings, numbers, and operators color-coded

**Example:**

```text
MySQL root@127.0.0.1:3306(sakila)> SELECT id, name FROM users WHERE age > 18;
→ SELECT id, name FROM users WHERE age > 18;
  ↑ This line shows with syntax highlighting
```

### 2. AI-Powered EXPLAIN Analysis

Automatically analyze EXPLAIN output using OpenAI GPT models for performance tuning advice.

**Configuration:**

```ini
[main]
ai_analysis = true  # Enable AI analysis (requires OPENAI_API_KEY)
```

**Requirements:**

- Set `OPENAI_API_KEY` environment variable
- Works with `EXPLAIN`, `EXPLAIN FORMAT=JSON`, `EXPLAIN FORMAT=TREE`, etc.

### 3. JSON Export for External Tools

Export EXPLAIN query plans as JSON for use with external visualization tools like pt-visual-explain.

**Configuration:**

```ini
[main]
json_export = true  # Enable JSON export for external tools
```

**Usage:**

- Run `EXPLAIN FORMAT=JSON` queries
- JSON appears in output for piping to external tools
- Compatible with pt-visual-explain and other analysis tools

### 4. Built-in Visual Explain

Create ASCII tree representations of query execution plans without external dependencies.

**Configuration:**

```ini
[main]
visual_explain = true  # Enable built-in visual explain
```

**Features:**

- Tree view of table access patterns
- Shows costs, row estimates, and join hierarchies
- No external tools required
- Works offline

### 5. Improved Fuzzy Completion

The completion system now ranks suggestions by quality:

- **Prefix matches** score highest (typing "SEL" suggests "SELECT" first)
- **Exact case matches** are preferred
- **Compact matches** rank higher (shorter match spans)
- **Position matters** - matches at the start rank higher

**Example:**
Typing "se" suggests in order:

1. `SELECT` (prefix match)
2. `SET` (prefix match)
3. `USER` (contains "se")

### 6. Configurable Colors

Colors are loaded from `~/.go-myclirc` on startup:

- No code changes needed
- Edit config file and restart go-mycli
- Colors apply to all SQL highlighting

## Tips

### Create Custom Themes

1. Copy the default config:

   ```bash
   cp ~/.go-myclirc ~/.go-myclirc.backup
   ```

2. Edit `~/.go-myclirc` with your favorite colors

3. Restart go-mycli to see changes

### Test Colors

Use the built-in test command to see your highlighting:

```sql
SELECT id, name FROM users WHERE age > 18;
```

### Revert to Defaults

Delete your config file to reset to defaults:

```bash
rm ~/.go-myclirc
```

Next time you run go-mycli, it will recreate the default config.

## Troubleshooting

### Colors Not Showing

1. Check terminal supports 256 colors:

   ```bash
   echo $TERM
   # Should show: xterm-256color or similar
   ```

2. Verify config file syntax:

   ```bash
   cat ~/.go-myclirc
   ```

3. Check for INI syntax errors (missing `=`, wrong section names)

### Config Not Loading

1. Ensure file is in home directory:

   ```bash
   ls -la ~/.go-myclirc
   ```

2. Check file permissions:

   ```bash
   chmod 644 ~/.go-myclirc
   ```

3. Look for error messages when starting go-mycli

## Advanced Configuration

### Per-Project Config

While go-mycli reads from `~/.go-myclirc`, you can maintain multiple configs:

```bash
# Save different themes
cp ~/.go-myclirc ~/.go-myclirc-dark
cp ~/.go-myclirc ~/.go-myclirc-light

# Switch themes
cp ~/.go-myclirc-dark ~/.go-myclirc
```

### Color Reference

Common colors in hex format:

- Red: `#FF0000`
- Green: `#00FF00`
- Blue: `#0000FF`
- Yellow: `#FFFF00`
- Cyan: `#00FFFF`
- Magenta: `#FF00FF`
- White: `#FFFFFF`
- Black: `#000000`
- Gray: `#808080`

Use a color picker to find exact hex values for your preferred colors.

## Future Enhancements

Planned configuration options:

- Multi-line mode toggle
- Auto-completion settings
- History size limits
- Output format preferences
- Key binding customization

## See Also

- [Chroma Styles Gallery](https://xyproto.github.io/splash/docs/all.html)
- [SQL Syntax Reference](https://dev.mysql.com/doc/)

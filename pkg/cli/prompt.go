package cli

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/klauspost/compress/zstd"
)

type PromptExecutor struct {
	db                   *sql.DB
	user                 string
	host                 string
	port                 int
	database             string
	buffer               string
	tables               []string
	columns              map[string][]string // table -> columns
	databases            []string
	cacheTime            time.Time
	highlighter          *SyntaxHighlighter
	zstdCompressionLevel int
	nonInteractive       bool // true when reading from pipe/file
	sourceFileMode       bool // true when executing from \. or source command
	enableSuggestions    bool
	enableAIAnalysis     bool // enable AI-powered EXPLAIN analysis
	enableJSONExport     bool // enable JSON export for external tools
	enableVisualExplain  bool // enable built-in visual explain
	aiServerURL          string
	aiServerMode         string
	aiCachePath          string
	aiDetailLevel        string
}

// ExplainNode represents a node in the query execution plan
type ExplainNode struct {
	TableName    string                 `json:"table_name,omitempty"`
	AccessType   string                 `json:"access_type,omitempty"`
	Key          string                 `json:"key,omitempty"`
	KeyLen       interface{}            `json:"key_len,omitempty"`
	Ref          interface{}            `json:"ref,omitempty"`
	Rows         interface{}            `json:"rows,omitempty"`
	Filtered     interface{}            `json:"filtered,omitempty"`
	Extra        interface{}            `json:"extra,omitempty"`
	Partitions   interface{}            `json:"partitions,omitempty"`
	CostInfo     map[string]interface{} `json:"cost_info,omitempty"`
	UsedColumns  []string               `json:"used_columns,omitempty"`
	PossibleKeys []string               `json:"possible_keys,omitempty"`
	QueryBlock   *ExplainNode           `json:"query_block,omitempty"`
	NestedLoop   []interface{}          `json:"nested_loop,omitempty"`
	Table        *ExplainNode           `json:"table,omitempty"`
}

// ExplainPlan represents the root of the query execution plan
type ExplainPlan struct {
	QueryBlock *ExplainNode `json:"query_block"`
}

func (p *PromptExecutor) refreshCache() {
	// Only refresh cache every 30 seconds to avoid too many queries
	if time.Since(p.cacheTime) < 30*time.Second {
		return
	}

	// Get databases
	if rows, err := p.db.Query("SHOW DATABASES"); err == nil {
		p.databases = nil
		for rows.Next() {
			var db string
			if rows.Scan(&db) == nil {
				p.databases = append(p.databases, db)
			}
		}
		rows.Close()
	}

	// Get tables from current database
	if rows, err := p.db.Query("SHOW TABLES"); err == nil {
		p.tables = nil
		for rows.Next() {
			var table string
			if rows.Scan(&table) == nil {
				p.tables = append(p.tables, table)
			}
		}
		rows.Close()
	}

	// Get columns for each table
	p.columns = make(map[string][]string)
	for _, table := range p.tables {
		if rows, err := p.db.Query(fmt.Sprintf("DESCRIBE `%s`", table)); err == nil {
			var columns []string
			for rows.Next() {
				var field sql.NullString
				var typ, null, key sql.NullString
				var def, extra sql.NullString
				if rows.Scan(&field, &typ, &null, &key, &def, &extra) == nil && field.Valid {
					columns = append(columns, field.String)
				}
			}
			p.columns[table] = columns
			rows.Close()
		}
	}

	p.cacheTime = time.Now()
}

// scoredSuggestion holds a suggestion with its match score
type scoredSuggestion struct {
	suggestion prompt.Suggest
	score      int
	matchPos   int
}

// findMatches implements fuzzy matching with quality scoring
func (p *PromptExecutor) findMatches(word string, suggestions []prompt.Suggest) []prompt.Suggest {
	if word == "" {
		return suggestions
	}

	var scored []scoredSuggestion
	wordUpper := strings.ToUpper(word)
	wordLower := strings.ToLower(word)

	// Create fuzzy regex pattern: "k.*?e.*?y" for "key"
	var pattern strings.Builder
	for i, char := range wordUpper {
		if i > 0 {
			pattern.WriteString(".*?")
		}
		pattern.WriteString(regexp.QuoteMeta(string(char)))
	}

	regex, err := regexp.Compile("(?i)" + pattern.String())
	if err != nil {
		// If regex fails, fall back to simple prefix matching
		var matches []prompt.Suggest
		for _, suggestion := range suggestions {
			if strings.HasPrefix(strings.ToUpper(suggestion.Text), wordUpper) {
				matches = append(matches, suggestion)
			}
		}
		return matches
	}

	// Find matches using fuzzy regex with scoring
	for _, suggestion := range suggestions {
		suggUpper := strings.ToUpper(suggestion.Text)

		if match := regex.FindStringIndex(suggUpper); match != nil {
			// Calculate match score (higher is better)
			score := p.calculateMatchScore(wordLower, suggestion.Text, match[0], match[1])

			scored = append(scored, scoredSuggestion{
				suggestion: suggestion,
				score:      score,
				matchPos:   match[0],
			})
		}
	}

	// Sort by score (descending), then by match position (ascending)
	p.sortScoredSuggestions(scored)

	// Extract suggestions
	matches := make([]prompt.Suggest, len(scored))
	for i, s := range scored {
		matches[i] = s.suggestion
	}

	return matches
}

// calculateMatchScore computes a quality score for a match
func (p *PromptExecutor) calculateMatchScore(word, suggestion string, matchStart, matchEnd int) int {
	score := 1000 // Base score

	// Prefer matches at the start
	score -= matchStart * 10

	// Prefer shorter match spans (more compact matches)
	matchLength := matchEnd - matchStart
	score -= matchLength

	// Prefer exact case matches
	wordLower := strings.ToLower(word)
	suggLower := strings.ToLower(suggestion)
	if strings.Contains(suggestion, word) {
		score += 100 // Exact case match
	} else if strings.Contains(suggLower, wordLower) {
		score += 50 // Case-insensitive match
	}

	// Prefer prefix matches
	if strings.HasPrefix(suggLower, wordLower) {
		score += 200
	}

	// Prefer shorter suggestions
	score -= len(suggestion)

	return score
}

// sortScoredSuggestions sorts suggestions by score and position
func (p *PromptExecutor) sortScoredSuggestions(scored []scoredSuggestion) {
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			// Sort by score (descending), then position (ascending)
			if scored[i].score < scored[j].score ||
				(scored[i].score == scored[j].score && scored[i].matchPos > scored[j].matchPos) {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
}

func (p *PromptExecutor) ExecuteSQL(sql string, useVertical bool) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return
	}

	// Handle \G vertical output format (must be done before DESC conversion)
	// Note: useVertical is now passed from the caller, but we still check for \G in case it's embedded
	if len(sql) >= 2 && strings.ToUpper(sql[len(sql)-2:]) == "\\G" {
		useVertical = true
		sql = strings.TrimSpace(sql[:len(sql)-2])
	}

	// Convert DESC to DESCRIBE
	if strings.HasPrefix(strings.ToUpper(sql), "DESC ") {
		// Remove "DESC " (case-insensitive) and trim
		remaining := strings.TrimSpace(sql[5:])
		sql = "DESCRIBE " + remaining
	}

	// Check if it's a query (returns rows) or statement (affects rows)
	// Use Contains instead of HasPrefix to handle comments before the actual SQL
	sqlUpper := strings.ToUpper(sql)
	if strings.Contains(sqlUpper, "SELECT") ||
		strings.HasPrefix(sqlUpper, "DESCRIBE") ||
		strings.HasPrefix(sqlUpper, "DESC") ||
		strings.Contains(sqlUpper, "SHOW") ||
		strings.Contains(sqlUpper, "EXPLAIN") {
		p.executeQuery(sql, useVertical)
	} else {
		p.executeStatement(sql)
	}
}

func (p *PromptExecutor) executeQuery(query string, useVertical bool) {
	start := time.Now()
	rows, err := p.db.Query(query)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		p.maybeSuggestFixedSQL(query, err)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		fmt.Printf("Error getting columns: %v\n", err)
		return
	}

	// Read all rows
	var allRows [][]string
	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		err := rows.Scan(scanArgs...)
		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			return
		}

		row := make([]string, len(columns))
		for i, val := range values {
			if val == nil {
				row[i] = "NULL"
			} else {
				// Handle different types properly
				switch v := val.(type) {
				case []byte:
					row[i] = string(v)
				default:
					row[i] = fmt.Sprintf("%v", v)
				}
			}
		}
		allRows = append(allRows, row)
	}

	if err := rows.Err(); err != nil {
		fmt.Printf("Error iterating rows: %v\n", err)
		return
	}

	// Format output based on \G flag
	var result string
	if useVertical {
		result = formatVerticalTable(columns, allRows)
	} else {
		result = formatMySQLTable(columns, allRows)
	}
	result += fmt.Sprintf("\n%d row%s in set (%.3fs)\n", len(allRows), plural(len(allRows)), elapsed.Seconds())
	fmt.Print(result)

	// Check if this was an EXPLAIN query and AI analysis is enabled
	if p.enableAIAnalysis && isExplainQuery(query) {
		// Capture the EXPLAIN output for AI analysis
		explainOutput := result
		// Note: AI analysis runs synchronously for now to avoid database connection issues
		// In a production version, this could be made asynchronous with proper connection handling
		if err := p.analyzeExplainWithAI(query, explainOutput); err != nil {
			fmt.Printf("AI analysis failed: %v\n", err)
		}
	}

	// Check if user wants to export JSON for external tools or show visual explain
	if (p.enableJSONExport || p.enableVisualExplain) && isExplainQuery(query) {
		var jsonPlan string
		var errJSON error

		// If the user already requested FORMAT=JSON in the query, try to extract it from the current output
		if strings.Contains(strings.ToUpper(query), "FORMAT=JSON") {
			jsonPlan, errJSON = p.extractJSONFromExplainOutput(result)
		} else {
			// User didn't ask for JSON - try to obtain it automatically
			// Prefer the MySQL 8.4+ JSON capture if available
			if p.isMySQL84Plus() {
				jsonPlan, errJSON = p.executeExplainWithJSONCapture(query)
				if errJSON != nil {
					// Fall back to explicit EXPLAIN FORMAT=JSON
					originalQuery, _, _ := extractQueryFromExplain(query)
					var v string
					row := p.db.QueryRow("EXPLAIN FORMAT=JSON " + originalQuery)
					errJSON = row.Scan(&v)
					if errJSON == nil {
						jsonPlan = v
					}
				}
			} else {
				// For older MySQL versions, run EXPLAIN FORMAT=JSON explicitly and read the JSON from the first column
				originalQuery, _, _ := extractQueryFromExplain(query)
				var v string
				row := p.db.QueryRow("EXPLAIN FORMAT=JSON " + originalQuery)
				errJSON = row.Scan(&v)
				if errJSON == nil {
					jsonPlan = v
				}
			}
		}

		// If we did not obtain JSON, show a helpful warning
		if errJSON != nil || jsonPlan == "" {
			fmt.Printf("⚠️  Could not obtain JSON plan for EXPLAIN: %v\n", errJSON)
			fmt.Println("   Tip: Try running EXPLAIN FORMAT=JSON <your query> or enable JSON export in ~/.go-myclirc (json_export=true)")
		} else {
			if p.enableJSONExport {
				fmt.Println("\n📤 JSON Export for External Tools:")
				fmt.Println("==================================")
				fmt.Printf("Raw JSON: %s\n", jsonPlan)
				fmt.Println("\n💡 Tip: Pipe this JSON to tools like pt-visual-explain:")
				fmt.Println("   echo 'raw_json_here' | pt-visual-explain")
			}

			// Show our built-in visual explain if enabled
			if p.enableVisualExplain {
				if visualPlan, err := p.visualExplain(jsonPlan); err == nil {
					fmt.Println("\n🌳 Built-in Visual Explain:")
					fmt.Println("===========================")
					fmt.Print(visualPlan)
				}
			}
		}
	}
}

func (p *PromptExecutor) executeStatement(stmt string) {
	start := time.Now()
	result, err := p.db.Exec(stmt)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		p.maybeSuggestFixedSQL(stmt, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		fmt.Printf("Query OK\nTime: %.3fs\n", elapsed.Seconds())
		return
	}

	fmt.Printf("Query OK, %d row%s affected\nTime: %.3fs\n", rowsAffected, plural(int(rowsAffected)), elapsed.Seconds())
}

func (p *PromptExecutor) Executor(in string) {
	// Handle backslash commands immediately
	in = strings.TrimSpace(in)
	if strings.HasPrefix(in, "\\") {
		switch {
		case in == "\\q", in == "\\quit":
			fmt.Println("Bye")
			os.Exit(0)
		case in == "\\c", in == "\\clear":
			// Clear the buffer and reset
			p.buffer = ""
			return
		case in == "\\h", in == "\\help":
			fmt.Println("MySQL commands:")
			fmt.Println("\\c, \\clear    Clear the current input statement")
			fmt.Println("\\colors       Test syntax highlighting with examples")
			fmt.Println("\\config       Show current syntax highlighting configuration")
			fmt.Println("\\g, \\go       Send command to mysql server")
			fmt.Println("\\h, \\help     Display this help")
			fmt.Println("\\p, \\print    Print current command")
			fmt.Println("\\q, \\quit     Exit mysql")
			fmt.Println("\\r, \\connect  Reconnect to the server")
			fmt.Println("\\s            Display server status")
			fmt.Println("\\u <db>       Use another database. Takes database name as argument")
			fmt.Println("\\. <file>     Execute an SQL script file. Takes a file name as an argument. Supports zstd compressed files")
			fmt.Println("\\! <cmd>      Execute a system shell command")
			fmt.Println("\\suggestions  Toggle suggestions: \"on\" or \"off\"")
			fmt.Println("\\ai           Toggle AI EXPLAIN analysis: \"on\" or \"off\"")
			fmt.Println("\\json         Toggle JSON export for external tools: \"on\" or \"off\"")
			fmt.Println("\\visual       Toggle built-in visual explain: \"on\" or \"off\"")
			return
		case in == "\\s":
			p.showServerStatus()
			return
		case in == "\\config":
			p.showConfig()
			return
		case in == "\\colors", in == "\\test-colors":
			p.testSyntaxHighlighting()
			return
		case strings.HasPrefix(in, "\\suggestions"):
			// Syntax: \suggestions [on|off|toggle]
			parts := strings.Fields(in)
			if len(parts) == 1 {
				// Toggle
				p.enableSuggestions = !p.enableSuggestions
				fmt.Printf("Suggestions now %v\n", p.enableSuggestions)
			} else if len(parts) >= 2 {
				arg := strings.ToLower(parts[1])
				switch arg {
				case "on", "true":
					p.enableSuggestions = true
					fmt.Println("Suggestions enabled")
				case "off", "false":
					p.enableSuggestions = false
					fmt.Println("Suggestions disabled")
				case "toggle":
					p.enableSuggestions = !p.enableSuggestions
					fmt.Printf("Suggestions now %v\n", p.enableSuggestions)
				default:
					fmt.Printf("Unknown argument to \\suggestions: %s\n", arg)
				}
			}

			// Persist change to user config file
			cfg := LoadSyntaxConfig()
			if cfg != nil {
				cfg.EnableSuggestions = p.enableSuggestions
				_ = SaveSyntaxConfig(cfg)
			}
			return
		case strings.HasPrefix(in, "\\ai"):
			// Syntax: \ai [on|off|toggle]
			parts := strings.Fields(in)
			if len(parts) == 1 {
				// Toggle
				p.enableAIAnalysis = !p.enableAIAnalysis
				fmt.Printf("AI analysis now %v\n", p.enableAIAnalysis)
			} else if len(parts) >= 2 {
				arg := strings.ToLower(parts[1])
				switch arg {
				case "on", "true":
					p.enableAIAnalysis = true
					fmt.Println("AI analysis enabled")
				case "off", "false":
					p.enableAIAnalysis = false
					fmt.Println("AI analysis disabled")
				case "toggle":
					p.enableAIAnalysis = !p.enableAIAnalysis
					fmt.Printf("AI analysis now %v\n", p.enableAIAnalysis)
				default:
					fmt.Printf("Unknown argument to \\ai: %s\n", arg)
				}
			}

			// Persist change to user config file
			cfg := LoadSyntaxConfig()
			if cfg != nil {
				cfg.EnableAIAnalysis = p.enableAIAnalysis
				_ = SaveSyntaxConfig(cfg)
			}
			return
		case strings.HasPrefix(in, "\\json"):
			// Syntax: \json [on|off|toggle]
			parts := strings.Fields(in)
			if len(parts) == 1 {
				// Toggle
				p.enableJSONExport = !p.enableJSONExport
				fmt.Printf("JSON export now %v\n", p.enableJSONExport)
			} else if len(parts) >= 2 {
				arg := strings.ToLower(parts[1])
				switch arg {
				case "on", "true":
					p.enableJSONExport = true
					fmt.Println("JSON export enabled")
				case "off", "false":
					p.enableJSONExport = false
					fmt.Println("JSON export disabled")
				case "toggle":
					p.enableJSONExport = !p.enableJSONExport
					fmt.Printf("JSON export now %v\n", p.enableJSONExport)
				default:
					fmt.Printf("Unknown argument to \\json: %s\n", arg)
				}
			}

			// Persist change to user config file
			cfg := LoadSyntaxConfig()
			if cfg != nil {
				cfg.EnableJSONExport = p.enableJSONExport
				_ = SaveSyntaxConfig(cfg)
			}
			return
		case strings.HasPrefix(in, "\\visual"):
			// Syntax: \visual [on|off|toggle]
			parts := strings.Fields(in)
			if len(parts) == 1 {
				// Toggle
				p.enableVisualExplain = !p.enableVisualExplain
				fmt.Printf("Visual explain now %v\n", p.enableVisualExplain)
			} else if len(parts) >= 2 {
				arg := strings.ToLower(parts[1])
				switch arg {
				case "on", "true":
					p.enableVisualExplain = true
					fmt.Println("Visual explain enabled")
				case "off", "false":
					p.enableVisualExplain = false
					fmt.Println("Visual explain disabled")
				case "toggle":
					p.enableVisualExplain = !p.enableVisualExplain
					fmt.Printf("Visual explain now %v\n", p.enableVisualExplain)
				default:
					fmt.Printf("Unknown argument to \\visual: %s\n", arg)
				}
			}

			// Persist change to user config file
			cfg := LoadSyntaxConfig()
			if cfg != nil {
				cfg.EnableVisualExplain = p.enableVisualExplain
				_ = SaveSyntaxConfig(cfg)
			}
			return
		case strings.HasPrefix(in, "\\u "):
			// Extract database name after \u
			dbName := strings.TrimSpace(in[3:])
			if dbName != "" {
				p.switchDatabase(dbName)
			}
			return
		case strings.HasPrefix(in, "\\. "):
			// Extract filename after \.
			fileName := strings.TrimSpace(in[3:])
			if fileName != "" {
				p.sourceFile(fileName)
			}
			return
		case strings.HasPrefix(in, "\\! "):
			// Extract command after \!
			cmd := strings.TrimSpace(in[3:])
			if cmd != "" {
				p.executeSystemCommand(cmd)
			}
			return
		case in == "\\p", in == "\\print":
			p.printCurrentCommand()
			return
		case in == "\\g", in == "\\go":
			// Execute current buffer immediately
			if p.buffer != "" {
				sql := strings.TrimSpace(p.buffer)
				if sql != "" {
					p.ExecuteSQL(sql, false)
					p.buffer = ""
				}
			}
			return
		case in == "\\r", in == "\\connect":
			p.reconnect()
			return
		default:
			fmt.Printf("Unknown command: %s\n", in)
			return
		}
	}

	// Handle regular exit commands
	if in == "exit" || in == "quit" || in == "bye" {
		fmt.Println("Bye")
		os.Exit(0)
	}

	// Handle 'source' command (MySQL compatibility)
	if strings.HasPrefix(strings.ToLower(in), "source ") {
		fileName := strings.TrimSpace(in[7:])
		if fileName != "" {
			p.sourceFile(fileName)
		}
		return
	}

	// Add input to buffer
	p.buffer += in + "\n"

	// Check if buffer contains a complete SQL statement (ends with semicolon or \G)
	statementTerminated := false
	useVertical := false

	if strings.Contains(p.buffer, ";") {
		statementTerminated = true
	} else if strings.Contains(p.buffer, "\\G") {
		statementTerminated = true
		useVertical = true
	}

	if statementTerminated {
		// Extract the SQL statement up to the terminator
		var sql string
		var remaining string

		if strings.Contains(p.buffer, ";") {
			parts := strings.SplitN(p.buffer, ";", 2)
			sql = strings.TrimSpace(parts[0])
			remaining = strings.TrimSpace(parts[1])
		} else if strings.Contains(p.buffer, "\\G") {
			parts := strings.SplitN(p.buffer, "\\G", 2)
			sql = strings.TrimSpace(parts[0])
			remaining = strings.TrimSpace(parts[1])
		}

		if sql != "" {
			// In non-interactive mode (piped input), print the SQL statement before executing (like mysql -vvv)
			// But NOT when executing from source/\. commands (sourceFileMode)
			if p.nonInteractive && !p.sourceFileMode {
				fmt.Println("--------------")
				fmt.Println(sql)
				fmt.Println("--------------")
				fmt.Println()
			} else if !p.nonInteractive && !p.sourceFileMode {
				p.printHighlightedSQL(sql)
			}
			p.ExecuteSQL(sql, useVertical)
		} // Keep any remaining content after the terminator
		p.buffer = remaining
	}
}

func (p *PromptExecutor) Completer(in prompt.Document) []prompt.Suggest {
	// Refresh cache if needed - force refresh if cache is empty
	if time.Since(p.cacheTime) > 30*time.Second || len(p.tables) == 0 {
		p.refreshCache()
	}

	// Get the current line and word
	line := in.CurrentLine()
	word := in.GetWordBeforeCursor()
	cursorPos := in.CursorPositionCol()

	// Parse SQL context using the smart parser
	ctx := ParseSQLContext(line, cursorPos)

	// Only show suggestions when user is actually typing or has specific SQL context
	lineTrimmed := strings.TrimSpace(line)
	if word == "" && lineTrimmed == "" {
		return nil // Return empty suggestions for empty lines
	}

	// Check if we should show suggestions on empty word (after keywords like FROM, WHERE, etc.)
	showContextOnEmptyWord := p.shouldShowContextSuggestions(lineTrimmed, ctx)
	if word == "" && !showContextOnEmptyWord {
		return nil
	}

	// Build suggestions based on parsed context
	suggestions := p.buildContextAwareSuggestions(ctx)

	// Filter suggestions based on current word being typed
	if word != "" {
		suggestions = p.findMatches(strings.ToLower(word), suggestions)
	}

	return suggestions
}

// shouldShowContextSuggestions determines if we should show suggestions even without a typed word
func (p *PromptExecutor) shouldShowContextSuggestions(line string, ctx *SQLParseResult) bool {
	if line == "" {
		return false
	}

	lineUpper := strings.ToUpper(line)

	// Show suggestions after these keywords
	contextTriggers := []string{
		" FROM", " JOIN", " WHERE", " HAVING", " ON",
		" ORDER BY", " GROUP BY", " USE", " SHOW",
		" SET", " INTO", " UPDATE", " VALUES",
	}

	for _, trigger := range contextTriggers {
		if strings.HasSuffix(lineUpper, trigger) {
			return true
		}
	}

	// Show suggestions after table. pattern (for column completion)
	if ctx.Context == ContextTableDot {
		return true
	}

	// Show after comma in SELECT or FROM clauses
	if ctx.AfterComma && (ctx.HasFrom || strings.Contains(lineUpper, "SELECT")) {
		return true
	}

	return false
}

// buildContextAwareSuggestions builds suggestions based on the parsed SQL context
func (p *PromptExecutor) buildContextAwareSuggestions(ctx *SQLParseResult) []prompt.Suggest {
	var suggestions []prompt.Suggest

	switch ctx.Context {
	case ContextTableDot:
		// After "table." - show only columns from that specific table
		suggestions = p.getColumnsForTable(ctx.CurrentTable, ctx)

	case ContextTable:
		// After FROM, JOIN, UPDATE, INTO - show tables and databases
		suggestions = p.getTableSuggestions()

	case ContextColumn:
		// After SELECT, WHERE, ORDER BY, GROUP BY - show columns from query tables
		suggestions = p.getColumnSuggestions(ctx)

	case ContextJoinOn:
		// After JOIN ... ON - show columns that are likely join keys
		suggestions = p.getJoinColumnSuggestions(ctx)

	case ContextDatabase:
		// After USE - show databases
		suggestions = p.getDatabaseSuggestions()

	case ContextShowItem:
		// After SHOW - show SHOW options
		suggestions = p.getShowItemSuggestions()

	case ContextAlias:
		// After table name - could be alias or next keyword
		// Don't suggest anything specific, let user type alias or keyword

	case ContextValue:
		// After operator - user is typing a value, minimal suggestions
		// Could suggest NULL, TRUE, FALSE, or common values

	case ContextKeyword:
		// Start of statement or after AND/OR - show all keywords
		suggestions = p.getKeywordSuggestions()

	default:
		// Fallback to keyword suggestions
		suggestions = p.getKeywordSuggestions()
	}

	return suggestions
}

// getColumnsForTable returns columns for a specific table (handles aliases)
func (p *PromptExecutor) getColumnsForTable(tableName string, ctx *SQLParseResult) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Resolve alias to actual table name
	resolvedTable := ctx.ResolveAlias(tableName)

	// Try exact match first
	if cols, exists := p.columns[resolvedTable]; exists {
		for _, col := range cols {
			suggestions = append(suggestions, prompt.Suggest{
				Text:        col,
				Description: fmt.Sprintf("Column from %s", resolvedTable),
			})
		}
		return suggestions
	}

	// Try case-insensitive match
	for table, cols := range p.columns {
		if strings.EqualFold(table, resolvedTable) {
			for _, col := range cols {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        col,
					Description: fmt.Sprintf("Column from %s", table),
				})
			}
			return suggestions
		}
	}

	return suggestions
}

// getTableSuggestions returns table and database suggestions
func (p *PromptExecutor) getTableSuggestions() []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Add tables first (higher priority)
	for _, table := range p.tables {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        table,
			Description: "Table",
		})
	}

	// Add databases for schema-qualified references
	for _, db := range p.databases {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        db,
			Description: "Database",
		})
	}

	return suggestions
}

// getColumnSuggestions returns column suggestions based on tables in the query
func (p *PromptExecutor) getColumnSuggestions(ctx *SQLParseResult) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Add * for SELECT context
	if ctx.LastKeyword == "SELECT" || (!ctx.HasWhere && !ctx.HasOrderBy && !ctx.HasGroupBy && !ctx.HasHaving) {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        "*",
			Description: "All columns",
		})
	}

	// If we have identified tables in the query, show their columns
	if len(ctx.Tables) > 0 {
		seenColumns := make(map[string]bool)

		for _, tableName := range ctx.Tables {
			// Check if there's an alias for this table
			tableAlias := ""
			for _, alias := range ctx.Aliases {
				if alias.TableName == tableName {
					tableAlias = alias.Alias
					break
				}
			}

			if cols, exists := p.columns[tableName]; exists {
				for _, col := range cols {
					// Avoid duplicate column names
					if seenColumns[col] {
						// If duplicate, use qualified name
						qualifiedName := tableName + "." + col
						if tableAlias != "" {
							qualifiedName = tableAlias + "." + col
						}
						suggestions = append(suggestions, prompt.Suggest{
							Text:        qualifiedName,
							Description: fmt.Sprintf("Column from %s", tableName),
						})
					} else {
						seenColumns[col] = true
						suggestions = append(suggestions, prompt.Suggest{
							Text:        col,
							Description: fmt.Sprintf("Column from %s", tableName),
						})
					}
				}
			}
		}

		// Also add table-qualified columns for explicit references
		for _, tableName := range ctx.Tables {
			tableAlias := ""
			for _, alias := range ctx.Aliases {
				if alias.TableName == tableName {
					tableAlias = alias.Alias
					break
				}
			}

			prefix := tableName
			if tableAlias != "" {
				prefix = tableAlias
			}

			suggestions = append(suggestions, prompt.Suggest{
				Text:        prefix + ".",
				Description: fmt.Sprintf("Columns from %s", tableName),
			})
		}
	} else {
		// No tables identified yet - show tables to help complete FROM clause
		if !ctx.HasFrom {
			for _, table := range p.tables {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        table,
					Description: "Table (add FROM clause)",
				})
			}
		} else {
			// Has FROM but we couldn't parse table names - show all columns
			for table, cols := range p.columns {
				for _, col := range cols {
					suggestions = append(suggestions, prompt.Suggest{
						Text:        col,
						Description: fmt.Sprintf("Column from %s", table),
					})
				}
			}
		}
	}

	// Add functions in SELECT context
	if !ctx.HasFrom || ctx.LastKeyword == "SELECT" {
		suggestions = append(suggestions, p.getFunctionSuggestions()...)
	}

	return suggestions
}

// getJoinColumnSuggestions returns column suggestions optimized for JOIN ON clauses
func (p *PromptExecutor) getJoinColumnSuggestions(ctx *SQLParseResult) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// In JOIN ON context, prefer columns that might be foreign keys (ending with _id or named id)
	for _, tableName := range ctx.Tables {
		tableAlias := ""
		for _, alias := range ctx.Aliases {
			if alias.TableName == tableName {
				tableAlias = alias.Alias
				break
			}
		}

		prefix := tableName
		if tableAlias != "" {
			prefix = tableAlias
		}

		if cols, exists := p.columns[tableName]; exists {
			// First add likely join columns (id, *_id)
			for _, col := range cols {
				colLower := strings.ToLower(col)
				if col == "id" || strings.HasSuffix(colLower, "_id") {
					suggestions = append(suggestions, prompt.Suggest{
						Text:        prefix + "." + col,
						Description: fmt.Sprintf("Join key from %s", tableName),
					})
				}
			}

			// Then add other columns
			for _, col := range cols {
				colLower := strings.ToLower(col)
				if col != "id" && !strings.HasSuffix(colLower, "_id") {
					suggestions = append(suggestions, prompt.Suggest{
						Text:        prefix + "." + col,
						Description: fmt.Sprintf("Column from %s", tableName),
					})
				}
			}
		}
	}

	return suggestions
}

// getDatabaseSuggestions returns database suggestions
func (p *PromptExecutor) getDatabaseSuggestions() []prompt.Suggest {
	var suggestions []prompt.Suggest
	for _, db := range p.databases {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        db,
			Description: "Database",
		})
	}
	return suggestions
}

// getShowItemSuggestions returns SHOW command options
func (p *PromptExecutor) getShowItemSuggestions() []prompt.Suggest {
	return []prompt.Suggest{
		{Text: "DATABASES", Description: "Show databases"},
		{Text: "TABLES", Description: "Show tables"},
		{Text: "COLUMNS FROM", Description: "Show columns from table"},
		{Text: "INDEX FROM", Description: "Show index from table"},
		{Text: "CREATE TABLE", Description: "Show create table"},
		{Text: "CREATE VIEW", Description: "Show create view"},
		{Text: "CREATE DATABASE", Description: "Show create database"},
		{Text: "GRANTS", Description: "Show grants"},
		{Text: "GRANTS FOR", Description: "Show grants for user"},
		{Text: "PROCESSLIST", Description: "Show process list"},
		{Text: "FULL PROCESSLIST", Description: "Show full process list"},
		{Text: "VARIABLES", Description: "Show variables"},
		{Text: "GLOBAL VARIABLES", Description: "Show global variables"},
		{Text: "SESSION VARIABLES", Description: "Show session variables"},
		{Text: "STATUS", Description: "Show status"},
		{Text: "GLOBAL STATUS", Description: "Show global status"},
		{Text: "SESSION STATUS", Description: "Show session status"},
		{Text: "ENGINE", Description: "Show engine status"},
		{Text: "ENGINES", Description: "Show storage engines"},
		{Text: "MASTER STATUS", Description: "Show master status"},
		{Text: "SLAVE STATUS", Description: "Show slave status"},
		{Text: "REPLICA STATUS", Description: "Show replica status"},
		{Text: "BINARY LOGS", Description: "Show binary logs"},
		{Text: "BINLOG EVENTS", Description: "Show binlog events"},
		{Text: "WARNINGS", Description: "Show warnings"},
		{Text: "ERRORS", Description: "Show errors"},
		{Text: "TABLE STATUS", Description: "Show table status"},
		{Text: "OPEN TABLES", Description: "Show open tables"},
		{Text: "TRIGGERS", Description: "Show triggers"},
		{Text: "EVENTS", Description: "Show events"},
		{Text: "PLUGINS", Description: "Show plugins"},
		{Text: "PRIVILEGES", Description: "Show privileges"},
		{Text: "PROFILES", Description: "Show profiles"},
		{Text: "PROFILE", Description: "Show profile"},
		{Text: "COLLATION", Description: "Show collation"},
		{Text: "CHARACTER SET", Description: "Show character sets"},
	}
}

// getKeywordSuggestions returns SQL keyword suggestions
func (p *PromptExecutor) getKeywordSuggestions() []prompt.Suggest {
	// Combine MySQL keywords with common starting statements
	suggestions := make([]prompt.Suggest, 0, len(MySQLKeywords)+10)

	// Add common statement starters first
	starters := []prompt.Suggest{
		{Text: "SELECT", Description: "Query data"},
		{Text: "INSERT INTO", Description: "Insert data"},
		{Text: "UPDATE", Description: "Update data"},
		{Text: "DELETE FROM", Description: "Delete data"},
		{Text: "CREATE TABLE", Description: "Create table"},
		{Text: "ALTER TABLE", Description: "Alter table"},
		{Text: "DROP TABLE", Description: "Drop table"},
		{Text: "SHOW", Description: "Show information"},
		{Text: "DESCRIBE", Description: "Describe table"},
		{Text: "EXPLAIN", Description: "Explain query"},
		{Text: "USE", Description: "Use database"},
	}
	suggestions = append(suggestions, starters...)
	suggestions = append(suggestions, MySQLKeywords...)

	return suggestions
}

// getFunctionSuggestions returns SQL function suggestions
func (p *PromptExecutor) getFunctionSuggestions() []prompt.Suggest {
	return []prompt.Suggest{
		// Aggregate functions
		{Text: "COUNT(", Description: "Count rows"},
		{Text: "SUM(", Description: "Sum values"},
		{Text: "AVG(", Description: "Average value"},
		{Text: "MIN(", Description: "Minimum value"},
		{Text: "MAX(", Description: "Maximum value"},
		{Text: "GROUP_CONCAT(", Description: "Concatenate group"},

		// String functions
		{Text: "CONCAT(", Description: "Concatenate strings"},
		{Text: "SUBSTRING(", Description: "Extract substring"},
		{Text: "LENGTH(", Description: "String length"},
		{Text: "UPPER(", Description: "Uppercase"},
		{Text: "LOWER(", Description: "Lowercase"},
		{Text: "TRIM(", Description: "Trim whitespace"},
		{Text: "REPLACE(", Description: "Replace string"},
		{Text: "LEFT(", Description: "Left substring"},
		{Text: "RIGHT(", Description: "Right substring"},

		// Date functions
		{Text: "NOW()", Description: "Current timestamp"},
		{Text: "CURDATE()", Description: "Current date"},
		{Text: "CURTIME()", Description: "Current time"},
		{Text: "DATE(", Description: "Extract date"},
		{Text: "YEAR(", Description: "Extract year"},
		{Text: "MONTH(", Description: "Extract month"},
		{Text: "DAY(", Description: "Extract day"},
		{Text: "DATE_FORMAT(", Description: "Format date"},
		{Text: "DATE_ADD(", Description: "Add to date"},
		{Text: "DATE_SUB(", Description: "Subtract from date"},
		{Text: "DATEDIFF(", Description: "Date difference"},
		{Text: "TIMESTAMPDIFF(", Description: "Timestamp difference"},

		// Numeric functions
		{Text: "ROUND(", Description: "Round number"},
		{Text: "FLOOR(", Description: "Floor value"},
		{Text: "CEIL(", Description: "Ceiling value"},
		{Text: "ABS(", Description: "Absolute value"},
		{Text: "MOD(", Description: "Modulo"},

		// Control flow
		{Text: "IF(", Description: "If condition"},
		{Text: "IFNULL(", Description: "If null"},
		{Text: "NULLIF(", Description: "Null if equal"},
		{Text: "COALESCE(", Description: "First non-null"},
		{Text: "CASE", Description: "Case expression"},

		// JSON functions
		{Text: "JSON_EXTRACT(", Description: "Extract JSON"},
		{Text: "JSON_UNQUOTE(", Description: "Unquote JSON"},
		{Text: "JSON_OBJECT(", Description: "Create JSON object"},
		{Text: "JSON_ARRAY(", Description: "Create JSON array"},

		// Window functions
		{Text: "ROW_NUMBER() OVER (", Description: "Row number"},
		{Text: "RANK() OVER (", Description: "Rank"},
		{Text: "DENSE_RANK() OVER (", Description: "Dense rank"},
		{Text: "LAG(", Description: "Previous row value"},
		{Text: "LEAD(", Description: "Next row value"},

		// Conversion
		{Text: "CAST(", Description: "Cast type"},
		{Text: "CONVERT(", Description: "Convert type"},

		// Other
		{Text: "DISTINCT", Description: "Distinct values"},
		{Text: "AS", Description: "Alias"},
	}
}

func (p *PromptExecutor) ExitChecker(in string, breakline bool) bool {
	return false // Not used anymore, we exit directly with os.Exit()
}

// livePrefix returns the current prompt prefix
func (p *PromptExecutor) livePrefix() (string, bool) {
	var dbPart string
	if p.database != "" {
		dbPart = fmt.Sprintf("(%s)", p.database)
	}
	return fmt.Sprintf("MySQL %s@%s:%d%s> ", p.user, p.host, p.port, dbPart), true
}

// formatMySQLTable formats data in classic MySQL table style
func formatMySQLTable(columns []string, rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	// Calculate column widths
	colWidths := make([]int, len(columns))
	for i, col := range columns {
		colWidths[i] = len(col)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// Create separator line
	var sep strings.Builder
	sep.WriteString("+")
	for _, width := range colWidths {
		sep.WriteString(strings.Repeat("-", width+2))
		sep.WriteString("+")
	}
	sepLine := sep.String()

	// Format header
	var result strings.Builder
	result.WriteString(sepLine)
	result.WriteString("\n|")
	for i, col := range columns {
		result.WriteString(fmt.Sprintf(" %-*s |", colWidths[i], col))
	}
	result.WriteString("\n")
	result.WriteString(sepLine)

	// Format rows
	for _, row := range rows {
		result.WriteString("\n|")
		for i, cell := range row {
			result.WriteString(fmt.Sprintf(" %-*s |", colWidths[i], cell))
		}
	}
	result.WriteString("\n")
	result.WriteString(sepLine)

	return result.String()
}

// formatVerticalTable formats data in MySQL vertical format (\G)
func formatVerticalTable(columns []string, rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	var result strings.Builder
	for i, row := range rows {
		result.WriteString(fmt.Sprintf("*************************** %d. row ***************************\n", i+1))
		for j, col := range columns {
			value := row[j]
			if value == "" {
				value = "(NULL)"
			}
			// Highlight specific column names in neon green
			colDisplay := col
			if col == "Table" || col == "Create Table" || col == "Database" || col == "View" || col == "Create View" {
				colDisplay = fmt.Sprintf("\033[92m%s\033[0m", col)
			}
			result.WriteString(fmt.Sprintf("%s: %s\n", colDisplay, value))
		}
		if i < len(rows)-1 {
			result.WriteString("\n")
		}
	}
	return result.String()
}

// plural returns "s" if n != 1, empty string otherwise
func plural(n int) string {
	if n != 1 {
		return "s"
	}
	return ""
}

// StartPrompt starts the interactive MySQL prompt
func StartPrompt(db *sql.DB, user, host string, port int, database string, zstdCompressionLevel int, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel string) error {
	// Create default config file if it doesn't exist
	_ = SaveDefaultSyntaxConfig()

	// Print initial connection info like mycli
	fmt.Println("go-mycli 0.1.0")

	// Check if stdin is a terminal (interactive mode)
	stat, err := os.Stdin.Stat()
	isTerminal := err == nil && (stat.Mode()&os.ModeCharDevice) != 0

	if !isTerminal {
		// Non-interactive mode: read from stdin line by line
		return runNonInteractive(db, user, host, port, database, zstdCompressionLevel, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel)
	}

	// Use go-prompt for interactive mode with syntax highlighting
	return startGoPrompt(db, user, host, port, database, zstdCompressionLevel, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel)
}

// startGoPrompt starts the go-prompt-based prompt with syntax highlighting
func startGoPrompt(db *sql.DB, user, host string, port int, database string, zstdCompressionLevel int, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel string) error {
	// Load syntax config and use it to set suggestion toggle
	cfg := LoadSyntaxConfig()

	// Create executor
	if aiServerURL == "" {
		aiServerURL = cfg.AiServerURL
	}
	if aiServerMode == "" {
		aiServerMode = cfg.AiServerMode
	}
	if aiCachePath == "" {
		aiCachePath = cfg.AiCachePath
	}
	executor := &PromptExecutor{
		db:                   db,
		user:                 user,
		host:                 host,
		port:                 port,
		database:             database,
		buffer:               "",
		tables:               nil,
		columns:              make(map[string][]string),
		databases:            nil,
		cacheTime:            time.Time{},
		highlighter:          NewSyntaxHighlighter(),
		enableSuggestions:    cfg.EnableSuggestions,
		enableAIAnalysis:     cfg.EnableAIAnalysis,
		enableJSONExport:     cfg.EnableJSONExport,
		enableVisualExplain:  cfg.EnableVisualExplain,
		zstdCompressionLevel: zstdCompressionLevel,
		aiServerURL:          aiServerURL,
		aiServerMode:         aiServerMode,
		aiCachePath:          aiCachePath,
		aiDetailLevel:        aiDetailLevel,
	}

	// Create go-prompt instance with syntax highlighting
	p := prompt.New(
		executor.Executor,
		executor.Completer,
		prompt.OptionLivePrefix(executor.livePrefix),
		prompt.OptionTitle("go-mycli"),
	)

	// Run the prompt
	p.Run()
	return nil
}

func runNonInteractive(db *sql.DB, user, host string, port int, database string, zstdCompressionLevel int, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel string) error {
	cfg := LoadSyntaxConfig()
	if aiServerURL == "" {
		aiServerURL = cfg.AiServerURL
	}
	if aiServerMode == "" {
		aiServerMode = cfg.AiServerMode
	}
	if aiCachePath == "" {
		aiCachePath = cfg.AiCachePath
	}
	executor := &PromptExecutor{
		db:                   db,
		user:                 user,
		host:                 host,
		port:                 port,
		database:             database,
		buffer:               "",
		tables:               nil,
		columns:              make(map[string][]string),
		databases:            nil,
		cacheTime:            time.Time{},
		highlighter:          NewSyntaxHighlighter(),
		enableSuggestions:    cfg.EnableSuggestions,
		enableAIAnalysis:     cfg.EnableAIAnalysis,
		enableJSONExport:     cfg.EnableJSONExport,
		enableVisualExplain:  cfg.EnableVisualExplain,
		zstdCompressionLevel: zstdCompressionLevel,
		aiServerURL:          aiServerURL,
		aiServerMode:         aiServerMode,
		aiCachePath:          aiCachePath,
		aiDetailLevel:        aiDetailLevel,
		nonInteractive:       true,
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		// Process the line through the normal Executor
		// This will accumulate multi-line statements in the buffer
		// and execute them when a terminator (; or \G) is found
		executor.Executor(line)
	}

	// After all input is processed, if there's still content in buffer without terminator,
	// execute it (for cases where the last statement doesn't have a semicolon)
	if executor.buffer != "" {
		sql := strings.TrimSpace(executor.buffer)
		if sql != "" {
			executor.ExecuteSQL(sql, false)
			executor.buffer = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// showConfig displays the current syntax highlighting configuration
func (p *PromptExecutor) showConfig() {
	config := LoadSyntaxConfig()
	if config == nil {
		fmt.Println("No configuration loaded (using defaults)")
		return
	}

	fmt.Println("Current Syntax Highlighting Configuration:")
	fmt.Println("----------------------------------------")
	fmt.Printf("Style: %s\n", config.Style)
	fmt.Printf("Custom Colors: %d\n", len(config.Colors))
	fmt.Printf("Suggestions enabled: %v\n", config.EnableSuggestions)
	fmt.Printf("AI analysis enabled: %v\n", config.EnableAIAnalysis)
	fmt.Printf("JSON export enabled: %v\n", config.EnableJSONExport)
	fmt.Printf("Visual explain enabled: %v\n", config.EnableVisualExplain)

	if len(config.Colors) > 0 {
		fmt.Println("\nColor Overrides:")
		for token, color := range config.Colors {
			fmt.Printf("  %s: %s\n", token, color)
		}
	}

	fmt.Println("\nConfig file: ~/.go-myclirc")
}

// testSyntaxHighlighting displays a sample query with syntax highlighting
func (p *PromptExecutor) testSyntaxHighlighting() {
	if p.highlighter == nil {
		fmt.Println("Syntax highlighting is not available")
		return
	}

	fmt.Println("Syntax Highlighting Test")
	fmt.Println("========================")
	fmt.Println()

	testQueries := []string{
		"SELECT * FROM users WHERE age > 18",
		"SELECT COUNT(*), AVG(salary) FROM employees GROUP BY department",
		"UPDATE products SET price = 99.99 WHERE category = 'electronics'",
		"INSERT INTO orders (user_id, total, status) VALUES (1, 150.00, 'pending')",
		"DELETE FROM sessions WHERE created_at < '2024-01-01'",
		"CREATE TABLE customers (id INT PRIMARY KEY, name VARCHAR(100), email VARCHAR(255))",
	}

	for i, query := range testQueries {
		fmt.Printf("Example %d:\n", i+1)
		fmt.Println(p.highlighter.HighlightSQL(query))
		fmt.Println()
	}

	fmt.Println("Color Legend:")
	fmt.Println("  Keywords (SELECT, FROM, WHERE): Cyan")
	fmt.Println("  Functions (COUNT, AVG, SUM):    Orange")
	fmt.Println("  Table/Column names:             Green")
	fmt.Println("  String literals:                Yellow")
	fmt.Println("  Numbers:                        Purple")
	fmt.Println("  Operators (=, >, <):            Pink")
	fmt.Println()
	fmt.Println("To customize colors, edit ~/.go-myclirc")
}

// showServerStatus displays server status information like mycli
func (p *PromptExecutor) showServerStatus() {
	fmt.Println("--------------")

	// Get connection ID
	var connectionID int
	err := p.db.QueryRow("SELECT CONNECTION_ID()").Scan(&connectionID)
	if err != nil {
		fmt.Printf("Connection id:\t\t<error: %v>\n", err)
	} else {
		fmt.Printf("Connection id:\t\t%d\n", connectionID)
	}

	// Current database
	fmt.Printf("Current database:\t%s\n", p.database)

	// Current user
	fmt.Printf("Current user:\t\t%s@%s\n", p.user, p.host)

	// Current pager (we don't support pager yet)
	fmt.Println("Current pager:\t\tstdout")

	// Server version
	var version string
	err = p.db.QueryRow("SELECT VERSION()").Scan(&version)
	if err != nil {
		fmt.Printf("Server version:\t\t<error: %v>\n", err)
	} else {
		fmt.Printf("Server version:\t\t%s\n", version)
	}

	// Protocol version (hardcoded for MySQL)
	fmt.Println("Protocol version:\t10")

	// Connection type
	fmt.Printf("Connection:\t\t%s via TCP/IP\n", p.host)

	// Server character set
	var serverCharset string
	err = p.db.QueryRow("SELECT @@character_set_server").Scan(&serverCharset)
	if err != nil {
		fmt.Printf("Server characterset:\t<error: %v>\n", err)
	} else {
		fmt.Printf("Server characterset:\t%s\n", serverCharset)
	}

	// Database character set
	var dbCharset string
	err = p.db.QueryRow("SELECT @@character_set_database").Scan(&dbCharset)
	if err != nil {
		fmt.Printf("Db characterset:\t\t<error: %v>\n", err)
	} else {
		fmt.Printf("Db characterset:\t\t%s\n", dbCharset)
	}

	// Client character set
	var clientCharset string
	err = p.db.QueryRow("SELECT @@character_set_client").Scan(&clientCharset)
	if err != nil {
		fmt.Printf("Client characterset:\t<error: %v>\n", err)
	} else {
		fmt.Printf("Client characterset:\t%s\n", clientCharset)
	}

	// Connection character set
	var connCharset string
	err = p.db.QueryRow("SELECT @@character_set_connection").Scan(&connCharset)
	if err != nil {
		fmt.Printf("Conn. characterset:\t<error: %v>\n", err)
	} else {
		fmt.Printf("Conn. characterset:\t%s\n", connCharset)
	}

	// TCP port
	fmt.Printf("TCP port:\t\t%d\n", p.port)

	// Uptime
	var uptime int
	err = p.db.QueryRow("SELECT @@uptime").Scan(&uptime)
	if err != nil {
		fmt.Printf("Uptime:\t\t\t<error: %v>\n", err)
	} else {
		days := uptime / 86400
		hours := (uptime % 86400) / 3600
		minutes := (uptime % 3600) / 60
		seconds := uptime % 60
		fmt.Printf("Uptime:\t\t\t%d day%s %02d hour%s %02d min %02d sec\n",
			days, plural(days), hours, plural(hours), minutes, seconds)
	}

	fmt.Println("--------------")

	// Show connection statistics
	var threadsConnected, queries, slowQueries, opens, flushTables, openTables, queriesPerSecondAvg float64
	err = p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'Threads_connected'").Scan(&sql.NullString{}, &threadsConnected)
	if err == nil {
		fmt.Printf("Threads: %d  ", int(threadsConnected))
	}

	err = p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'Queries'").Scan(&sql.NullString{}, &queries)
	if err == nil {
		fmt.Printf("Queries: %d  ", int(queries))
	}

	err = p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'Slow_queries'").Scan(&sql.NullString{}, &slowQueries)
	if err == nil {
		fmt.Printf("Slow queries: %d  ", int(slowQueries))
	}

	err = p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'Opened_tables'").Scan(&sql.NullString{}, &opens)
	if err == nil {
		fmt.Printf("Opens: %d  ", int(opens))
	}

	err = p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'Flush_commands'").Scan(&sql.NullString{}, &flushTables)
	if err == nil {
		fmt.Printf("Flush tables: %d  ", int(flushTables))
	}

	err = p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'Open_tables'").Scan(&sql.NullString{}, &openTables)
	if err == nil {
		fmt.Printf("Open tables: %d  ", int(openTables))
	}

	err = p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'Queries'").Scan(&sql.NullString{}, &queries)
	if err == nil && uptime > 0 {
		queriesPerSecondAvg = queries / float64(uptime)
		fmt.Printf("Queries per second avg: %.6f", queriesPerSecondAvg)
	}

	fmt.Println()
}

// switchDatabase switches to a different database
func (p *PromptExecutor) switchDatabase(dbName string) {
	// Execute USE statement
	_, err := p.db.Exec("USE " + dbName)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Update the current database
	p.database = dbName
	fmt.Printf("You are now connected to database \"%s\" as user \"%s\"\n", dbName, p.user)
}

// sourceFile executes an SQL script file
func (p *PromptExecutor) sourceFile(fileName string) {
	// Read the file
	content, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Printf("Error opening file '%s': %v\n", fileName, err)
		return
	}

	// Check if file is zstd compressed by checking the magic number
	// zstd files start with 0x28, 0xB5, 0x2F, 0xFD
	isCompressed := len(content) >= 4 &&
		content[0] == 0x28 && content[1] == 0xB5 &&
		content[2] == 0x2F && content[3] == 0xFD

	var sqlContent []byte
	if isCompressed {
		// Decompress the content
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			fmt.Printf("Error creating zstd decoder: %v\n", err)
			return
		}
		defer decoder.Close()

		sqlContent, err = decoder.DecodeAll(content, nil)
		if err != nil {
			fmt.Printf("Error decompressing file '%s': %v\n", fileName, err)
			return
		}
		fmt.Printf("Decompressed zstd file '%s'\n", fileName)
	} else {
		// File is not compressed, use as-is
		sqlContent = content
	}

	// Set source file mode to suppress SQL statement printing (like MySQL client)
	oldSourceFileMode := p.sourceFileMode
	p.sourceFileMode = true
	defer func() { p.sourceFileMode = oldSourceFileMode }()

	// Process the SQL content line by line through the normal executor
	// This properly handles multi-line statements, comments, and terminators
	scanner := bufio.NewScanner(strings.NewReader(string(sqlContent)))
	for scanner.Scan() {
		line := scanner.Text()
		p.Executor(line)
	}

	// Execute any remaining buffered content
	if p.buffer != "" {
		sql := strings.TrimSpace(p.buffer)
		if sql != "" {
			p.ExecuteSQL(sql, false)
			p.buffer = ""
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file content: %v\n", err)
	}
}

// executeSystemCommand executes a system shell command
func (p *PromptExecutor) executeSystemCommand(cmd string) {
	// Execute the command
	output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing command: %v\n", err)
		return
	}

	// Print the output
	fmt.Print(string(output))
}

// printCurrentCommand prints the current command buffer
func (p *PromptExecutor) printCurrentCommand() {
	if p.buffer == "" {
		fmt.Println("No current command")
	} else {
		fmt.Println(p.buffer)
	}
}

// maybeSuggestFixedSQL prints a friendly suggestion when MySQL reports a syntax error.
// This is intentionally simple and only handles very common mistakes.
func (p *PromptExecutor) maybeSuggestFixedSQL(sql string, err error) {
	if !p.enableSuggestions {
		return
	}
	// Call the shared suggester to get a suggestion string (or empty if none)
	suggestion := SuggestFixedSQL(p, sql, err)
	if suggestion == "" {
		return
	}
	fmt.Println()
	fmt.Println("Did you mean:")
	fmt.Printf("  %s\n", suggestion)
}

// printHighlightedSQL renders the command with syntax highlighting ahead of execution (interactive only)
func (p *PromptExecutor) printHighlightedSQL(sql string) {
	if p.highlighter == nil {
		fmt.Println(sql)
		return
	}

	highlighted := p.highlighter.HighlightSQL(sql)
	fmt.Printf("→ %s\n\n", highlighted)
}

// reconnect reconnects to the MySQL server
func (p *PromptExecutor) reconnect() {
	// Close current connection if present
	if p.db != nil {
		_ = p.db.Close()
	}

	// Reconnect using the same parameters
	config, err := ReadMySQLConfig("", "")
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		return
	}

	mergedConfig := MergeConfig(config, p.user, "", p.host, p.port, "", p.database)
	dsn := BuildDSN(mergedConfig.User, mergedConfig.Password, mergedConfig.Host, mergedConfig.Port, mergedConfig.Database, mergedConfig.Socket, p.zstdCompressionLevel)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}

	// Test connection
	if err := db.Ping(); err != nil {
		fmt.Printf("Error pinging database: %v\n", err)
		_ = db.Close()
		return
	}

	// Update the database connection
	p.db = db

	// Clear caches
	p.tables = nil
	p.columns = make(map[string][]string)
	p.databases = nil
	p.cacheTime = time.Time{}

	fmt.Println("Connection reestablished")
}

// extractJSONFromExplainOutput extracts JSON from EXPLAIN FORMAT=JSON output
func (p *PromptExecutor) extractJSONFromExplainOutput(output string) (string, error) {
	// Look for JSON content in the output
	// For EXPLAIN FORMAT=JSON, the JSON is typically in the first column of the first row
	// or spread across multiple lines when using \G format

	// First, try to find the entire JSON object across multiple lines
	lines := strings.Split(output, "\n")

	// Look for "EXPLAIN:" or the start of JSON
	var jsonLines []string
	inJSON := false
	startFound := false

	for _, line := range lines {
		// Stop at row separator after JSON started
		if startFound && (strings.Contains(line, "row in set") || strings.Contains(line, "+---")) {
			break
		}

		trimmed := strings.TrimSpace(line)

		// Skip row separator lines and table borders
		if strings.Contains(trimmed, "***") || strings.HasPrefix(trimmed, "+---") {
			continue
		}

		// Remove table cell borders if present (| content |)
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		}

		// Skip empty lines
		if len(trimmed) == 0 {
			continue
		}

		// Check if line contains "EXPLAIN: {" (vertical format \G)
		if strings.HasPrefix(trimmed, "EXPLAIN:") {
			afterExplain := strings.TrimSpace(trimmed[8:])
			if strings.HasPrefix(afterExplain, "{") || strings.HasPrefix(afterExplain, "[") {
				startFound = true
				inJSON = true
				jsonLines = append(jsonLines, afterExplain)
			}
			continue
		}

		// Start collecting when we see { or [
		if !startFound && (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
			startFound = true
			inJSON = true
		}

		// Collect all lines once we've started
		if inJSON {
			jsonLines = append(jsonLines, trimmed)
		}
	}

	// Try to parse the collected lines as JSON
	if len(jsonLines) > 0 {
		jsonStr := strings.Join(jsonLines, "\n")
		// Validate it's proper JSON
		var test interface{}
		if err := json.Unmarshal([]byte(jsonStr), &test); err == nil {
			return jsonStr, nil
		}
	}

	// Fallback: Original single-line logic
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			// Extract JSON until we find the end
			startIdx := strings.Index(line, "{")
			if startIdx == -1 {
				startIdx = strings.Index(line, "[")
			}
			if startIdx == -1 {
				continue
			}

			// Simple JSON extraction - find matching brace/bracket
			jsonContent := line[startIdx:]
			openBraces := 0
			openBrackets := 0

			for i, char := range jsonContent {
				switch char {
				case '{':
					openBraces++
				case '}':
					openBraces--
				case '[':
					openBrackets++
				case ']':
					openBrackets--
				}

				if openBraces == 0 && openBrackets == 0 && i > 0 {
					return jsonContent[:i+1], nil
				}
			}

			// If we can't find the end, return what we have
			if openBraces == 0 && openBrackets == 0 {
				return jsonContent, nil
			}
		}
	}

	return "", fmt.Errorf("no JSON found in EXPLAIN output")
}

// visualExplain creates a visual tree representation of the query execution plan
func (p *PromptExecutor) visualExplain(jsonPlan string) (string, error) {
	var plan ExplainPlan
	if err := json.Unmarshal([]byte(jsonPlan), &plan); err != nil {
		return "", fmt.Errorf("failed to parse JSON plan: %v", err)
	}

	if plan.QueryBlock == nil {
		return "", fmt.Errorf("no query block found in plan")
	}

	var result strings.Builder
	result.WriteString("Query Execution Plan:\n")
	result.WriteString("=====================\n\n")

	p.buildVisualTree(&result, plan.QueryBlock, "", true)
	return result.String(), nil
}

// buildVisualTree recursively builds the visual tree representation
func (p *PromptExecutor) buildVisualTree(result *strings.Builder, node *ExplainNode, prefix string, isLast bool) {
	if node == nil {
		return
	}

	// Build the current node's display
	var nodeInfo strings.Builder

	// Table name and access type
	if node.TableName != "" {
		nodeInfo.WriteString(node.TableName)
		if node.AccessType != "" {
			nodeInfo.WriteString(fmt.Sprintf(" (%s)", node.AccessType))
		}
	} else if node.QueryBlock != nil {
		nodeInfo.WriteString("Query Block")
	}

	// Key information
	if node.Key != "" {
		nodeInfo.WriteString(fmt.Sprintf(" using %s", node.Key))
	}

	// Rows and cost info
	var details []string
	if node.Rows != nil {
		details = append(details, fmt.Sprintf("rows=%v", node.Rows))
	}
	if node.Filtered != nil {
		details = append(details, fmt.Sprintf("filtered=%v%%", node.Filtered))
	}
	if node.CostInfo != nil {
		if cost, ok := node.CostInfo["query_cost"]; ok {
			details = append(details, fmt.Sprintf("cost=%v", cost))
		}
	}

	if len(details) > 0 {
		nodeInfo.WriteString(fmt.Sprintf(" [%s]", strings.Join(details, ", ")))
	}

	// Add the current line
	result.WriteString(prefix)
	if isLast {
		result.WriteString("└── ")
	} else {
		result.WriteString("├── ")
	}
	result.WriteString(nodeInfo.String())
	result.WriteString("\n")

	// Prepare prefix for children
	newPrefix := prefix
	if isLast {
		newPrefix += "    "
	} else {
		newPrefix += "│   "
	}

	// Handle nested structures
	children := p.getChildNodes(node)
	for i, child := range children {
		isLastChild := i == len(children)-1
		if childNode, ok := child.(*ExplainNode); ok {
			p.buildVisualTree(result, childNode, newPrefix, isLastChild)
		}
	}
}

// getChildNodes extracts child nodes from various nested structures
func (p *PromptExecutor) getChildNodes(node *ExplainNode) []interface{} {
	var children []interface{}

	// Handle nested loop joins
	if node.NestedLoop != nil {
		children = append(children, node.NestedLoop...)
	}

	// Handle table references
	if node.Table != nil {
		children = append(children, node.Table)
	}

	// Handle query blocks
	if node.QueryBlock != nil {
		children = append(children, node.QueryBlock)
	}

	return children
}

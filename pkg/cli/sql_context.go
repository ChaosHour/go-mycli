package cli

import (
	"regexp"
	"strings"
)

// SQLContext represents the current parsing context for auto-completion
type SQLContext int

const (
	ContextUnknown  SQLContext = iota
	ContextKeyword             // Start of statement or after keywords like AND, OR
	ContextTable               // After FROM, JOIN, UPDATE, INTO, DESC, DESCRIBE
	ContextColumn              // After SELECT, WHERE, ORDER BY, GROUP BY, HAVING, SET
	ContextDatabase            // After USE, or schema qualification
	ContextFunction            // Inside function call
	ContextAlias               // After AS or table name (expecting alias)
	ContextOperator            // After column name (expecting =, >, <, etc.)
	ContextValue               // After operator (expecting value)
	ContextJoinOn              // After JOIN ... ON (expecting join condition)
	ContextShowItem            // After SHOW keyword
	ContextTableDot            // After table. (expecting column from specific table)
)

// TableAlias maps alias names to table names
type TableAlias struct {
	Alias     string
	TableName string
}

// SQLParseResult contains the parsed context information
type SQLParseResult struct {
	Context        SQLContext
	CurrentTable   string   // The table being referenced (for table.column)
	Tables         []string // Tables mentioned in the query
	Aliases        []TableAlias
	LastKeyword    string
	PartialWord    string
	InParentheses  int  // Nesting level of parentheses
	HasFrom        bool // Whether FROM clause has been seen
	HasWhere       bool
	HasOrderBy     bool
	HasGroupBy     bool
	HasHaving      bool
	IsSubquery     bool
	AfterComma     bool // Just typed a comma
	ExpectingAlias bool
}

// ParseSQLContext analyzes the current SQL input and determines the context
func ParseSQLContext(input string, cursorPos int) *SQLParseResult {
	result := &SQLParseResult{
		Context: ContextKeyword,
		Tables:  []string{},
		Aliases: []TableAlias{},
	}

	if cursorPos > len(input) {
		cursorPos = len(input)
	}

	// Get text up to cursor
	textToCursor := input[:cursorPos]
	if textToCursor == "" {
		return result
	}

	// Track parentheses depth
	result.InParentheses = countParenthesesDepth(textToCursor)

	// Tokenize the input
	tokens := tokenizeSQL(textToCursor)
	if len(tokens) == 0 {
		return result
	}

	// Parse tokens to understand context
	result.parseTokens(tokens)

	// Check for table.column pattern
	result.checkTableDotPattern(textToCursor)

	return result
}

// tokenizeSQL splits SQL into tokens while preserving structure
func tokenizeSQL(sql string) []string {
	var tokens []string
	var current strings.Builder
	inString := false
	stringChar := rune(0)
	inBacktick := false

	for i, ch := range sql {
		switch {
		case ch == '\'' || ch == '"':
			if !inString {
				inString = true
				stringChar = ch
			} else if ch == stringChar {
				// Check for escaped quote
				if i > 0 && sql[i-1] != '\\' {
					inString = false
				}
			}
			current.WriteRune(ch)

		case ch == '`':
			inBacktick = !inBacktick
			current.WriteRune(ch)

		case !inString && !inBacktick && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}

		case !inString && !inBacktick && (ch == ',' || ch == '(' || ch == ')' || ch == ';' || ch == '.'):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(ch))

		case !inString && !inBacktick && (ch == '=' || ch == '>' || ch == '<' || ch == '!'):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// Handle multi-char operators like >=, <=, !=, <>
			if i+1 < len(sql) {
				next := sql[i+1]
				if (ch == '>' || ch == '<' || ch == '!' || ch == '=') && next == '=' {
					tokens = append(tokens, string(ch)+string(next))
					continue
				}
				if ch == '<' && next == '>' {
					tokens = append(tokens, "<>")
					continue
				}
			}
			tokens = append(tokens, string(ch))

		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// parseTokens analyzes the token stream to determine context
func (r *SQLParseResult) parseTokens(tokens []string) {
	if len(tokens) == 0 {
		return
	}

	// Track the last significant keyword and state
	var lastToken string
	var prevToken string
	expectingTableName := false
	expectingColumnName := false
	afterAs := false

	for i, token := range tokens {
		upperToken := strings.ToUpper(token)
		prevToken = lastToken
		lastToken = token

		// Track clause presence
		switch upperToken {
		case "FROM":
			r.HasFrom = true
		case "WHERE":
			r.HasWhere = true
		case "ORDER":
			if i+1 < len(tokens) && strings.ToUpper(tokens[i+1]) == "BY" {
				r.HasOrderBy = true
			}
		case "GROUP":
			if i+1 < len(tokens) && strings.ToUpper(tokens[i+1]) == "BY" {
				r.HasGroupBy = true
			}
		case "HAVING":
			r.HasHaving = true
		}

		// State machine for context detection
		switch upperToken {
		case "SELECT":
			r.Context = ContextColumn
			expectingColumnName = true
			expectingTableName = false

		case "FROM", "JOIN", "INNER", "LEFT", "RIGHT", "OUTER", "CROSS", "NATURAL":
			if upperToken == "FROM" || upperToken == "JOIN" ||
				(upperToken != "INNER" && upperToken != "LEFT" && upperToken != "RIGHT" && upperToken != "OUTER" && upperToken != "CROSS" && upperToken != "NATURAL") {
				r.Context = ContextTable
				expectingTableName = true
				expectingColumnName = false
			}

		case "UPDATE", "INTO", "TABLE":
			r.Context = ContextTable
			expectingTableName = true
			expectingColumnName = false

		case "DESCRIBE", "DESC", "EXPLAIN":
			if !r.HasFrom { // DESC as DESCRIBE, not DESC for ORDER BY DESC
				r.Context = ContextTable
				expectingTableName = true
				expectingColumnName = false
			}

		case "USE":
			r.Context = ContextDatabase
			expectingTableName = false
			expectingColumnName = false

		case "WHERE", "AND", "OR", "HAVING":
			r.Context = ContextColumn
			expectingColumnName = true
			expectingTableName = false

		case "SET":
			r.Context = ContextColumn
			expectingColumnName = true
			expectingTableName = false

		case "ORDER", "GROUP":
			// Wait for BY
			r.Context = ContextKeyword

		case "BY":
			if strings.ToUpper(prevToken) == "ORDER" || strings.ToUpper(prevToken) == "GROUP" {
				r.Context = ContextColumn
				expectingColumnName = true
				expectingTableName = false
			}

		case "ON":
			r.Context = ContextJoinOn
			expectingColumnName = true
			expectingTableName = false

		case "AS":
			afterAs = true
			r.ExpectingAlias = true
			r.Context = ContextAlias

		case "SHOW":
			r.Context = ContextShowItem

		case ",":
			r.AfterComma = true
			// Comma resets to previous context (column or table list)
			// Reset ExpectingAlias since comma ends that possibility
			r.ExpectingAlias = false
			if r.Context == ContextTable || r.Context == ContextAlias {
				// In table list (FROM users, orders)
				expectingTableName = true
				expectingColumnName = false
				r.Context = ContextTable
			} else if expectingColumnName || r.Context == ContextColumn {
				r.Context = ContextColumn
			}

		case "(":
			r.InParentheses++
			// Could be subquery or function call
			if isFunction(prevToken) {
				r.Context = ContextFunction
			}

		case ")":
			r.InParentheses--

		case ".":
			// table.column pattern - the next token should be a column
			if !isKeyword(prevToken) && prevToken != "" && prevToken != "," {
				r.CurrentTable = prevToken
				r.Context = ContextTableDot
			}

		case "=", ">", "<", ">=", "<=", "!=", "<>", "LIKE", "IN", "BETWEEN", "IS":
			r.Context = ContextValue
			r.ExpectingAlias = false

		default:
			// It's an identifier (table name, column name, alias, etc.)
			if afterAs {
				// This is an alias (explicit with AS)
				if len(r.Tables) > 0 {
					lastTable := r.Tables[len(r.Tables)-1]
					r.Aliases = append(r.Aliases, TableAlias{
						Alias:     token,
						TableName: lastTable,
					})
				}
				afterAs = false
				r.ExpectingAlias = false
			} else if expectingTableName && !isKeyword(upperToken) && token != "," {
				// This is a table name
				tableName := strings.Trim(token, "`")
				r.Tables = append(r.Tables, tableName)
				// After table name, we might get an alias
				r.ExpectingAlias = true
				r.Context = ContextAlias
				expectingTableName = false
			} else if r.ExpectingAlias && !isKeyword(upperToken) && token != "," && upperToken != "AS" && token != "." {
				// This is likely an alias (implicit, without AS)
				// Only if it's not a clause keyword that would start a new section
				if len(r.Tables) > 0 && !isClauseKeyword(upperToken) {
					lastTable := r.Tables[len(r.Tables)-1]
					r.Aliases = append(r.Aliases, TableAlias{
						Alias:     token,
						TableName: lastTable,
					})
				}
				r.ExpectingAlias = false
			}
		}

		// Special handling for JOIN variations
		if upperToken == "INNER" || upperToken == "LEFT" || upperToken == "RIGHT" ||
			upperToken == "OUTER" || upperToken == "CROSS" || upperToken == "NATURAL" {
			// These precede JOIN, keep looking
			continue
		}
	}

	// Store the last keyword and partial word
	if len(tokens) > 0 {
		lastToken := tokens[len(tokens)-1]
		if isKeyword(strings.ToUpper(lastToken)) {
			r.LastKeyword = strings.ToUpper(lastToken)
		} else {
			r.PartialWord = lastToken
			if len(tokens) > 1 {
				prevToken := tokens[len(tokens)-2]
				if isKeyword(strings.ToUpper(prevToken)) {
					r.LastKeyword = strings.ToUpper(prevToken)
				}
			}
		}
	}
}

// checkTableDotPattern detects if the cursor is right after "table."
func (r *SQLParseResult) checkTableDotPattern(text string) {
	// Pattern: identifier followed by dot at the end
	pattern := regexp.MustCompile(`(\w+)\.\s*$`)
	if matches := pattern.FindStringSubmatch(text); len(matches) > 1 {
		r.CurrentTable = matches[1]
		r.Context = ContextTableDot
	}

	// Also check for backtick-quoted identifiers
	pattern2 := regexp.MustCompile("`([^`]+)`\\.\\s*$")
	if matches := pattern2.FindStringSubmatch(text); len(matches) > 1 {
		r.CurrentTable = matches[1]
		r.Context = ContextTableDot
	}
}

// countParenthesesDepth counts unclosed parentheses
func countParenthesesDepth(text string) int {
	depth := 0
	inString := false
	stringChar := rune(0)

	for i, ch := range text {
		switch {
		case ch == '\'' || ch == '"':
			if !inString {
				inString = true
				stringChar = ch
			} else if ch == stringChar && (i == 0 || text[i-1] != '\\') {
				inString = false
			}
		case !inString && ch == '(':
			depth++
		case !inString && ch == ')':
			depth--
		}
	}
	return depth
}

// isKeyword checks if a token is a SQL keyword
func isKeyword(token string) bool {
	keywords := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "AND": true, "OR": true,
		"JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true, "OUTER": true,
		"CROSS": true, "NATURAL": true, "ON": true, "AS": true, "ORDER": true,
		"BY": true, "GROUP": true, "HAVING": true, "LIMIT": true, "OFFSET": true,
		"INSERT": true, "INTO": true, "VALUES": true, "UPDATE": true, "SET": true,
		"DELETE": true, "CREATE": true, "ALTER": true, "DROP": true, "TABLE": true,
		"INDEX": true, "VIEW": true, "DATABASE": true, "SCHEMA": true, "USE": true,
		"SHOW": true, "DESCRIBE": true, "DESC": true, "EXPLAIN": true, "ANALYZE": true,
		"UNION": true, "ALL": true, "DISTINCT": true, "BETWEEN": true, "LIKE": true,
		"IN": true, "IS": true, "NULL": true, "NOT": true, "TRUE": true, "FALSE": true,
		"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
		"EXISTS": true, "ANY": true, "SOME": true, "ASC": true, "WITH": true,
		"RECURSIVE": true, "OVER": true, "PARTITION": true, "WINDOW": true,
		"ROWS": true, "RANGE": true, "PRECEDING": true, "FOLLOWING": true, "CURRENT": true,
		"FIRST": true, "LAST": true, "NULLS": true,
	}
	return keywords[token]
}

// isClauseKeyword checks if a token starts a new clause
func isClauseKeyword(token string) bool {
	clauses := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "JOIN": true,
		"INNER": true, "LEFT": true, "RIGHT": true, "OUTER": true,
		"CROSS": true, "NATURAL": true, "ON": true, "ORDER": true,
		"GROUP": true, "HAVING": true, "LIMIT": true, "UNION": true,
		"SET": true, "VALUES": true,
	}
	return clauses[token]
}

// isFunction checks if a token is a known SQL function
func isFunction(token string) bool {
	upper := strings.ToUpper(token)
	functions := map[string]bool{
		// Aggregate functions
		"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
		"GROUP_CONCAT": true, "BIT_AND": true, "BIT_OR": true, "BIT_XOR": true,
		"STD": true, "STDDEV": true, "STDDEV_POP": true, "STDDEV_SAMP": true,
		"VAR_POP": true, "VAR_SAMP": true, "VARIANCE": true,

		// String functions
		"CONCAT": true, "CONCAT_WS": true, "SUBSTRING": true, "SUBSTR": true,
		"LEFT": true, "RIGHT": true, "LENGTH": true, "CHAR_LENGTH": true,
		"UPPER": true, "LOWER": true, "TRIM": true, "LTRIM": true, "RTRIM": true,
		"REPLACE": true, "REVERSE": true, "REPEAT": true, "SPACE": true,
		"LPAD": true, "RPAD": true, "INSTR": true, "LOCATE": true, "POSITION": true,
		"ASCII": true, "CHAR": true, "ORD": true, "HEX": true, "UNHEX": true,
		"FORMAT": true, "INSERT": true, "FIELD": true, "FIND_IN_SET": true,
		"MAKE_SET": true, "EXPORT_SET": true, "SOUNDEX": true, "QUOTE": true,

		// Numeric functions
		"ABS": true, "CEIL": true, "CEILING": true, "FLOOR": true, "ROUND": true,
		"TRUNCATE": true, "MOD": true, "POW": true, "POWER": true, "SQRT": true,
		"EXP": true, "LOG": true, "LOG10": true, "LOG2": true, "LN": true,
		"SIN": true, "COS": true, "TAN": true, "ASIN": true, "ACOS": true, "ATAN": true,
		"COT": true, "RAND": true, "SIGN": true, "PI": true, "RADIANS": true, "DEGREES": true,

		// Date/time functions
		"NOW": true, "CURDATE": true, "CURRENT_DATE": true, "CURTIME": true,
		"CURRENT_TIME": true, "CURRENT_TIMESTAMP": true, "LOCALTIME": true,
		"LOCALTIMESTAMP": true, "SYSDATE": true, "UTC_DATE": true, "UTC_TIME": true,
		"UTC_TIMESTAMP": true, "DATE": true, "TIME": true, "DATETIME": true,
		"TIMESTAMP": true, "YEAR": true, "MONTH": true, "DAY": true, "DAYOFMONTH": true,
		"DAYOFWEEK": true, "DAYOFYEAR": true, "WEEKDAY": true, "WEEK": true,
		"WEEKOFYEAR": true, "YEARWEEK": true, "HOUR": true, "MINUTE": true, "SECOND": true,
		"MICROSECOND": true, "QUARTER": true, "DATE_ADD": true, "DATE_SUB": true,
		"ADDDATE": true, "SUBDATE": true, "ADDTIME": true, "SUBTIME": true,
		"DATEDIFF": true, "TIMEDIFF": true, "TIMESTAMPDIFF": true, "TIMESTAMPADD": true,
		"DATE_FORMAT": true, "TIME_FORMAT": true, "STR_TO_DATE": true, "GET_FORMAT": true,
		"FROM_UNIXTIME": true, "UNIX_TIMESTAMP": true, "FROM_DAYS": true, "TO_DAYS": true,
		"TO_SECONDS": true, "SEC_TO_TIME": true, "TIME_TO_SEC": true, "MAKEDATE": true,
		"MAKETIME": true, "EXTRACT": true, "LAST_DAY": true, "PERIOD_ADD": true,
		"PERIOD_DIFF": true, "CONVERT_TZ": true,

		// Control flow
		"IF": true, "IFNULL": true, "NULLIF": true, "COALESCE": true, "GREATEST": true,
		"LEAST": true, "ISNULL": true,

		// JSON functions
		"JSON_OBJECT": true, "JSON_ARRAY": true, "JSON_EXTRACT": true, "JSON_UNQUOTE": true,
		"JSON_SET": true, "JSON_INSERT": true, "JSON_REPLACE": true, "JSON_REMOVE": true,
		"JSON_CONTAINS": true, "JSON_CONTAINS_PATH": true, "JSON_TYPE": true,
		"JSON_KEYS": true, "JSON_LENGTH": true, "JSON_DEPTH": true, "JSON_VALID": true,
		"JSON_SEARCH": true, "JSON_MERGE": true, "JSON_MERGE_PATCH": true, "JSON_MERGE_PRESERVE": true,
		"JSON_ARRAYAGG": true, "JSON_OBJECTAGG": true, "JSON_PRETTY": true, "JSON_QUOTE": true,
		"JSON_STORAGE_SIZE": true, "JSON_STORAGE_FREE": true, "JSON_TABLE": true,

		// Window functions
		"ROW_NUMBER": true, "RANK": true, "DENSE_RANK": true, "NTILE": true,
		"LEAD": true, "LAG": true, "FIRST_VALUE": true, "LAST_VALUE": true,
		"NTH_VALUE": true, "CUME_DIST": true, "PERCENT_RANK": true,

		// Other
		"CAST": true, "CONVERT": true, "BINARY": true, "CHARSET": true, "COLLATION": true,
		"CONNECTION_ID": true, "CURRENT_USER": true, "DATABASE": true, "FOUND_ROWS": true,
		"LAST_INSERT_ID": true, "ROW_COUNT": true, "SCHEMA": true, "SESSION_USER": true,
		"SYSTEM_USER": true, "USER": true, "VERSION": true, "BENCHMARK": true,
		"COERCIBILITY": true, "UUID": true,
		"UUID_SHORT": true, "UUID_TO_BIN": true, "BIN_TO_UUID": true,
		"INET_ATON": true, "INET_NTOA": true, "INET6_ATON": true, "INET6_NTOA": true,
		"IS_IPV4": true, "IS_IPV6": true, "IS_IPV4_COMPAT": true, "IS_IPV4_MAPPED": true,
		"MD5": true, "SHA": true, "SHA1": true, "SHA2": true, "AES_ENCRYPT": true,
		"AES_DECRYPT": true, "COMPRESS": true, "UNCOMPRESS": true,
		"UNCOMPRESSED_LENGTH": true, "RANDOM_BYTES": true,
	}
	return functions[upper]
}

// ResolveAlias returns the table name for an alias, or the original name if not an alias
func (r *SQLParseResult) ResolveAlias(name string) string {
	for _, alias := range r.Aliases {
		if strings.EqualFold(alias.Alias, name) {
			return alias.TableName
		}
	}
	return name
}

// GetContextDescription returns a human-readable description of the context
func (r *SQLParseResult) GetContextDescription() string {
	switch r.Context {
	case ContextKeyword:
		return "SQL keyword expected"
	case ContextTable:
		return "Table name expected"
	case ContextColumn:
		return "Column name expected"
	case ContextDatabase:
		return "Database name expected"
	case ContextFunction:
		return "Function argument expected"
	case ContextAlias:
		return "Alias expected"
	case ContextOperator:
		return "Operator expected"
	case ContextValue:
		return "Value expected"
	case ContextJoinOn:
		return "Join condition expected"
	case ContextShowItem:
		return "SHOW option expected"
	case ContextTableDot:
		return "Column from " + r.CurrentTable + " expected"
	default:
		return "Unknown context"
	}
}

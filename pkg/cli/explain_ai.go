package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-mycli/pkg/ai"
)

// ExplainAnalysis represents the data needed for AI analysis
type ExplainAnalysis struct {
	Query       string `json:"query"`
	ExplainJSON string `json:"explain_json"`
	Schema      string `json:"schema"`
}

// SchemaInfo holds table and index metadata
type SchemaInfo struct {
	Tables  []TableInfo `json:"tables"`
	Indexes []IndexInfo `json:"indexes"`
}

// TableInfo represents table column information
type TableInfo struct {
	TableName string       `json:"table_name"`
	Columns   []ColumnInfo `json:"columns"`
}

// ColumnInfo represents column metadata
type ColumnInfo struct {
	ColumnName string `json:"column_name"`
	OrdinalPos int    `json:"ordinal_position"`
	ColumnType string `json:"column_type"`
	IsNullable string `json:"is_nullable"`
	ColumnKey  string `json:"column_key"`
	Extra      string `json:"extra"`
}

// IndexInfo represents index metadata
type IndexInfo struct {
	TableName  string `json:"table_name"`
	NonUnique  int    `json:"non_unique"`
	IndexName  string `json:"index_name"`
	SeqInIndex int    `json:"seq_in_index"`
	ColumnName string `json:"column_name"`
}

// collectSchemaSnapshot collects table and index metadata for the current database
func (p *PromptExecutor) collectSchemaSnapshot() (*SchemaInfo, error) {
	schema := &SchemaInfo{}

	// Collect table column information
	columnsQuery := `
		SELECT TABLE_NAME, COLUMN_NAME, ORDINAL_POSITION, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, EXTRA
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME, ORDINAL_POSITION`

	rows, err := p.db.Query(columnsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	tableMap := make(map[string]*TableInfo)
	for rows.Next() {
		var col ColumnInfo
		var tableName string
		err := rows.Scan(&tableName, &col.ColumnName, &col.OrdinalPos, &col.ColumnType, &col.IsNullable, &col.ColumnKey, &col.Extra)
		if err != nil {
			return nil, fmt.Errorf("failed to scan column row: %w", err)
		}

		if tableMap[tableName] == nil {
			tableMap[tableName] = &TableInfo{TableName: tableName}
		}
		tableMap[tableName].Columns = append(tableMap[tableName].Columns, col)
	}

	for _, table := range tableMap {
		schema.Tables = append(schema.Tables, *table)
	}

	// Collect index information
	indexesQuery := `
		SELECT TABLE_NAME, NON_UNIQUE, INDEX_NAME, SEQ_IN_INDEX, COLUMN_NAME
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

	rows, err = p.db.Query(indexesQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var idx IndexInfo
		err := rows.Scan(&idx.TableName, &idx.NonUnique, &idx.IndexName, &idx.SeqInIndex, &idx.ColumnName)
		if err != nil {
			return nil, fmt.Errorf("failed to scan index row: %w", err)
		}
		schema.Indexes = append(schema.Indexes, idx)
	}

	return schema, nil
}

// extractQueryFromExplain extracts the original query from an EXPLAIN statement
func extractQueryFromExplain(explainStmt string) (string, string, error) {
	// Remove EXPLAIN prefix and any FORMAT clauses
	query := strings.TrimSpace(explainStmt)

	// Handle different EXPLAIN formats
	if strings.HasPrefix(strings.ToUpper(query), "EXPLAIN ") {
		query = strings.TrimSpace(query[8:])
	}

	// Remove FORMAT clauses
	format := "TABULAR"
	if strings.HasPrefix(strings.ToUpper(query), "FORMAT=") {
		// Extract format type
		if strings.HasPrefix(strings.ToUpper(query), "FORMAT=JSON ") {
			format = "JSON"
			query = strings.TrimSpace(query[12:])
		} else if strings.HasPrefix(strings.ToUpper(query), "FORMAT=TREE ") {
			format = "TREE"
			query = strings.TrimSpace(query[12:])
		} else if strings.HasPrefix(strings.ToUpper(query), "FORMAT=TRADITIONAL ") {
			format = "TRADITIONAL"
			query = strings.TrimSpace(query[18:])
		}
	}

	// Handle ANALYZE
	if strings.HasPrefix(strings.ToUpper(query), "ANALYZE ") {
		format = "ANALYZE"
		query = strings.TrimSpace(query[8:])
	}

	return query, format, nil
}

// analyzeExplainWithAI sends EXPLAIN data to AI for performance analysis
func (p *PromptExecutor) analyzeExplainWithAI(explainStmt string, explainOutput string) error {
	// Extract the original query and format
	originalQuery, format, err := extractQueryFromExplain(explainStmt)
	if err != nil {
		return fmt.Errorf("failed to extract query from EXPLAIN: %w", err)
	}

	var jsonPlan string

	// Use MySQL 8.4+ JSON capture feature if available
	if p.isMySQL84Plus() && format != "ANALYZE" {
		jsonPlan, err = p.executeExplainWithJSONCapture(explainStmt)
		if err != nil {
			// Fall back to parsing the output if JSON capture fails
			fmt.Printf("JSON capture failed, falling back to output parsing: %v\n", err)
			// Extract clean JSON from the output
			jsonPlan, err = p.extractJSONFromExplainOutput(explainOutput)
			if err != nil {
				return fmt.Errorf("failed to extract JSON from EXPLAIN output: %w", err)
			}
		}
	} else {
		// Extract clean JSON from the output for older MySQL versions or ANALYZE format
		jsonPlan, err = p.extractJSONFromExplainOutput(explainOutput)
		if err != nil {
			return fmt.Errorf("failed to extract JSON from EXPLAIN output: %w", err)
		}
	}

	// Collect schema snapshot
	schema, err := p.collectSchemaSnapshot()
	if err != nil {
		return fmt.Errorf("failed to collect schema: %w", err)
	}

	// Prepare schema JSON
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Prepare analysis payload
	analysis := ExplainAnalysis{
		Query:       originalQuery,
		ExplainJSON: jsonPlan,
		Schema:      string(schemaJSON),
	}

	// Get AI advice using configured MCP server
	var advice string
	if p.aiServerMode != "" || p.aiServerURL != "" {
		client, err := ai.NewAIClient(p.aiServerMode, p.aiServerURL, p.aiCachePath)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}
		advice, err = client.ExplainPlan(analysis.Query, analysis.ExplainJSON, analysis.Schema, p.aiDetailLevel)
		if err != nil {
			return fmt.Errorf("failed to get AI advice: %w", err)
		}
	} else {
		return fmt.Errorf("AI analysis not configured. Set --ai-server-url and --ai-server-mode")
	}

	// Display the advice
	fmt.Println("\n🤖 AI Performance Analysis:")
	fmt.Println("==========================")
	if p.aiServerMode != "" || p.aiServerURL != "" {
		fmt.Printf("(AI backend: %s %s)\n", p.aiServerMode, p.aiServerURL)
	}
	fmt.Println(advice)
	fmt.Println()

	return nil
}

// isMySQL84Plus checks if the MySQL server version is 8.4 or higher
func (p *PromptExecutor) isMySQL84Plus() bool {
	var version string
	err := p.db.QueryRow("SELECT VERSION()").Scan(&version)
	if err != nil {
		return false
	}

	// Parse version string (e.g., "8.4.0" or "8.4.0-commercial")
	version = strings.Split(version, "-")[0] // Remove suffix if present
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}

	return major > 8 || (major == 8 && minor >= 4)
}

// executeExplainWithJSONCapture executes EXPLAIN using MySQL 8.4+ JSON capture feature
func (p *PromptExecutor) executeExplainWithJSONCapture(explainStmt string) (string, error) {
	// Extract the original query
	originalQuery, _, err := extractQueryFromExplain(explainStmt)
	if err != nil {
		return "", fmt.Errorf("failed to extract query: %w", err)
	}

	// Generate a unique variable name to avoid conflicts
	varName := fmt.Sprintf("@go_mycli_explain_%d", time.Now().UnixNano())

	// Create the enhanced EXPLAIN statement using INTO syntax
	enhancedExplain := fmt.Sprintf("EXPLAIN FORMAT=JSON INTO %s %s", varName, originalQuery)

	// Execute the EXPLAIN with JSON capture
	_, err = p.db.Exec(enhancedExplain)
	if err != nil {
		return "", fmt.Errorf("failed to execute enhanced EXPLAIN: %w", err)
	}

	// Retrieve the JSON from the user variable
	var jsonPlan string
	err = p.db.QueryRow(fmt.Sprintf("SELECT %s", varName)).Scan(&jsonPlan)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve JSON plan: %w", err)
	}

	// Clean up the user variable
	_, _ = p.db.Exec(fmt.Sprintf("SET %s = NULL", varName))

	return jsonPlan, nil
}

// isExplainQuery checks if a query is an EXPLAIN statement
func isExplainQuery(query string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "EXPLAIN")
}

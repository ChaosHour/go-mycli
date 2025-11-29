package cli

import (
	"testing"
)

func TestParseSQLContext_Empty(t *testing.T) {
	result := ParseSQLContext("", 0)
	if result.Context != ContextKeyword {
		t.Errorf("Expected ContextKeyword for empty input, got %v", result.Context)
	}
}

func TestParseSQLContext_SelectStart(t *testing.T) {
	result := ParseSQLContext("SELECT ", 7)
	if result.Context != ContextColumn {
		t.Errorf("Expected ContextColumn after SELECT, got %v", result.Context)
	}
}

func TestParseSQLContext_FromClause(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM ", 14)
	if result.Context != ContextTable {
		t.Errorf("Expected ContextTable after FROM, got %v", result.Context)
	}
	if !result.HasFrom {
		t.Error("Expected HasFrom to be true")
	}
}

func TestParseSQLContext_WhereClause(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users WHERE ", 26)
	if result.Context != ContextColumn {
		t.Errorf("Expected ContextColumn after WHERE, got %v", result.Context)
	}
	if !result.HasWhere {
		t.Error("Expected HasWhere to be true")
	}
	if len(result.Tables) != 1 || result.Tables[0] != "users" {
		t.Errorf("Expected tables to contain 'users', got %v", result.Tables)
	}
}

func TestParseSQLContext_TableDot(t *testing.T) {
	result := ParseSQLContext("SELECT users.", 13)
	if result.Context != ContextTableDot {
		t.Errorf("Expected ContextTableDot after 'users.', got %v", result.Context)
	}
	if result.CurrentTable != "users" {
		t.Errorf("Expected CurrentTable to be 'users', got %s", result.CurrentTable)
	}
}

func TestParseSQLContext_JoinOn(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users JOIN orders ON ", 35)
	if result.Context != ContextJoinOn {
		t.Errorf("Expected ContextJoinOn after JOIN...ON, got %v", result.Context)
	}
	if len(result.Tables) != 2 {
		t.Errorf("Expected 2 tables, got %d", len(result.Tables))
	}
}

func TestParseSQLContext_UseDatabase(t *testing.T) {
	result := ParseSQLContext("USE ", 4)
	if result.Context != ContextDatabase {
		t.Errorf("Expected ContextDatabase after USE, got %v", result.Context)
	}
}

func TestParseSQLContext_Show(t *testing.T) {
	result := ParseSQLContext("SHOW ", 5)
	if result.Context != ContextShowItem {
		t.Errorf("Expected ContextShowItem after SHOW, got %v", result.Context)
	}
}

func TestParseSQLContext_Alias(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users AS u WHERE u.", 33)
	if result.Context != ContextTableDot {
		t.Errorf("Expected ContextTableDot after 'u.', got %v", result.Context)
	}
	if result.CurrentTable != "u" {
		t.Errorf("Expected CurrentTable to be 'u', got %s", result.CurrentTable)
	}
	// Check alias resolution
	resolved := result.ResolveAlias("u")
	if resolved != "users" {
		t.Errorf("Expected ResolveAlias('u') to return 'users', got %s", resolved)
	}
}

func TestParseSQLContext_ImplicitAlias(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users u WHERE ", 28)
	// Implicit alias detection: "users u" should detect u as alias for users
	if len(result.Aliases) != 1 {
		t.Errorf("Expected 1 alias, got %d: %+v", len(result.Aliases), result.Aliases)
		return
	}
	if result.Aliases[0].Alias != "u" || result.Aliases[0].TableName != "users" {
		t.Errorf("Expected alias 'u' -> 'users', got %+v", result.Aliases[0])
	}
}

func TestParseSQLContext_OrderBy(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users ORDER BY ", 29)
	if result.Context != ContextColumn {
		t.Errorf("Expected ContextColumn after ORDER BY, got %v", result.Context)
	}
	if !result.HasOrderBy {
		t.Error("Expected HasOrderBy to be true")
	}
}

func TestParseSQLContext_GroupBy(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users GROUP BY ", 29)
	if result.Context != ContextColumn {
		t.Errorf("Expected ContextColumn after GROUP BY, got %v", result.Context)
	}
	if !result.HasGroupBy {
		t.Error("Expected HasGroupBy to be true")
	}
}

func TestParseSQLContext_Update(t *testing.T) {
	result := ParseSQLContext("UPDATE ", 7)
	if result.Context != ContextTable {
		t.Errorf("Expected ContextTable after UPDATE, got %v", result.Context)
	}
}

func TestParseSQLContext_InsertInto(t *testing.T) {
	result := ParseSQLContext("INSERT INTO ", 12)
	if result.Context != ContextTable {
		t.Errorf("Expected ContextTable after INSERT INTO, got %v", result.Context)
	}
}

func TestParseSQLContext_MultipleTables(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users, orders WHERE ", 34)
	if len(result.Tables) != 2 {
		t.Errorf("Expected 2 tables, got %d: %v", len(result.Tables), result.Tables)
	}
}

func TestParseSQLContext_LeftJoin(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users LEFT JOIN ", 30)
	if result.Context != ContextTable {
		t.Errorf("Expected ContextTable after LEFT JOIN, got %v", result.Context)
	}
}

func TestParseSQLContext_SetClause(t *testing.T) {
	result := ParseSQLContext("UPDATE users SET ", 17)
	if result.Context != ContextColumn {
		t.Errorf("Expected ContextColumn after SET, got %v", result.Context)
	}
}

func TestParseSQLContext_AfterOperator(t *testing.T) {
	result := ParseSQLContext("SELECT * FROM users WHERE id = ", 31)
	if result.Context != ContextValue {
		t.Errorf("Expected ContextValue after '=', got %v", result.Context)
	}
}

func TestParseSQLContext_Describe(t *testing.T) {
	result := ParseSQLContext("DESCRIBE ", 9)
	if result.Context != ContextTable {
		t.Errorf("Expected ContextTable after DESCRIBE, got %v", result.Context)
	}
}

func TestTokenizeSQL_Basic(t *testing.T) {
	tokens := tokenizeSQL("SELECT * FROM users")
	expected := []string{"SELECT", "*", "FROM", "users"}
	if len(tokens) != len(expected) {
		t.Errorf("Expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
	for i, tok := range expected {
		if tokens[i] != tok {
			t.Errorf("Token %d: expected %s, got %s", i, tok, tokens[i])
		}
	}
}

func TestTokenizeSQL_WithOperators(t *testing.T) {
	tokens := tokenizeSQL("WHERE id = 1")
	expected := []string{"WHERE", "id", "=", "1"}
	if len(tokens) != len(expected) {
		t.Errorf("Expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
}

func TestTokenizeSQL_WithDot(t *testing.T) {
	tokens := tokenizeSQL("users.id")
	expected := []string{"users", ".", "id"}
	if len(tokens) != len(expected) {
		t.Errorf("Expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
}

func TestTokenizeSQL_WithParentheses(t *testing.T) {
	tokens := tokenizeSQL("COUNT(*)")
	expected := []string{"COUNT", "(", "*", ")"}
	if len(tokens) != len(expected) {
		t.Errorf("Expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
}

func TestIsKeyword(t *testing.T) {
	tests := []struct {
		token    string
		expected bool
	}{
		{"SELECT", true},
		{"FROM", true},
		{"WHERE", true},
		{"users", false},
		{"id", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isKeyword(tt.token)
		if result != tt.expected {
			t.Errorf("isKeyword(%q) = %v, expected %v", tt.token, result, tt.expected)
		}
	}
}

func TestIsFunction(t *testing.T) {
	tests := []struct {
		token    string
		expected bool
	}{
		{"COUNT", true},
		{"SUM", true},
		{"NOW", true},
		{"users", false},
		{"SELECT", false},
	}

	for _, tt := range tests {
		result := isFunction(tt.token)
		if result != tt.expected {
			t.Errorf("isFunction(%q) = %v, expected %v", tt.token, result, tt.expected)
		}
	}
}

func TestCountParenthesesDepth(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"SELECT COUNT(*)", 0},
		{"SELECT COUNT(", 1},
		{"SELECT ((a", 2},
		{"SELECT (a + (b))", 0},
	}

	for _, tt := range tests {
		result := countParenthesesDepth(tt.input)
		if result != tt.expected {
			t.Errorf("countParenthesesDepth(%q) = %d, expected %d", tt.input, result, tt.expected)
		}
	}
}

func TestGetContextDescription(t *testing.T) {
	result := &SQLParseResult{Context: ContextTable}
	desc := result.GetContextDescription()
	if desc != "Table name expected" {
		t.Errorf("Expected 'Table name expected', got %s", desc)
	}

	result.Context = ContextTableDot
	result.CurrentTable = "users"
	desc = result.GetContextDescription()
	if desc != "Column from users expected" {
		t.Errorf("Expected 'Column from users expected', got %s", desc)
	}
}

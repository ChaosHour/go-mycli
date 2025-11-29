package cli

import (
	"testing"
)

func TestExtractQueryFromExplain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		format   string
	}{
		{"EXPLAIN SELECT * FROM users", "SELECT * FROM users", "TABULAR"},
		{"EXPLAIN FORMAT=JSON SELECT * FROM users", "SELECT * FROM users", "JSON"},
		{"EXPLAIN FORMAT=TREE SELECT * FROM users WHERE id = 1", "SELECT * FROM users WHERE id = 1", "TREE"},
		{"EXPLAIN ANALYZE SELECT * FROM users", "SELECT * FROM users", "ANALYZE"},
	}

	for _, test := range tests {
		query, format, err := extractQueryFromExplain(test.input)
		if err != nil {
			t.Errorf("extractQueryFromExplain(%q) returned error: %v", test.input, err)
			continue
		}
		if query != test.expected {
			t.Errorf("extractQueryFromExplain(%q) = %q, expected %q", test.input, query, test.expected)
		}
		if format != test.format {
			t.Errorf("extractQueryFromExplain(%q) format = %q, expected %q", test.input, format, test.format)
		}
	}
}

func TestIsExplainQuery(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"EXPLAIN SELECT * FROM users", true},
		{"EXPLAIN FORMAT=JSON SELECT * FROM users", true},
		{"explain select * from users", true},
		{"SELECT * FROM users", false},
		{"SHOW TABLES", false},
		{"", false},
	}

	for _, test := range tests {
		result := isExplainQuery(test.input)
		if result != test.expected {
			t.Errorf("isExplainQuery(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

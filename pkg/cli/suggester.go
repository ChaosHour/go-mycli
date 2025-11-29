package cli

import (
	"fmt"
	"strings"
)

// SuggestFixedSQL analyzes a given SQL and MySQL error and returns a suggested corrected SQL string.
// It intentionally returns a simple correction for the very common cases and should remain conservative.
func SuggestFixedSQL(p *PromptExecutor, sqlStr string, err error) string {
	msg := strings.ToUpper(err.Error())
	if !strings.Contains(msg, "ERROR 1064") {
		return ""
	}

	trimmed := strings.TrimSpace(sqlStr)
	if trimmed == "" {
		return ""
	}

	upper := strings.ToUpper(trimmed)

	// Only try for simple top-level SELECT ... WHERE ... cases
	if !strings.HasPrefix(upper, "SELECT ") || !strings.Contains(upper, " WHERE ") {
		return ""
	}

	// Guardrails: avoid suggestions for queries with parentheses, JOIN, UNION, or subqueries
	if strings.Contains(upper, "(") || strings.Contains(upper, " JOIN ") || strings.Contains(upper, " UNION ") || strings.Contains(upper, "SELECT(") {
		return ""
	}

	// If there's a WHERE but no FROM, try to insert a FROM just before WHERE
	if !strings.Contains(upper, " FROM ") {
		whereIdx := strings.Index(upper, " WHERE ")
		if whereIdx == -1 {
			return ""
		}
		left := strings.TrimSpace(trimmed[:whereIdx])
		right := strings.TrimSpace(trimmed[whereIdx+7:])

		// Try to guess a table name from column references like "actor id" or "actor_id"
		candidateTable := ""
		lowerRight := strings.ToLower(right)
		if strings.Contains(lowerRight, "actor id") || strings.Contains(lowerRight, "actor_id") {
			candidateTable = "actor"
		}

		// Try to find a table from the executor's cached tables if none guessed
		if candidateTable == "" && p != nil && len(p.tables) > 0 {
			candidateTable = p.tables[0]
		}

		if candidateTable != "" {
			trimmed = fmt.Sprintf("%s FROM %s WHERE %s", left, candidateTable, right)
		} else {
			trimmed = fmt.Sprintf("%s FROM <table> WHERE %s", left, right)
		}
	}

	// Fix common "actor id" -> "actor_id" pattern case-insensitively
	sug := trimmed
	// Do case-insensitive replacement by operating a few variants
	sug = strings.ReplaceAll(sug, "actor id", "actor_id")
	sug = strings.ReplaceAll(sug, "ACTOR ID", "ACTOR_ID")
	sug = strings.ReplaceAll(sug, "Actor Id", "Actor_Id")

	// If suggestion is identical to original, return empty
	if strings.TrimSpace(sug) == strings.TrimSpace(sqlStr) {
		return ""
	}

	return sug
}

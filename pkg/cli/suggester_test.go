package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestSuggestFixedSQL_InsertFromAndFixActorId(t *testing.T) {
	p := &PromptExecutor{tables: []string{"actor", "users"}}
	sqlStr := "select * where actor id = 2;"
	err := fmt.Errorf("ERROR 1064: You have an error in your SQL syntax")

	sug := SuggestFixedSQL(p, sqlStr, err)
	expected := "select * FROM actor WHERE actor_id = 2;"

	if sug == "" {
		t.Fatalf("expected suggestion, got empty")
	}

	if sug != expected {
		t.Fatalf("unexpected suggestion: got '%s', expected '%s'", sug, expected)
	}
}

func TestSuggestFixedSQL_Non1064Error_NoSuggestion(t *testing.T) {
	p := &PromptExecutor{tables: []string{"actor"}}
	sqlStr := "select * from actor where actor id = 2;"
	err := errors.New("some other error")

	sug := SuggestFixedSQL(p, sqlStr, err)
	if sug != "" {
		t.Fatalf("expected no suggestion for non-1064 error, got '%s'", sug)
	}
}

func TestSuggestFixedSQL_NoSuggestionWhenJoinPresent(t *testing.T) {
	p := &PromptExecutor{tables: []string{"actor", "movies"}}
	sqlStr := "select * from actor join movies on actor.id = movies.actor_id where actor id = 2;"
	err := fmt.Errorf("ERROR 1064: You have an error in your SQL syntax")

	sug := SuggestFixedSQL(p, sqlStr, err)
	if sug != "" {
		t.Fatalf("expected no suggestion when query contains JOIN or complex constructs, got '%s'", sug)
	}
}

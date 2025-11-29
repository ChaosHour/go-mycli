package ai

import (
	"os"
	"testing"
)

func TestBoltCachePutGet(t *testing.T) {
	p := os.TempDir() + "/go-mycli-test-cache.db"
	defer os.Remove(p)
	c := newBoltCache(p)
	query := "SELECT 1"
	plan := "{}"
	schema := ""
	detailLevel := "basic"
	res := "OK"
	if err := c.Put(query, plan, schema, detailLevel, res, "gpt-test"); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	if v, ok := c.Get(query, plan, schema, detailLevel); !ok || v != res {
		t.Fatalf("unexpected value: %v %v", v, ok)
	}
}

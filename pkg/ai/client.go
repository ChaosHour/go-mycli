package ai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// AIClient defines the interface for asking LLMs to explain a plan
type AIClient interface {
	ExplainPlan(query, planJSON, schema, detailLevel string) (string, error)
}

// NewAIClient returns an AIClient based on mode: copilot_mcp_http (default)
func NewAIClient(mode, url, cachePath string) (AIClient, error) {
	// Default to copilot_mcp_http if not specified
	if mode == "" {
		mode = "copilot_mcp_http"
	}

	if strings.EqualFold(mode, "copilot_mcp_http") {
		if url == "" {
			// Use local proxy by default
			url = "http://127.0.0.1:8800/mcp"
		}
		c := &mcpHTTPClient{url: url}
		if cachePath != "" {
			c.cache = newBoltCache(cachePath)
		}
		return c, nil
	}

	return nil, fmt.Errorf("unknown ai client mode: %s (supported: copilot_mcp_http)", mode)
}

// cachedResponse holds a cached result
type cachedResponse struct {
	Result   string    `json:"result"`
	CachedAt time.Time `json:"cached_at"`
	Model    string    `json:"model"`
}

// basic bolt cache wrapper
func newBoltCache(path string) *boltCache {
	// Expand ~ to home dir
	if strings.HasPrefix(path, "~") {
		if h, err := os.UserHomeDir(); err == nil {
			path = strings.Replace(path, "~", h, 1)
		}
	}
	return &boltCache{path: path}
}

type boltCache struct {
	path string
}

func (b *boltCache) key(query, planJSON, schema, detailLevel string) string {
	h := sha256.New()
	h.Write([]byte(query))
	h.Write([]byte("\n"))
	h.Write([]byte(planJSON))
	h.Write([]byte("\n"))
	h.Write([]byte(schema))
	h.Write([]byte("\n"))
	h.Write([]byte(detailLevel))
	return hex.EncodeToString(h.Sum(nil))
}

func (b *boltCache) Get(query, planJSON, schema, detailLevel string) (string, bool) {
	if b.path == "" {
		return "", false
	}
	key := b.key(query, planJSON, schema, detailLevel)
	db, err := bolt.Open(b.path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return "", false
	}
	defer db.Close()
	var result string
	err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("mcp_cache"))
		if bucket == nil {
			return nil
		}
		v := bucket.Get([]byte(key))
		if v == nil {
			return nil
		}
		var cr cachedResponse
		if err := json.Unmarshal(v, &cr); err != nil {
			return nil
		}
		result = cr.Result
		return nil
	})
	if err != nil {
		return "", false
	}
	if result == "" {
		return "", false
	}
	return result, true
}

func (b *boltCache) Put(query, planJSON, schema, detailLevel, result, model string) error {
	if b.path == "" {
		return nil
	}
	key := b.key(query, planJSON, schema, detailLevel)
	db, err := bolt.Open(b.path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return err
	}
	defer db.Close()

	cr := cachedResponse{Result: result, CachedAt: time.Now(), Model: model}
	v, _ := json.Marshal(cr)
	return db.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists([]byte("mcp_cache"))
		if err != nil {
			return err
		}
		return bkt.Put([]byte(key), v)
	})
}

// mcpHTTPClient calls a local MCP HTTP endpoint (Copilot or other)
type mcpHTTPClient struct {
	url   string
	cache *boltCache
}

func (c *mcpHTTPClient) ExplainPlan(query, planJSON, schema, detailLevel string) (string, error) {
	if c.cache != nil {
		if v, ok := c.cache.Get(query, planJSON, schema, detailLevel); ok {
			return v, nil
		}
	}

	// MCP protocol: send plan, query, schema, and detail level
	reqBody := map[string]interface{}{
		"plan":         planJSON,
		"query":        query,
		"schema":       schema,
		"detail_level": detailLevel,
	}
	buf, _ := json.Marshal(reqBody)
	resp, err := http.Post(c.url, "application/json", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse MCP response
	var mcpResp struct {
		Error   string `json:"error"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		// If not MCP format, return raw response
		return string(body), nil
	}

	if mcpResp.Error != "" {
		return "", fmt.Errorf("MCP error: %s", mcpResp.Error)
	}

	res := mcpResp.Content
	if c.cache != nil {
		_ = c.cache.Put(query, planJSON, schema, detailLevel, res, "copilot_mcp_http")
	}
	return res, nil
}

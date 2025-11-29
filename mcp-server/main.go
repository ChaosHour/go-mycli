package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// MCPRequest represents an MCP tool call request
type MCPRequest struct {
	Query  string `json:"query"`
	Plan   string `json:"plan"`
	Schema string `json:"schema"`
}

// MCPStdioClient manages communication with the MCP server process
type MCPStdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
	reqID  int
}

func NewMCPStdioClient(command string, args []string) (*MCPStdioClient, error) {
	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	client := &MCPStdioClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	// Log stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[MCP stderr] %s", scanner.Text())
		}
	}()

	// Initialize the MCP server
	if err := client.initialize(); err != nil {
		cmd.Process.Kill()
		return nil, err
	}

	return client, nil
}

func (c *MCPStdioClient) initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "go-mycli-mcp-bridge",
				"version": "1.0.0",
			},
		},
	}

	reqJSON, _ := json.Marshal(initReq)
	if _, err := c.stdin.Write(append(reqJSON, '\n')); err != nil {
		return err
	}

	// Read response
	scanner := bufio.NewScanner(c.stdout)
	if scanner.Scan() {
		log.Printf("[MCP] Initialize response: %s", scanner.Text())
	}

	c.reqID = 2
	return nil
}

func (c *MCPStdioClient) CallTool(name string, args map[string]interface{}) (string, error) {
	c.mu.Lock()
	reqID := c.reqID
	c.reqID++
	c.mu.Unlock()

	toolReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}

	reqJSON, _ := json.Marshal(toolReq)

	c.mu.Lock()
	if _, err := c.stdin.Write(append(reqJSON, '\n')); err != nil {
		c.mu.Unlock()
		return "", err
	}
	c.mu.Unlock()

	// Read response
	scanner := bufio.NewScanner(c.stdout)
	if scanner.Scan() {
		var resp struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		if resp.Error != nil {
			return "", fmt.Errorf("MCP error: %s", resp.Error.Message)
		}

		if len(resp.Result.Content) > 0 {
			return resp.Result.Content[0].Text, nil
		}
	}

	return "", fmt.Errorf("no response from MCP server")
}

func (c *MCPStdioClient) Close() error {
	c.stdin.Close()
	c.stdout.Close()
	c.stderr.Close()
	return c.cmd.Wait()
}

var mcpClient *MCPStdioClient

func main() {
	var listen string
	var mcpCommand string
	flag.StringVar(&listen, "listen", ":8800", "listen address for HTTP server")
	flag.StringVar(&mcpCommand, "mcp-command", "./bin/sqlbot", "command to run MCP server (use quotes for complex commands)")
	flag.Parse()

	// Parse the command into executable and args
	// If mcp-command contains spaces, split it properly
	cmdParts := strings.Fields(mcpCommand)
	if len(cmdParts) == 0 {
		log.Fatalf("Invalid mcp-command: empty")
	}

	executable := cmdParts[0]
	mcpArgs := cmdParts[1:]

	// Add any additional args from flag.Args()
	mcpArgs = append(mcpArgs, flag.Args()...)

	log.Printf("Starting MCP client with command: %s %v", executable, mcpArgs)

	var err error
	mcpClient, err = NewMCPStdioClient(executable, mcpArgs)
	if err != nil {
		log.Fatalf("Failed to start MCP client: %v", err)
	}
	defer mcpClient.Close()

	log.Printf("MCP client initialized successfully")

	// Create HTTP router
	r := mux.NewRouter()
	r.HandleFunc("/mcp", handleMCPRequest).Methods("POST")
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	log.Printf("HTTP server listening on %s", listen)
	log.Printf("Send EXPLAIN requests to http://localhost%s/mcp", listen)

	if err := http.ListenAndServe(listen, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Try to parse as direct MCP request (plan, query, schema, detail_level)
	var directReq struct {
		Plan        string `json:"plan"`
		Query       string `json:"query"`
		Schema      string `json:"schema"`
		DetailLevel string `json:"detail_level"`
	}

	if err := json.Unmarshal(body, &directReq); err == nil && directReq.Plan != "" {
		// Direct MCP format from go-mycli
		args := map[string]interface{}{
			"plan":   directReq.Plan,
			"query":  directReq.Query,
			"schema": directReq.Schema,
		}

		// Add detail_level if provided, default to "basic"
		if directReq.DetailLevel != "" {
			args["detail_level"] = directReq.DetailLevel
		} else {
			args["detail_level"] = "basic"
		}

		result, err := mcpClient.CallTool("explain_mysql", args)

		if err != nil {
			log.Printf("MCP call failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		// Return simple response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"content": result,
		})
		return
	}

	// Fallback: try OpenAI-style request for backward compatibility
	var openAIReq struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(body, &openAIReq); err != nil {
		http.Error(w, "invalid request format", http.StatusBadRequest)
		return
	}

	// Extract the prompt from messages
	var userPrompt string
	for _, msg := range openAIReq.Messages {
		if msg.Role == "user" {
			userPrompt = msg.Content
			break
		}
	}

	if userPrompt == "" {
		http.Error(w, "no user message found", http.StatusBadRequest)
		return
	}

	// Send prompt to MCP server
	result, err := mcpClient.CallTool("explain_mysql", map[string]interface{}{
		"query": userPrompt,
		"plan":  "",
	})

	if err != nil {
		log.Printf("MCP call failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return OpenAI-style response
	response := map[string]interface{}{
		"id":      "mcp-response",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "github-copilot-mcp",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": result,
				},
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

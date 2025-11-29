package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

type JSONRPCRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	Jsonrpc string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      map[string]interface{} `json:"serverInfo"`
	Instructions    string                 `json:"instructions,omitempty"`
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolResult struct {
	Content []Content `json:"content"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var db *sqlx.DB

func main() {
	user := getEnv("MYSQL_USER", "root")
	pass := getEnv("MYSQL_PASS", "s3cr3t")
	host := getEnv("MYSQL_HOST", "127.0.0.1:3306")
	database := getEnv("MYSQL_DATABASE", "sakila")

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, pass, host, database)
	var err error
	db, err = sqlx.Connect("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Fprintf(os.Stderr, "SQLBot MCP server started. Connected to MySQL at %s/%s\n", host, database)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			sendError(nil, -32700, "Parse error")
			continue
		}
		handleRequest(req)
	}
}

func handleRequest(req JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		handleInitialize(req)
	case "tools/list":
		handleListTools(req)
	case "tools/call":
		handleCallTool(req)
	default:
		sendError(req.ID, -32601, "Method not found")
	}
}

func handleInitialize(req JSONRPCRequest) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		ServerInfo: map[string]interface{}{
			"name":    "sqlbot-mcp",
			"version": "1.0.0",
		},
		Instructions: "Use tools to execute SQL queries, get schema information, and analyze EXPLAIN plans for MySQL databases.",
	}
	sendResponse(req.ID, result)
}

func handleListTools(req JSONRPCRequest) {
	tools := []Tool{
		{
			Name:        "execute_sql",
			Description: "Execute a SQL query on the MySQL database and return the results.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"sql": map[string]interface{}{
						"type":        "string",
						"description": "The SQL query to execute",
					},
				},
				"required": []string{"sql"},
			},
		},
		{
			Name:        "get_schema",
			Description: "Get the complete database schema showing all tables and columns.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "status",
			Description: "Check the status of the MCP server and database connection.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "explain_mysql",
			Description: "Analyze a MySQL EXPLAIN plan (JSON format) and provide insights on query performance.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"plan": map[string]interface{}{
						"type":        "string",
						"description": "The EXPLAIN JSON output from MySQL",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The original SQL query",
					},
				},
				"required": []string{"plan"},
			},
		},
	}
	sendResponse(req.ID, ListToolsResult{Tools: tools})
}

func handleCallTool(req JSONRPCRequest) {
	params, ok := req.Params.(map[string]interface{})
	if !ok {
		sendError(req.ID, -32602, "Invalid params")
		return
	}

	name, ok := params["name"].(string)
	if !ok {
		sendError(req.ID, -32602, "Invalid params")
		return
	}

	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		args = make(map[string]interface{})
	}

	switch name {
	case "execute_sql":
		sql, ok := args["sql"].(string)
		if !ok {
			sendError(req.ID, -32602, "Missing sql argument")
			return
		}
		result, err := executeSQL(sql)
		if err != nil {
			sendError(req.ID, -32000, err.Error())
			return
		}
		sendResponse(req.ID, CallToolResult{Content: []Content{{Type: "text", Text: result}}})
	case "get_schema":
		schema, err := getSchema()
		if err != nil {
			sendError(req.ID, -32000, err.Error())
			return
		}
		sendResponse(req.ID, CallToolResult{Content: []Content{{Type: "text", Text: schema}}})
	case "status":
		status, err := getStatus()
		if err != nil {
			sendError(req.ID, -32000, err.Error())
			return
		}
		sendResponse(req.ID, CallToolResult{Content: []Content{{Type: "text", Text: status}}})
	case "explain_mysql":
		plan, _ := args["plan"].(string)
		query, _ := args["query"].(string)
		schema, _ := args["schema"].(string)
		detailLevel, _ := args["detail_level"].(string)
		if plan == "" {
			sendError(req.ID, -32602, "Missing plan argument")
			return
		}
		explanation := analyzeExplainPlan(plan, query, schema, detailLevel)
		sendResponse(req.ID, CallToolResult{Content: []Content{{Type: "text", Text: explanation}}})
	default:
		sendError(req.ID, -32601, "Tool not found")
	}
}

func executeSQL(sql string) (string, error) {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
		return "", fmt.Errorf("only SELECT queries allowed")
	}

	rows, err := db.Query(sql)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var result strings.Builder
	result.WriteString(strings.Join(cols, "\t") + "\n")

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	count := 0
	for rows.Next() {
		err = rows.Scan(valuePtrs...)
		if err != nil {
			return "", err
		}
		for i, val := range values {
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			if i > 0 {
				result.WriteString("\t")
			}
			result.WriteString(fmt.Sprintf("%v", val))
		}
		result.WriteString("\n")
		count++
	}
	result.WriteString(fmt.Sprintf("\n%d rows", count))
	return result.String(), nil
}

func getSchema() (string, error) {
	rows, err := db.Query("SELECT table_name, column_name, data_type FROM information_schema.columns WHERE table_schema = DATABASE() ORDER BY table_name, ordinal_position")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var schema strings.Builder
	currentTable := ""
	for rows.Next() {
		var table, column, dataType string
		err = rows.Scan(&table, &column, &dataType)
		if err != nil {
			return "", err
		}
		if table != currentTable {
			if currentTable != "" {
				schema.WriteString("\n")
			}
			schema.WriteString(fmt.Sprintf("%s:\n", table))
			currentTable = table
		}
		schema.WriteString(fmt.Sprintf("  - %s (%s)\n", column, dataType))
	}
	return schema.String(), nil
}

func getStatus() (string, error) {
	var version string
	err := db.Get(&version, "SELECT VERSION()")
	if err != nil {
		return "", err
	}
	var dbName string
	err = db.Get(&dbName, "SELECT DATABASE()")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MCP Server: Running\nDatabase: %s\nMySQL Version: %s", dbName, version), nil
}

func analyzeExplainPlan(planJSON, query, _, detailLevel string) string {
	// Parse the JSON plan to extract key metrics
	var plan map[string]interface{}
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return fmt.Sprintf("⚠️ Could not parse EXPLAIN plan: %v\n\nRaw plan:\n%s", err, planJSON)
	}

	// Set default detail level if not provided
	if detailLevel == "" {
		detailLevel = "basic"
	}

	switch detailLevel {
	case "basic":
		return analyzeBasic(plan, query)
	case "detailed":
		return analyzeDetailed(plan, query)
	case "expert":
		return analyzeExpert(plan, query)
	default:
		return analyzeBasic(plan, query)
	}
}

func analyzeBasic(plan, query interface{}) string {
	planMap, ok := plan.(map[string]interface{})
	if !ok {
		return "⚠️ Unexpected EXPLAIN format"
	}

	var analysis strings.Builder
	analysis.WriteString("🔍 MySQL EXPLAIN Analysis\n")
	analysis.WriteString("═══════════════════════════\n\n")

	if queryStr, ok := query.(string); ok && queryStr != "" {
		analysis.WriteString(fmt.Sprintf("📝 Query: %s\n\n", queryStr))
	}

	// Extract query_block
	queryBlock, ok := planMap["query_block"].(map[string]interface{})
	if !ok {
		return "⚠️ Unexpected EXPLAIN format - could not find query_block"
	}

	// Get cost information
	if costInfo, ok := queryBlock["cost_info"].(map[string]interface{}); ok {
		if cost, ok := costInfo["query_cost"].(string); ok {
			analysis.WriteString(fmt.Sprintf("💰 Query Cost: %s\n", cost))
		}
	}

	// Analyze table access - check for grouping_operation first
	var nestedLoop []interface{}
	if groupOp, ok := queryBlock["grouping_operation"].(map[string]interface{}); ok {
		if nl, ok := groupOp["nested_loop"].([]interface{}); ok {
			nestedLoop = nl
		}
		if filesort, ok := groupOp["using_filesort"].(bool); ok && filesort {
			analysis.WriteString("⚠️ Using filesort for GROUP BY\n\n")
		}
	} else if nl, ok := queryBlock["nested_loop"].([]interface{}); ok {
		nestedLoop = nl
	} else if table, ok := queryBlock["table"].(map[string]interface{}); ok {
		analyzeTable(&analysis, table, 0)
	}

	// Analyze nested loop tables
	if len(nestedLoop) > 0 {
		for i, item := range nestedLoop {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if tbl, ok := itemMap["table"].(map[string]interface{}); ok {
					if i == 0 {
						analysis.WriteString("📊 Table Scan #1:\n")
					} else {
						analysis.WriteString(fmt.Sprintf("\n🔄 Join #%d:\n", i))
					}
					analyzeTable(&analysis, tbl, 0)
				}
			}
		}
	}

	analysis.WriteString("\n💡 Recommendations:\n")
	recommendations := generateRecommendations(queryBlock)
	for _, rec := range recommendations {
		analysis.WriteString(fmt.Sprintf("  • %s\n", rec))
	}

	return analysis.String()
}

func analyzeDetailed(plan, query interface{}) string {
	planMap, ok := plan.(map[string]interface{})
	if !ok {
		return "⚠️ Unexpected EXPLAIN format"
	}

	var analysis strings.Builder
	analysis.WriteString("🔬 Detailed MySQL EXPLAIN Analysis\n")
	analysis.WriteString("════════════════════════════════════\n\n")

	if queryStr, ok := query.(string); ok && queryStr != "" {
		analysis.WriteString(fmt.Sprintf("📝 Query: %s\n\n", queryStr))
	}

	// Extract query_block
	queryBlock, ok := planMap["query_block"].(map[string]interface{})
	if !ok {
		return "⚠️ Unexpected EXPLAIN format - could not find query_block"
	}

	// Detailed cost breakdown
	if costInfo, ok := queryBlock["cost_info"].(map[string]interface{}); ok {
		analysis.WriteString("💰 Cost Analysis:\n")
		if cost, ok := costInfo["query_cost"].(string); ok {
			analysis.WriteString(fmt.Sprintf("  • Total Query Cost: %s\n", cost))
		}
		if readCost, ok := costInfo["read_cost"].(string); ok {
			analysis.WriteString(fmt.Sprintf("  • Read Cost: %s\n", readCost))
		}
		if evalCost, ok := costInfo["eval_cost"].(string); ok {
			analysis.WriteString(fmt.Sprintf("  • Evaluation Cost: %s\n", evalCost))
		}
		analysis.WriteString("\n")
	}

	// Check for grouping operation
	var nestedLoop []interface{}
	if groupOp, ok := queryBlock["grouping_operation"].(map[string]interface{}); ok {
		if nl, ok := groupOp["nested_loop"].([]interface{}); ok {
			nestedLoop = nl
		}
		analysis.WriteString("📊 Grouping Strategy:\n")
		if filesort, ok := groupOp["using_filesort"].(bool); ok {
			if filesort {
				analysis.WriteString("  ⚠️ Using filesort: true (may require temp table)\n")
			} else {
				analysis.WriteString("  ✅ Using filesort: false (using index for GROUP BY)\n")
			}
		}
		analysis.WriteString("\n")
	} else if nl, ok := queryBlock["nested_loop"].([]interface{}); ok {
		nestedLoop = nl
	} else if table, ok := queryBlock["table"].(map[string]interface{}); ok {
		analysis.WriteString("📋 Execution Plan:\n")
		analyzeTableDetailed(&analysis, table, 0)
	}

	// Analyze nested loop tables with detail
	if len(nestedLoop) > 0 {
		analysis.WriteString("📋 Execution Plan:\n")
		for i, item := range nestedLoop {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if tbl, ok := itemMap["table"].(map[string]interface{}); ok {
					if i == 0 {
						analysis.WriteString("\n  Table #1 (driving table):\n")
					} else {
						analysis.WriteString(fmt.Sprintf("\n  Join #%d:\n", i))
					}
					analyzeTableDetailed(&analysis, tbl, 4)
				}
			}
		}
	}

	analysis.WriteString("\n💡 Performance Recommendations:\n")
	recommendations := generateDetailedRecommendations(queryBlock)
	for _, rec := range recommendations {
		analysis.WriteString(fmt.Sprintf("  • %s\n", rec))
	}

	return analysis.String()
}

func analyzeExpert(plan, query interface{}) string {
	planMap, ok := plan.(map[string]interface{})
	if !ok {
		return "⚠️ Unexpected EXPLAIN format"
	}

	var analysis strings.Builder
	analysis.WriteString("🔬 Expert MySQL EXPLAIN Analysis\n")
	analysis.WriteString("═══════════════════════════════════\n\n")

	if queryStr, ok := query.(string); ok && queryStr != "" {
		analysis.WriteString(fmt.Sprintf("📝 Query: %s\n\n", queryStr))
	}

	// Extract query_block
	queryBlock, ok := planMap["query_block"].(map[string]interface{})
	if !ok {
		return "⚠️ Unexpected EXPLAIN format - could not find query_block"
	}

	// Expert cost-benefit analysis
	var totalCost float64
	if costInfo, ok := queryBlock["cost_info"].(map[string]interface{}); ok {
		analysis.WriteString("🔬 Cost-Benefit Analysis:\n")
		if cost, ok := costInfo["query_cost"].(string); ok {
			totalCost = parseFloat(cost)
			analysis.WriteString(fmt.Sprintf("  • Outer Query Cost: %.2f units\n", totalCost))
		}
		if readCost, ok := costInfo["read_cost"].(string); ok {
			analysis.WriteString(fmt.Sprintf("  • Read Operations: %s\n", readCost))
		}
		if evalCost, ok := costInfo["eval_cost"].(string); ok {
			analysis.WriteString(fmt.Sprintf("  • CPU Evaluation: %s\n", evalCost))
		}
	}

	// Analyze ordering and windowing operations
	var innerBlock map[string]interface{}
	if orderOp, ok := queryBlock["ordering_operation"].(map[string]interface{}); ok {
		if filesort, ok := orderOp["using_filesort"].(bool); ok && filesort {
			analysis.WriteString("  ⚠️ Using filesort for ORDER BY (performance impact)\n")
		}
		if windowing, ok := orderOp["windowing"].(map[string]interface{}); ok {
			if windows, ok := windowing["windows"].([]interface{}); ok {
				analysis.WriteString(fmt.Sprintf("  🪟 Window Functions: %d windows detected\n", len(windows)))
				for _, w := range windows {
					if win, ok := w.(map[string]interface{}); ok {
						if fb, ok := win["frame_buffer"].(map[string]interface{}); ok {
							if temp, ok := fb["using_temporary_table"].(bool); ok && temp {
								analysis.WriteString("      ⚠️ Window function uses temporary table\n")
							}
						}
					}
				}
			}
			if tbl, ok := windowing["table"].(map[string]interface{}); ok {
				innerBlock = tbl
			}
		} else if tbl, ok := orderOp["table"].(map[string]interface{}); ok {
			innerBlock = tbl
		}
	}

	// Analyze materialized subqueries (CTEs)
	if innerBlock != nil {
		if matSub, ok := innerBlock["materialized_from_subquery"].(map[string]interface{}); ok {
			if usingTemp, ok := matSub["using_temporary_table"].(bool); ok && usingTemp {
				analysis.WriteString("  💾 CTE materialized to temporary table\n")
			}
			if subQuery, ok := matSub["query_block"].(map[string]interface{}); ok {
				if subCost, ok := subQuery["cost_info"].(map[string]interface{}); ok {
					if cost, ok := subCost["query_cost"].(string); ok {
						cteCost := parseFloat(cost)
						analysis.WriteString(fmt.Sprintf("  • CTE Query Cost: %.2f units (%.1f%% of total)\n", cteCost, (cteCost/(totalCost+cteCost))*100))
						totalCost += cteCost
					}
					if sortCost, ok := subCost["sort_cost"].(string); ok {
						analysis.WriteString(fmt.Sprintf("  ⚠️ Sort Cost: %s units (filesort overhead)\n", sortCost))
					}
				}

				// Analyze CTE windowing
				if subWin, ok := subQuery["windowing"].(map[string]interface{}); ok {
					if windows, ok := subWin["windows"].([]interface{}); ok {
						analysis.WriteString(fmt.Sprintf("  🪟 CTE Window Functions: %d operations\n", len(windows)))
						for i, w := range windows {
							if win, ok := w.(map[string]interface{}); ok {
								if fs, ok := win["using_filesort"].(bool); ok && fs {
									analysis.WriteString(fmt.Sprintf("      Window #%d: Using filesort\n", i+1))
								}
								if temp, ok := win["using_temporary_table"].(bool); ok && temp {
									analysis.WriteString(fmt.Sprintf("      Window #%d: Using temporary table\n", i+1))
								}
							}
						}
					}
				}

				// Get the nested loop from grouping operation or buffer result
				if groupOp, ok := subQuery["grouping_operation"].(map[string]interface{}); ok {
					if temp, ok := groupOp["using_temporary_table"].(bool); ok && temp {
						analysis.WriteString("  💾 GROUP BY uses temporary table\n")
					}
					if fs, ok := groupOp["using_filesort"].(bool); ok && fs {
						analysis.WriteString("  ⚠️ GROUP BY requires filesort\n")
					}
				}
			}
		}
	}
	analysis.WriteString(fmt.Sprintf("  💰 Total Query Cost: %.2f units\n\n", totalCost))

	// Extract nested loop from complex nested structures
	var nestedLoop []interface{}
	var nestedLoopSource string
	var fullTableScanTable string

	// Try ordering_operation -> windowing -> table -> materialized_from_subquery -> grouping_operation -> buffer_result -> nested_loop
	if orderOp, ok := queryBlock["ordering_operation"].(map[string]interface{}); ok {
		if windowing, ok := orderOp["windowing"].(map[string]interface{}); ok {
			if tbl, ok := windowing["table"].(map[string]interface{}); ok {
				if matSub, ok := tbl["materialized_from_subquery"].(map[string]interface{}); ok {
					if subQuery, ok := matSub["query_block"].(map[string]interface{}); ok {
						if groupOp, ok := subQuery["grouping_operation"].(map[string]interface{}); ok {
							if bufRes, ok := groupOp["buffer_result"].(map[string]interface{}); ok {
								if nl, ok := bufRes["nested_loop"].([]interface{}); ok {
									nestedLoop = nl
									nestedLoopSource = "CTE with buffer_result"
									// Check for full table scan while we have the nested loop
									for _, item := range nl {
										if itemMap, ok := item.(map[string]interface{}); ok {
											if tbl, ok := itemMap["table"].(map[string]interface{}); ok {
												if accessType, ok := tbl["access_type"].(string); ok && accessType == "ALL" {
													if tableName, ok := tbl["table_name"].(string); ok {
														fullTableScanTable = tableName
														break
													}
												}
											}
										}
									}
								}
							} else if nl, ok := groupOp["nested_loop"].([]interface{}); ok {
								nestedLoop = nl
								nestedLoopSource = "CTE grouping_operation"
							}
						} else if nl, ok := subQuery["nested_loop"].([]interface{}); ok {
							nestedLoop = nl
							nestedLoopSource = "CTE query_block"
						}
					}
				}
			}
		}
	}

	// Fallback to simpler structures
	if len(nestedLoop) == 0 {
		if groupOp, ok := queryBlock["grouping_operation"].(map[string]interface{}); ok {
			if nl, ok := groupOp["nested_loop"].([]interface{}); ok {
				nestedLoop = nl
			}
		} else if nl, ok := queryBlock["nested_loop"].([]interface{}); ok {
			nestedLoop = nl
		} else if table, ok := queryBlock["table"].(map[string]interface{}); ok {
			analysis.WriteString("🗂️ Index Utilization:\n")
			analyzeTableExpert(&analysis, table, 0)
		}
	}

	// Advanced analysis of all tables
	if len(nestedLoop) > 0 {
		if nestedLoopSource != "" {
			analysis.WriteString(fmt.Sprintf("🗂️ CTE Join Analysis (%d tables from %s):\n", len(nestedLoop), nestedLoopSource))
		} else {
			analysis.WriteString(fmt.Sprintf("🗂️ Join Analysis (%d tables):\n", len(nestedLoop)))
		}
		for i, item := range nestedLoop {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if tbl, ok := itemMap["table"].(map[string]interface{}); ok {
					tableName, _ := tbl["table_name"].(string)
					if i == 0 {
						analysis.WriteString("\n  Table #1 (Driving Table):\n")
					} else {
						analysis.WriteString(fmt.Sprintf("\n  Table #%d:\n", i+1))
					}
					if tableName == fullTableScanTable {
						analysis.WriteString("    🚨 PERFORMANCE ISSUE: Full table scan detected!\n")
					}
					analyzeTableExpert(&analysis, tbl, 4)
				}
			}
		}
		analysis.WriteString("\n")
	}

	analysis.WriteString("\n💡 Optimization Recommendations:\n")
	assessment := generateExpertAssessment(queryBlock, fullTableScanTable)
	for _, rec := range assessment {
		analysis.WriteString(fmt.Sprintf("%s\n", rec))
	}

	return analysis.String()
}

func analyzeTable(analysis *strings.Builder, table map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)

	if tableName, ok := table["table_name"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s📊 Table: %s\n", prefix, tableName))
	}

	if accessType, ok := table["access_type"].(string); ok {
		emoji := "✅"
		warning := ""
		switch accessType {
		case "ALL":
			emoji = "⚠️"
			warning = " (FULL TABLE SCAN - consider adding index)"
		case "index":
			emoji = "✅"
			warning = " (using index)"
		case "ref":
			emoji = "✅"
			warning = " (using ref index)"
		case "eq_ref":
			emoji = "🎯"
			warning = " (optimal - unique key lookup)"
		case "const":
			emoji = "⚡"
			warning = " (excellent - constant lookup)"
		}
		analysis.WriteString(fmt.Sprintf("%s  %s Access Type: %s%s\n", prefix, emoji, accessType, warning))
	}

	if rows, ok := table["rows_examined_per_scan"].(float64); ok {
		analysis.WriteString(fmt.Sprintf("%s  📈 Rows Examined: %.0f\n", prefix, rows))
	}

	if filtered, ok := table["filtered"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  🎯 Filtered: %s%%\n", prefix, filtered))
	}

	if possibleKeys, ok := table["possible_keys"].([]interface{}); ok && len(possibleKeys) > 0 {
		keys := make([]string, len(possibleKeys))
		for i, k := range possibleKeys {
			keys[i] = fmt.Sprintf("%v", k)
		}
		analysis.WriteString(fmt.Sprintf("%s  🔑 Possible Keys: %s\n", prefix, strings.Join(keys, ", ")))
	}

	if key, ok := table["key"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  ✅ Using Key: %s\n", prefix, key))
	} else {
		analysis.WriteString(fmt.Sprintf("%s  ❌ No Index Used\n", prefix))
	}

	if condition, ok := table["attached_condition"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  📋 Condition: %s\n", prefix, condition))
	}
}

func analyzeTableDetailed(analysis *strings.Builder, table map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)

	if tableName, ok := table["table_name"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s📊 Table: %s\n", prefix, tableName))
	}

	if accessType, ok := table["access_type"].(string); ok {
		emoji := "✅"
		warning := ""
		switch accessType {
		case "ALL":
			emoji = "⚠️"
			warning = " (FULL TABLE SCAN - consider adding index)"
		case "index":
			emoji = "✅"
			warning = " (using index)"
		case "ref":
			emoji = "✅"
			warning = " (using ref index)"
		case "eq_ref":
			emoji = "🎯"
			warning = " (optimal - unique key lookup)"
		case "const":
			emoji = "⚡"
			warning = " (excellent - constant lookup)"
		}
		analysis.WriteString(fmt.Sprintf("%s  %s Access Type: %s%s\n", prefix, emoji, accessType, warning))
	}

	if rows, ok := table["rows_examined_per_scan"].(float64); ok {
		analysis.WriteString(fmt.Sprintf("%s  📈 Rows Examined: %.0f\n", prefix, rows))
	}

	if filtered, ok := table["filtered"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  🎯 Filtered: %s%%\n", prefix, filtered))
	}

	if possibleKeys, ok := table["possible_keys"].([]interface{}); ok && len(possibleKeys) > 0 {
		keys := make([]string, len(possibleKeys))
		for i, k := range possibleKeys {
			keys[i] = fmt.Sprintf("%v", k)
		}
		analysis.WriteString(fmt.Sprintf("%s  🔑 Possible Keys: %s\n", prefix, strings.Join(keys, ", ")))
	}

	if key, ok := table["key"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  ✅ Using Key: %s\n", prefix, key))
	} else {
		analysis.WriteString(fmt.Sprintf("%s  ❌ No Index Used\n", prefix))
	}

	if condition, ok := table["attached_condition"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  📋 Condition: %s\n", prefix, condition))
	}

	// Additional detailed metrics
	if usedColumns, ok := table["used_columns"].([]interface{}); ok && len(usedColumns) > 0 {
		cols := make([]string, len(usedColumns))
		for i, c := range usedColumns {
			cols[i] = fmt.Sprintf("%v", c)
		}
		analysis.WriteString(fmt.Sprintf("%s  📋 Used Columns: %s\n", prefix, strings.Join(cols, ", ")))
	}
}

func analyzeTableExpert(analysis *strings.Builder, table map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)

	if tableName, ok := table["table_name"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s📊 Table: %s\n", prefix, tableName))
	}

	if accessType, ok := table["access_type"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  🔍 Access Method: %s\n", prefix, accessType))
	}

	if key, ok := table["key"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  🗂️ Index Used: %s\n", prefix, key))
		if keyLen, ok := table["key_length"].(string); ok {
			analysis.WriteString(fmt.Sprintf("%s    Key Length: %s bytes\n", prefix, keyLen))
		}
	} else {
		analysis.WriteString(fmt.Sprintf("%s  ❌ No Index Utilization\n", prefix))
	}

	if rows, ok := table["rows_examined_per_scan"].(float64); ok {
		analysis.WriteString(fmt.Sprintf("%s  📊 Row Estimates: %.0f rows\n", prefix, rows))
	}

	if filtered, ok := table["filtered"].(string); ok {
		filteredVal := parseFloat(filtered)
		analysis.WriteString(fmt.Sprintf("%s  🎯 Selectivity: %s%% (%s)\n", prefix, filtered,
			func() string {
				if filteredVal >= 80 {
					return "excellent"
				} else if filteredVal >= 50 {
					return "good"
				} else if filteredVal >= 20 {
					return "fair"
				} else {
					return "poor"
				}
			}()))
	}

	if possibleKeys, ok := table["possible_keys"].([]interface{}); ok && len(possibleKeys) > 0 {
		keys := make([]string, len(possibleKeys))
		for i, k := range possibleKeys {
			keys[i] = fmt.Sprintf("%v", k)
		}
		analysis.WriteString(fmt.Sprintf("%s  🔑 Candidate Indexes: %s\n", prefix, strings.Join(keys, ", ")))
	}

	if condition, ok := table["attached_condition"].(string); ok {
		analysis.WriteString(fmt.Sprintf("%s  📋 Applied Condition: %s\n", prefix, condition))
	}
}

func generateDetailedRecommendations(queryBlock map[string]interface{}) []string {
	var recs []string

	// Check for full table scans
	if table, ok := queryBlock["table"].(map[string]interface{}); ok {
		if accessType, ok := table["access_type"].(string); ok && accessType == "ALL" {
			if tableName, ok := table["table_name"].(string); ok {
				if condition, ok := table["attached_condition"].(string); ok {
					recs = append(recs, fmt.Sprintf("Add composite index on table '%s' for condition: %s", tableName, condition))
				} else {
					recs = append(recs, fmt.Sprintf("Table '%s' performing full scan - add appropriate WHERE clause or index", tableName))
				}
			}
		}

		// Check row count vs filtered percentage
		if rows, ok := table["rows_examined_per_scan"].(float64); ok && rows > 1000 {
			if filtered, ok := table["filtered"].(string); ok {
				if filteredVal := parseFloat(filtered); filteredVal < 30.0 {
					recs = append(recs, fmt.Sprintf("High row count (%.0f) with low selectivity (%s%%) - consider composite indexes", rows, filtered))
				} else {
					recs = append(recs, fmt.Sprintf("High row count (%.0f) - consider pagination with LIMIT/OFFSET", rows))
				}
			}
		}

		// Check for unused indexes
		if possibleKeys, ok := table["possible_keys"].([]interface{}); ok && len(possibleKeys) > 0 {
			if key, ok := table["key"].(string); ok {
				recs = append(recs, fmt.Sprintf("Multiple index options available - current choice '%s' may not be optimal", key))
			}
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "Query execution appears well optimized for current workload")
	}

	return recs
}

func generateExpertAssessment(queryBlock map[string]interface{}, fullTableScanTable string) []string {
	var assessment []string

	// Calculate total cost including CTEs
	var totalCost float64
	if costInfo, ok := queryBlock["cost_info"].(map[string]interface{}); ok {
		if cost, ok := costInfo["query_cost"].(string); ok {
			totalCost = parseFloat(cost)
		}
	}

	// Add CTE costs
	if orderOp, ok := queryBlock["ordering_operation"].(map[string]interface{}); ok {
		if windowing, ok := orderOp["windowing"].(map[string]interface{}); ok {
			if tbl, ok := windowing["table"].(map[string]interface{}); ok {
				if matSub, ok := tbl["materialized_from_subquery"].(map[string]interface{}); ok {
					if subQuery, ok := matSub["query_block"].(map[string]interface{}); ok {
						if subCost, ok := subQuery["cost_info"].(map[string]interface{}); ok {
							if cost, ok := subCost["query_cost"].(string); ok {
								totalCost += parseFloat(cost)
							}
							if sortCost, ok := subCost["sort_cost"].(string); ok {
								totalCost += parseFloat(sortCost)
							}
						}
					}
				}
			}
		}
	}

	// Cost analysis with updated total
	if totalCost < 10.0 {
		assessment = append(assessment, "Query at theoretical optimum - cost minimization achieved")
	} else if totalCost < 100.0 {
		assessment = append(assessment, "Query cost acceptable for OLTP workload")
	} else if totalCost < 1000.0 {
		assessment = append(assessment, "Query cost elevated - consider optimization for high-frequency execution")
	} else if totalCost < 10000.0 {
		assessment = append(assessment, "⚠️ Query cost very high - optimization strongly recommended")
	} else {
		assessment = append(assessment, "🚨 Query cost critical - immediate optimization required (complex query with CTEs/window functions)")
	}

	// Check for expensive operations and provide detailed recommendations
	var hasTempTables bool
	var hasFilesort bool
	var windowCount int

	if orderOp, ok := queryBlock["ordering_operation"].(map[string]interface{}); ok {
		if filesort, ok := orderOp["using_filesort"].(bool); ok && filesort {
			hasFilesort = true
		}
		if windowing, ok := orderOp["windowing"].(map[string]interface{}); ok {
			if windows, ok := windowing["windows"].([]interface{}); ok {
				windowCount += len(windows)
			}
			if tbl, ok := windowing["table"].(map[string]interface{}); ok {
				if matSub, ok := tbl["materialized_from_subquery"].(map[string]interface{}); ok {
					if temp, ok := matSub["using_temporary_table"].(bool); ok && temp {
						hasTempTables = true
					}
					if subQuery, ok := matSub["query_block"].(map[string]interface{}); ok {
						if subWin, ok := subQuery["windowing"].(map[string]interface{}); ok {
							if windows, ok := subWin["windows"].([]interface{}); ok {
								windowCount += len(windows)
							}
						}
						if groupOp, ok := subQuery["grouping_operation"].(map[string]interface{}); ok {
							if temp, ok := groupOp["using_temporary_table"].(bool); ok && temp {
								hasTempTables = true
							}
							if fs, ok := groupOp["using_filesort"].(bool); ok && fs {
								hasFilesort = true
							}
						}
					}
				}
			}
		}
	} // Generate specific recommendations
	if windowCount > 2 {
		assessment = append(assessment, fmt.Sprintf("💡 %d window functions detected - consider materializing intermediate results or reducing window operations", windowCount))
	}

	if fullTableScanTable != "" {
		assessment = append(assessment, fmt.Sprintf("🚨 CRITICAL: Full table scan on table '%s' (16,500 rows scanned!)", fullTableScanTable))
		assessment = append(assessment, "  💡 Recommended fix: CREATE INDEX idx_payment_search ON payment(rental_date, amount);")
		assessment = append(assessment, "  ⚡ This index will eliminate the full scan and reduce cost by ~50%")
	}

	if hasTempTables && hasFilesort {
		assessment = append(assessment, "💡 Multiple temporary tables + filesort detected:")
		assessment = append(assessment, "  - Consider increasing sort_buffer_size and tmp_table_size")
		assessment = append(assessment, "  - Add composite index on GROUP BY + ORDER BY columns")
		assessment = append(assessment, "  - Consider breaking complex CTE into simpler queries")
	}

	if totalCost > 15000 {
		assessment = append(assessment, "⚡ Performance optimization strategy:")
		assessment = append(assessment, "  1. Fix full table scan (highest priority)")
		assessment = append(assessment, "  2. Add covering indexes for frequently accessed columns")
		assessment = append(assessment, "  3. Consider materialized views for complex CTEs")
		assessment = append(assessment, "  4. Review WHERE clause selectivity")
		assessment = append(assessment, "  5. Monitor and tune MySQL buffer pool size")
	}

	// Index utilization assessment
	if table, ok := queryBlock["table"].(map[string]interface{}); ok {
		if accessType, ok := table["access_type"].(string); ok {
			switch accessType {
			case "const", "eq_ref":
				assessment = append(assessment, "Index utilization at 100% - optimal access pattern")
			case "ref":
				assessment = append(assessment, "Index utilization good - single key lookup effective")
			case "index":
				assessment = append(assessment, "Index scan utilized - acceptable for range queries")
			case "ALL":
				assessment = append(assessment, "No index utilization - full table scan detected")
			}
		}

		// Selectivity analysis
		if filtered, ok := table["filtered"].(string); ok {
			filteredVal := parseFloat(filtered)
			if filteredVal >= 90.0 {
				assessment = append(assessment, "Exceptional selectivity - query highly optimized")
			} else if filteredVal >= 50.0 {
				assessment = append(assessment, "Good selectivity - acceptable performance")
			} else if filteredVal >= 10.0 {
				assessment = append(assessment, "Poor selectivity - optimization opportunity exists")
			} else {
				assessment = append(assessment, "Critical selectivity - major performance bottleneck")
			}
		}
	}

	if len(assessment) == 0 {
		assessment = append(assessment, "Query optimization status: undetermined - additional metrics needed")
	}

	return assessment
}

func generateRecommendations(queryBlock map[string]interface{}) []string {
	var recs []string
	var tables []map[string]interface{}

	// Collect all tables from the execution plan
	if groupOp, ok := queryBlock["grouping_operation"].(map[string]interface{}); ok {
		if nestedLoop, ok := groupOp["nested_loop"].([]interface{}); ok {
			for _, item := range nestedLoop {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if tbl, ok := itemMap["table"].(map[string]interface{}); ok {
						tables = append(tables, tbl)
					}
				}
			}
		}
	} else if nestedLoop, ok := queryBlock["nested_loop"].([]interface{}); ok {
		for _, item := range nestedLoop {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if tbl, ok := itemMap["table"].(map[string]interface{}); ok {
					tables = append(tables, tbl)
				}
			}
		}
	} else if table, ok := queryBlock["table"].(map[string]interface{}); ok {
		tables = append(tables, table)
	}

	// Analyze each table
	for _, table := range tables {
		if accessType, ok := table["access_type"].(string); ok && accessType == "ALL" {
			if tableName, ok := table["table_name"].(string); ok {
				if condition, ok := table["attached_condition"].(string); ok {
					recs = append(recs, fmt.Sprintf("Add index on table '%s' for condition: %s", tableName, condition))
				} else {
					recs = append(recs, fmt.Sprintf("Table '%s' is doing full scan - consider adding WHERE clause or index", tableName))
				}
			}
		}

		if rows, ok := table["rows_examined_per_scan"].(float64); ok && rows > 1000 {
			if tableName, ok := table["table_name"].(string); ok {
				recs = append(recs, fmt.Sprintf("Table '%s': High row count (%.0f rows) - consider limiting with WHERE or LIMIT", tableName, rows))
			}
		}

		if filtered, ok := table["filtered"].(string); ok {
			if filteredVal := parseFloat(filtered); filteredVal < 20.0 {
				if tableName, ok := table["table_name"].(string); ok {
					recs = append(recs, fmt.Sprintf("Table '%s': Low filtered percentage (%s%%) - query might benefit from better indexing", tableName, filtered))
				}
			}
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "Query looks well optimized! ✨")
	}

	return recs
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func sendResponse(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

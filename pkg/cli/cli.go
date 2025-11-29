package cli

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Start initializes the CLI with database connection and starts the interactive prompt
func Start(host string, port int, user, password, database, socket, loginPath, configFile, execute string, zstdCompressionLevel int, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel string) error {
	// Read MySQL config from files
	config, err := ReadMySQLConfig(loginPath, configFile)
	if err != nil {
		// Config reading failed, but continue with CLI args
		config = &MySQLConfig{}
	}

	// Merge config with CLI arguments (CLI takes precedence)
	mergedConfig := MergeConfig(config, user, password, host, port, socket, database)

	// Build DSN with compression if requested
	dsn := BuildDSN(mergedConfig.User, mergedConfig.Password, mergedConfig.Host, mergedConfig.Port, mergedConfig.Database, mergedConfig.Socket, zstdCompressionLevel)

	// Connect to database
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		// If compression was requested and connection failed, try without compression
		if zstdCompressionLevel > 0 {
			log.Printf("Warning: zstd compression (level %d) failed, retrying without compression...", zstdCompressionLevel)

			// Close the failed connection
			db.Close()

			// Build DSN without compression
			dsnNoCompress := BuildDSN(mergedConfig.User, mergedConfig.Password, mergedConfig.Host, mergedConfig.Port, mergedConfig.Database, mergedConfig.Socket, 0)

			// Try connecting without compression
			db, err = sql.Open("mysql", dsnNoCompress)
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}

			if err := db.Ping(); err != nil {
				return fmt.Errorf("failed to ping database (even without compression): %w", err)
			}

			log.Printf("Connected to MySQL (compression not supported by server)")
		} else {
			return fmt.Errorf("failed to ping database: %w", err)
		}
	} else {
		if zstdCompressionLevel > 0 {
			fmt.Println("Connected to MySQL (with zstd compression)")
		} else {
			fmt.Println("Connected to MySQL")
		}
	}

	// Print server version
	var version string
	if err := db.QueryRow("SELECT VERSION()").Scan(&version); err != nil {
		log.Printf("Failed to get version: %v", err)
	} else {
		fmt.Printf("MySQL version: %s\n", version)
	}

	// If execute flag is provided, execute the SQL and exit
	if execute != "" {
		return executeSQLAndExit(db, mergedConfig.User, mergedConfig.Host, mergedConfig.Port, mergedConfig.Database, execute, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel)
	}

	// Start the interactive prompt. Pass AI server settings for client overrides.
	return StartPrompt(db, mergedConfig.User, mergedConfig.Host, mergedConfig.Port, mergedConfig.Database, zstdCompressionLevel, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel)
}

// executeSQLAndExit executes a SQL command and exits
func executeSQLAndExit(db *sql.DB, user, host string, port int, database, sql string, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel string) error {
	cfg := LoadSyntaxConfig()
	if aiServerURL == "" {
		aiServerURL = cfg.AiServerURL
	}
	if aiServerMode == "" {
		aiServerMode = cfg.AiServerMode
	}
	if aiCachePath == "" {
		aiCachePath = cfg.AiCachePath
	}
	executor := &PromptExecutor{
		db:                   db,
		user:                 user,
		host:                 host,
		port:                 port,
		database:             database,
		buffer:               "",
		tables:               nil,
		columns:              make(map[string][]string),
		databases:            nil,
		cacheTime:            time.Time{},
		highlighter:          NewSyntaxHighlighter(),
		enableSuggestions:    cfg.EnableSuggestions,
		enableAIAnalysis:     cfg.EnableAIAnalysis,
		enableJSONExport:     cfg.EnableJSONExport,
		enableVisualExplain:  cfg.EnableVisualExplain,
		zstdCompressionLevel: 0, // Not used in non-interactive mode
		aiServerURL:          aiServerURL,
		aiServerMode:         aiServerMode,
		aiCachePath:          aiCachePath,
		aiDetailLevel:        aiDetailLevel,
	}

	// Execute the SQL command
	executor.ExecuteSQL(sql, false)
	return nil
}

// BuildDSN builds the MySQL DSN string
func BuildDSN(user, password, host string, port int, database, socket string, zstdCompressionLevel int) string {
	dsn := ""
	if socket != "" {
		dsn = fmt.Sprintf("%s:%s@unix(%s)/%s", user, password, socket, database)
	} else {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, host, port, database)
	}

	// Add zstd compression parameters if specified
	if zstdCompressionLevel > 0 {
		dsn += fmt.Sprintf("?compression-algorithms=zstd&zstd-level=%d", zstdCompressionLevel)
	}

	return dsn
}

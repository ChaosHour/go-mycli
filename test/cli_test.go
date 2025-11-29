package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"go-mycli/pkg/cli"
)

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		user                 string
		password             string
		host                 string
		port                 int
		database             string
		socket               string
		zstdCompressionLevel int
		expected             string
	}{
		{"user", "pass", "localhost", 3306, "db", "", 0, "user:pass@tcp(localhost:3306)/db"},
		{"user", "pass", "", 0, "db", "/tmp/mysql.sock", 0, "user:pass@unix(/tmp/mysql.sock)/db"},
		{"user", "pass", "localhost", 3306, "db", "", 3, "user:pass@tcp(localhost:3306)/db?compression-algorithms=zstd&zstd-level=3"},
		{"user", "pass", "", 0, "db", "/tmp/mysql.sock", 5, "user:pass@unix(/tmp/mysql.sock)/db?compression-algorithms=zstd&zstd-level=5"},
	}

	for _, tt := range tests {
		result := cli.BuildDSN(tt.user, tt.password, tt.host, tt.port, tt.database, tt.socket, tt.zstdCompressionLevel)
		if result != tt.expected {
			t.Errorf("BuildDSN(%q, %q, %q, %d, %q, %q, %d) = %q; want %q", tt.user, tt.password, tt.host, tt.port, tt.database, tt.socket, tt.zstdCompressionLevel, result, tt.expected)
		}
	}
}

func TestReadMySQLConfig(t *testing.T) {
	// Create a temporary directory to avoid conflicts with real config files
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, ".my.cnf")

	configContent := `[client]
user = testuser
password = testpass
host = testhost
port = 3307
database = testdb

[mysqld]
default_socket = /tmp/mysql.sock
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Temporarily change HOME to avoid reading real ~/.my.cnf
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Test the ReadMySQLConfig function with a specific config file
	testConfig, err := cli.ReadMySQLConfig("", configFile)
	if err != nil {
		t.Fatal(err)
	}

	if testConfig.User != "testuser" {
		t.Errorf("Expected user 'testuser', got '%s'", testConfig.User)
	}
	if testConfig.Password != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", testConfig.Password)
	}
	if testConfig.Host != "testhost" {
		t.Errorf("Expected host 'testhost', got '%s'", testConfig.Host)
	}
	if testConfig.Port != 3307 {
		t.Errorf("Expected port 3307, got %d", testConfig.Port)
	}
	if testConfig.Database != "testdb" {
		t.Errorf("Expected database 'testdb', got '%s'", testConfig.Database)
	}
}

func TestMergeConfig(t *testing.T) {
	config := &cli.MySQLConfig{
		User:     "configuser",
		Password: "configpass",
		Host:     "confighost",
		Port:     3306,
		Database: "configdb",
	}

	// Test CLI overrides
	merged := cli.MergeConfig(config, "cliuser", "clipass", "clihost", 3307, "", "clidb")

	if merged.User != "cliuser" {
		t.Errorf("Expected user 'cliuser', got '%s'", merged.User)
	}
	if merged.Password != "clipass" {
		t.Errorf("Expected password 'clipass', got '%s'", merged.Password)
	}
	if merged.Host != "clihost" {
		t.Errorf("Expected host 'clihost', got '%s'", merged.Host)
	}
	if merged.Port != 3307 {
		t.Errorf("Expected port 3307, got %d", merged.Port)
	}
	if merged.Database != "clidb" {
		t.Errorf("Expected database 'clidb', got '%s'", merged.Database)
	}

	// Test empty CLI values don't override config
	merged2 := cli.MergeConfig(config, "", "", "", 0, "", "")

	if merged2.User != "configuser" {
		t.Errorf("Expected user 'configuser', got '%s'", merged2.User)
	}
	if merged2.Host != "confighost" {
		t.Errorf("Expected host 'confighost', got '%s'", merged2.Host)
	}
}

func TestStripMatchingQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`'quoted'`, "quoted"},
		{`"quoted"`, "quoted"},
		{"unquoted", "unquoted"},
		{`'mismatch"`, `'mismatch"`},
		{"  spaced  ", "spaced"},
	}

	for _, tt := range tests {
		result := cli.StripMatchingQuotes(tt.input)
		if result != tt.expected {
			t.Errorf("StripMatchingQuotes(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFindMatches(t *testing.T) {
	// Note: This test would require exporting internal types and methods
	// For now, we'll skip testing the internal completer logic
	// The functionality is tested indirectly through integration tests
	t.Skip("Internal completer logic testing requires exported types")
}

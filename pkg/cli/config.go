package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

// MySQLConfig represents the parsed MySQL configuration
type MySQLConfig struct {
	User     string
	Password string
	Host     string
	Port     int
	Socket   string
	Database string
}

// ReadMySQLConfig reads MySQL configuration from standard config files
func ReadMySQLConfig(loginPath, configFilePath string) (*MySQLConfig, error) {
	config := &MySQLConfig{
		Port: 0, // 0 means not set, will default to 3306 later
	}

	// Config files to read in order (later ones override earlier ones)
	configFiles := []string{
		"/etc/my.cnf",
		"/etc/mysql/my.cnf",
		"/usr/local/etc/my.cnf",
		filepath.Join(os.Getenv("HOME"), ".my.cnf"),
	}

	// Add custom config file if specified (highest priority)
	if configFilePath != "" {
		configFiles = append(configFiles, configFilePath)
	}

	// Read each config file
	for _, configFile := range configFiles {
		if err := readConfigFile(configFile, config, loginPath); err != nil {
			// Skip files that don't exist or can't be read
			continue
		}
	}

	return config, nil
}

// readConfigFile reads a single MySQL config file
func readConfigFile(filename string, config *MySQLConfig, loginPath string) error {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return err
	}

	// Load the INI file
	cfg, err := ini.Load(filename)
	if err != nil {
		return err
	}

	// Sections to check in order of priority
	sections := []string{"client", "mysqld"}
	if loginPath != "" && loginPath != "client" {
		sections = append(sections, loginPath)
	}

	// Read values from sections
	for _, sectionName := range sections {
		section, err := cfg.GetSection(sectionName)
		if err != nil {
			continue // Section doesn't exist
		}

		// Read user
		if user := section.Key("user").String(); user != "" && config.User == "" {
			config.User = user
		}

		// Read password
		if password := section.Key("password").String(); password != "" && config.Password == "" {
			config.Password = password
		}

		// Read host
		if host := section.Key("host").String(); host != "" && config.Host == "" {
			config.Host = host
		}

		// Read port
		if portStr := section.Key("port").String(); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil && config.Port == 0 {
				config.Port = port
			}
		}

		// Read socket
		if socket := section.Key("socket").String(); socket != "" && config.Socket == "" && config.Host == "" {
			config.Socket = socket
		}

		// Read database
		if database := section.Key("database").String(); database != "" && config.Database == "" {
			config.Database = database
		}

		// Handle mysqld transformations
		if sectionName == "mysqld" {
			// Transform mysqld keys to client keys
			if socket := section.Key("default_socket").String(); socket != "" && config.Socket == "" && config.Host == "" {
				config.Socket = socket
			}
			if portStr := section.Key("default_port").String(); portStr != "" {
				if port, err := strconv.Atoi(portStr); err == nil && config.Port == 0 {
					config.Port = port
				}
			}
			if user := section.Key("default_user").String(); user != "" && config.User == "" {
				config.User = user
			}
		}
	}

	return nil
}

// StripMatchingQuotes removes surrounding quotes from a string
func StripMatchingQuotes(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) ||
		(strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) {
		return s[1 : len(s)-1]
	}
	return s
}

// MergeConfig merges config values with command line arguments
// Command line arguments take precedence over config file values
func MergeConfig(config *MySQLConfig, cliUser, cliPassword, cliHost string, cliPort int, cliSocket, cliDatabase string) *MySQLConfig {
	merged := &MySQLConfig{
		User:     config.User,
		Password: config.Password,
		Host:     config.Host,
		Port:     config.Port,
		Socket:   config.Socket,
		Database: config.Database,
	}

	// Override with CLI values if provided
	if cliUser != "" {
		merged.User = cliUser
	}
	if cliPassword != "" {
		merged.Password = cliPassword
	}
	if cliHost != "" {
		merged.Host = cliHost
	}
	if cliPort != 0 {
		merged.Port = cliPort
	}
	if cliSocket != "" {
		merged.Socket = cliSocket
	}
	if cliDatabase != "" {
		merged.Database = cliDatabase
	}

	// Set defaults
	if merged.User == "" {
		merged.User = os.Getenv("USER")
	}
	if merged.Host == "" && merged.Socket == "" {
		merged.Host = "localhost"
	}
	if merged.Port == 0 {
		merged.Port = 3306
	}

	// If both host and socket are configured, prefer TCP connection
	// Only clear socket if host was explicitly set (not defaulted to localhost)
	if cliHost != "" || config.Host != "" {
		if merged.Socket != "" {
			merged.Socket = ""
		}
	}

	return merged
}

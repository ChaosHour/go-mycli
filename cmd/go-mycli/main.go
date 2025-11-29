package main

import (
	"fmt"
	"log"
	"os"

	"go-mycli/pkg/cli"

	"github.com/spf13/cobra"
)

var (
	host                 string
	port                 int
	user                 string
	password             string
	database             string
	socket               string
	loginPath            string
	configFile           string
	execute              string
	zstdCompressionLevel int
	aiServerURL          string
	aiServerMode         string
	aiCachePath          string
	aiDetailLevel        string
)

var rootCmd = &cobra.Command{
	Use:   "go-mycli",
	Short: "A MySQL CLI client in Go",
	Long:  `A command line client for MySQL with interactive prompt support.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If database is provided as arg
		if len(args) > 0 {
			database = args[0]
		}

		// Start the CLI
		if err := cli.Start(host, port, user, password, database, socket, loginPath, configFile, execute, zstdCompressionLevel, aiServerURL, aiServerMode, aiCachePath, aiDetailLevel); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.Flags().StringVarP(&host, "host", "", "", "Host address of the database")
	rootCmd.Flags().IntVarP(&port, "port", "P", 3306, "Port number to use for connection")
	rootCmd.Flags().StringVarP(&user, "user", "u", "", "User name to connect to the database")
	rootCmd.Flags().StringVarP(&password, "password", "p", "", "Password to connect to the database")
	rootCmd.Flags().StringVarP(&database, "database", "D", "", "Database to use")
	rootCmd.Flags().StringVarP(&socket, "socket", "S", "", "The socket file to use for connection")
	rootCmd.Flags().StringVarP(&loginPath, "login-path", "g", "", "Read this path from the login file")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to MySQL config file")
	rootCmd.Flags().StringVarP(&execute, "execute", "e", "", "Execute command and quit")
	rootCmd.Flags().IntVar(&zstdCompressionLevel, "zstd-compression-level", 0, "The compression level to use for zstd compression (1-22, 0 to disable). Falls back to uncompressed if server doesn't support zstd")
	rootCmd.Flags().StringVar(&aiServerURL, "ai-server-url", "", "URL of AI server (MCP http endpoint); overrides config")
	rootCmd.Flags().StringVar(&aiServerMode, "ai-server-mode", "", "AI server mode: copilot_mcp_http|openai|mcp_stdio")
	rootCmd.Flags().StringVar(&aiCachePath, "ai-cache-path", "", "Path to local AI cache database")
	rootCmd.Flags().StringVar(&aiDetailLevel, "ai-detail-level", "basic", "AI analysis detail level: basic|detailed|expert")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"gopkg.in/ini.v1"
)

// SyntaxConfig holds the syntax highlighting configuration
type SyntaxConfig struct {
	Style               string
	UseCustomColors     bool
	EnableSuggestions   bool
	EnableAIAnalysis    bool
	EnableJSONExport    bool
	EnableVisualExplain bool
	AiServerURL         string
	AiServerMode        string
	AiCachePath         string
	Colors              map[string]string
}

// DefaultSyntaxConfig returns the default syntax configuration
func DefaultSyntaxConfig() *SyntaxConfig {
	return &SyntaxConfig{
		Style:               "monokai",
		UseCustomColors:     false,
		EnableSuggestions:   true,
		EnableAIAnalysis:    true,
		EnableJSONExport:    false,
		EnableVisualExplain: false,
		AiServerURL:         "http://127.0.0.1:8800/mcp",
		AiServerMode:        "copilot_mcp_http",
		AiCachePath:         "~/.go-mycli/ai_cache.db",
		Colors:              DefaultColors(),
	}
}

// DefaultColors returns the default color scheme
func DefaultColors() map[string]string {
	return map[string]string{
		"keyword":     "#66D9EF", // Bright cyan for SQL keywords (SELECT, FROM, WHERE)
		"name":        "#A6E22E", // Bright green for table/column names
		"builtin":     "#FD971F", // Orange for SQL functions (COUNT, SUM, etc.)
		"string":      "#E6DB74", // Yellow for string literals
		"number":      "#AE81FF", // Purple for numbers
		"operator":    "#F92672", // Pink for operators (=, >, <, etc.)
		"comment":     "#75715E", // Gray for comments
		"punctuation": "#F8F8F2", // White for punctuation (, ; etc.)
	}
}

// LoadSyntaxConfig loads syntax configuration from ~/.go-myclirc
func LoadSyntaxConfig() *SyntaxConfig {
	config := DefaultSyntaxConfig()

	// Check for local config in current directory first (./.go-myclirc), then fallback to $HOME/.go-myclirc
	// Also allow env var override GO_MYCLI_RC
	var configPath string
	if envPath := os.Getenv("GO_MYCLI_RC"); envPath != "" {
		configPath = envPath
	} else {
		if wd, err := os.Getwd(); err == nil {
			localPath := filepath.Join(wd, ".go-myclirc")
			if _, err := os.Stat(localPath); err == nil {
				configPath = localPath
			}
		}
		if configPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return config
			}
			configPath = filepath.Join(home, ".go-myclirc")
		}
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config file doesn't exist, return default
		return config
	}

	// Load INI file allowing values that start with '#'
	cfg, err := ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, configPath)
	if err != nil {
		// Failed to load, return default
		return config
	}

	customColorsKeySet := false
	// Load main section
	if cfg.HasSection("main") {
		main := cfg.Section("main")
		if main.HasKey("syntax_style") {
			config.Style = main.Key("syntax_style").String()
		}
		if main.HasKey("use_custom_colors") {
			if val, err := main.Key("use_custom_colors").Bool(); err == nil {
				config.UseCustomColors = val
				customColorsKeySet = true
			}
		}
		if main.HasKey("suggestions") {
			if val, err := main.Key("suggestions").Bool(); err == nil {
				config.EnableSuggestions = val
			}
		}
		if main.HasKey("ai_analysis") {
			if val, err := main.Key("ai_analysis").Bool(); err == nil {
				config.EnableAIAnalysis = val
			}
		}
		if main.HasKey("json_export") {
			if val, err := main.Key("json_export").Bool(); err == nil {
				config.EnableJSONExport = val
			}
		}
		if main.HasKey("visual_explain") {
			if val, err := main.Key("visual_explain").Bool(); err == nil {
				config.EnableVisualExplain = val
			}
		}
		if main.HasKey("ai_server_url") {
			config.AiServerURL = main.Key("ai_server_url").String()
		}
		if main.HasKey("ai_server_mode") {
			config.AiServerMode = main.Key("ai_server_mode").String()
		}
		if main.HasKey("ai_cache_path") {
			config.AiCachePath = main.Key("ai_cache_path").String()
		}
	}

	// Load colors section
	if cfg.HasSection("colors") {
		colors := cfg.Section("colors")
		for _, key := range colors.Keys() {
			val := sanitizeColorValue(key.String())
			if val == "" {
				continue
			}
			// Convert hex colors to uppercase for Chroma compatibility
			if strings.HasPrefix(val, "#") && len(val) == 7 {
				val = strings.ToUpper(val)
			}
			config.Colors[key.Name()] = val
		}
	} else if !customColorsKeySet {
		// No colors configured explicitly, respect style selection
		config.UseCustomColors = false
		config.Colors = map[string]string{}
	}

	return config
}

// SaveDefaultSyntaxConfig creates a default config file at ~/.go-myclirc
func SaveDefaultSyntaxConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(home, ".go-myclirc")

	// Don't overwrite existing config
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}

	// Create default config
	cfg := ini.Empty()

	// Main section
	main, _ := cfg.NewSection("main")
	main.NewKey("syntax_style", "monokai")
	main.NewKey("use_custom_colors", "true")
	main.NewKey("suggestions", "true")
	main.NewKey("ai_analysis", "false")
	main.NewKey("json_export", "false")
	main.NewKey("visual_explain", "false")
	main.NewKey("ai_server_url", "http://127.0.0.1:44044/mcp")
	main.NewKey("ai_server_mode", "copilot_mcp_http")
	main.NewKey("ai_cache_path", "~/.go-mycli/ai_cache.db")
	main.Comment = "# Syntax highlighting style: monokai, dracula, native, vim, etc.\n# Set use_custom_colors=false to use the style without overrides.\n# Enable AI-powered EXPLAIN analysis (requires OPENAI_API_KEY)\n# Enable JSON export for external tools like pt-visual-explain\n# Enable built-in visual explain tree representation\n# See: https://github.com/alecthomas/chroma#styles"

	// Colors section
	colors, _ := cfg.NewSection("colors")
	colors.NewKey("keyword", "#66D9EF")
	colors.NewKey("name", "#A6E22E")
	colors.NewKey("builtin", "#FD971F")
	colors.NewKey("string", "#E6DB74")
	colors.NewKey("number", "#AE81FF")
	colors.NewKey("operator", "#F92672")
	colors.NewKey("comment", "#75715E")
	colors.NewKey("punctuation", "#F8F8F2")
	colors.Comment = "# Custom colors for SQL tokens (hex format: #RRGGBB)\n# Keywords: SQL commands (SELECT, FROM, WHERE, etc.)\n# Name: Table and column names\n# Builtin: SQL functions (COUNT, SUM, MAX, etc.)\n# String: String literals ('text')\n# Number: Numeric values\n# Operator: Comparison and logical operators (=, >, AND, etc.)\n# Comment: SQL comments\n# Punctuation: Commas, semicolons, parentheses"

	// Save to file
	return cfg.SaveTo(configPath)
}

// CreateStyleFromConfig creates a Chroma style from configuration
func CreateStyleFromConfig(config *SyntaxConfig) *chroma.Style {
	if config == nil {
		config = DefaultSyntaxConfig()
	}

	base := resolveBaseStyle(config.Style)
	if base == nil {
		base = chroma.MustNewStyle("native", chroma.StyleEntries{})
	}

	if !config.UseCustomColors || len(config.Colors) == 0 {
		return base
	}

	entries := chroma.StyleEntries{}
	for tokenName, color := range config.Colors {
		switch strings.ToLower(tokenName) {
		case "keyword":
			entries[chroma.Keyword] = color
			entries[chroma.KeywordConstant] = color
			entries[chroma.KeywordDeclaration] = color
			entries[chroma.KeywordNamespace] = color
			entries[chroma.KeywordPseudo] = color
			entries[chroma.KeywordReserved] = color
			entries[chroma.KeywordType] = color
		case "name":
			entries[chroma.Name] = color
			entries[chroma.NameAttribute] = color
			entries[chroma.NameClass] = color
			entries[chroma.NameConstant] = color
			entries[chroma.NameDecorator] = color
			entries[chroma.NameEntity] = color
			entries[chroma.NameException] = color
			entries[chroma.NameLabel] = color
			entries[chroma.NameNamespace] = color
			entries[chroma.NameOther] = color
			entries[chroma.NameTag] = color
			entries[chroma.NameVariable] = color
		case "builtin":
			entries[chroma.NameBuiltin] = color
			entries[chroma.NameBuiltinPseudo] = color
			entries[chroma.NameFunction] = color
		case "string":
			entries[chroma.LiteralString] = color
			entries[chroma.LiteralStringBacktick] = color
			entries[chroma.LiteralStringChar] = color
			entries[chroma.LiteralStringDoc] = color
			entries[chroma.LiteralStringDouble] = color
			entries[chroma.LiteralStringEscape] = color
			entries[chroma.LiteralStringHeredoc] = color
			entries[chroma.LiteralStringInterpol] = color
			entries[chroma.LiteralStringOther] = color
			entries[chroma.LiteralStringRegex] = color
			entries[chroma.LiteralStringSingle] = color
			entries[chroma.LiteralStringSymbol] = color
		case "number":
			entries[chroma.LiteralNumber] = color
			entries[chroma.LiteralNumberBin] = color
			entries[chroma.LiteralNumberFloat] = color
			entries[chroma.LiteralNumberHex] = color
			entries[chroma.LiteralNumberInteger] = color
			entries[chroma.LiteralNumberIntegerLong] = color
			entries[chroma.LiteralNumberOct] = color
		case "operator":
			entries[chroma.Operator] = color
			entries[chroma.OperatorWord] = color
		case "comment":
			entries[chroma.Comment] = color
			entries[chroma.CommentHashbang] = color
			entries[chroma.CommentMultiline] = color
			entries[chroma.CommentPreproc] = color
			entries[chroma.CommentSingle] = color
			entries[chroma.CommentSpecial] = color
		case "punctuation":
			entries[chroma.Punctuation] = color
		}
	}

	entries[chroma.Text] = ""
	entries[chroma.TextWhitespace] = ""

	style, err := chroma.NewStyle("go-mycli-custom", entries)
	if err != nil {
		return base
	}

	return style
}

// SaveSyntaxConfig writes the provided syntax config back to ~/.go-myclirc
func SaveSyntaxConfig(config *SyntaxConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, ".go-myclirc")

	cfg := ini.Empty()
	main, _ := cfg.NewSection("main")
	main.NewKey("syntax_style", config.Style)
	main.NewKey("use_custom_colors", fmt.Sprintf("%v", config.UseCustomColors))
	main.NewKey("suggestions", fmt.Sprintf("%v", config.EnableSuggestions))
	main.NewKey("ai_analysis", fmt.Sprintf("%v", config.EnableAIAnalysis))
	main.NewKey("json_export", fmt.Sprintf("%v", config.EnableJSONExport))
	main.NewKey("visual_explain", fmt.Sprintf("%v", config.EnableVisualExplain))
	main.NewKey("ai_server_url", config.AiServerURL)
	main.NewKey("ai_server_mode", config.AiServerMode)
	main.NewKey("ai_cache_path", config.AiCachePath)

	colorsSection, _ := cfg.NewSection("colors")
	for k, v := range config.Colors {
		colorsSection.NewKey(k, v)
	}

	return cfg.SaveTo(configPath)
}

func resolveBaseStyle(name string) *chroma.Style {
	if name != "" {
		if strings.EqualFold(name, "smooth") {
			if smooth := smoothStyle(); smooth != nil {
				return smooth
			}
		}
		if style := styles.Get(name); style != nil {
			return style
		}
	}
	if style := styles.Get("monokai"); style != nil {
		return style
	}
	return nil
}

func smoothStyle() *chroma.Style {
	entries := chroma.StyleEntries{
		chroma.Background:          "bg:#0b1220",
		chroma.Text:                "#c4d0f4",
		chroma.TextWhitespace:      "#0b1220",
		chroma.Keyword:             "#9ac4ff bold",
		chroma.KeywordType:         "#9ac4ff bold",
		chroma.Name:                "#a8f5ff",
		chroma.NameFunction:        "#7de2d1",
		chroma.NameBuiltin:         "#7de2d1",
		chroma.LiteralString:       "#f9e2af",
		chroma.LiteralStringChar:   "#f9e2af",
		chroma.LiteralStringDouble: "#f9e2af",
		chroma.LiteralNumber:       "#f4b5ff",
		chroma.Operator:            "#f7768e",
		chroma.OperatorWord:        "#f7768e",
		chroma.Comment:             "italic #51617d",
		chroma.CommentHashbang:     "italic #51617d",
		chroma.CommentSingle:       "italic #51617d",
		chroma.Punctuation:         "#c4d0f4",
	}

	style, err := chroma.NewStyle("smooth-night", entries)
	if err != nil {
		return styles.Get("monokai")
	}
	return style
}

func sanitizeColorValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case ';':
			raw = strings.TrimSpace(raw[:i])
			return raw
		case '#':
			if i == 0 {
				continue
			}
			// treat as start of inline comment
			raw = strings.TrimSpace(raw[:i])
			return raw
		}
		if raw[i] == ' ' || raw[i] == '	' {
			// collapse multiple spaces but keep scanning to detect inline comments (# or ;) after whitespace
			continue
		}
	}

	return raw
}

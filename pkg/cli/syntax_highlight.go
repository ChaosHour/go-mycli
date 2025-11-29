package cli

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/c-bata/go-prompt"
)

// SyntaxHighlighter provides syntax highlighting for SQL input
type SyntaxHighlighter struct {
	lexer     chroma.Lexer
	formatter chroma.Formatter
	style     *chroma.Style
}

// NewSyntaxHighlighter creates a new syntax highlighter for MySQL
func NewSyntaxHighlighter() *SyntaxHighlighter {
	// Load user configuration
	config := LoadSyntaxConfig()

	// Use MySQL lexer from Chroma
	lexer := lexers.Get("mysql")
	if lexer == nil {
		// Fallback to SQL lexer if MySQL is not available
		lexer = lexers.Get("sql")
	}

	// Create style from configuration
	style := CreateStyleFromConfig(config)

	// Use terminal16m formatter for better ANSI color support
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		formatter = formatters.Get("terminal256")
	}
	if formatter == nil {
		formatter = formatters.Get("terminal")
	}
	if formatter == nil {
		formatter = formatters.Fallback
	}

	return &SyntaxHighlighter{
		lexer:     lexer,
		formatter: formatter,
		style:     style,
	}
}

// HighlightingWriter wraps a ConsoleWriter to apply syntax highlighting
type HighlightingWriter struct {
	inner       prompt.ConsoleWriter
	highlighter *SyntaxHighlighter
}

// NewHighlightingWriter creates a new highlighting writer
func NewHighlightingWriter(highlighter *SyntaxHighlighter) *HighlightingWriter {
	return &HighlightingWriter{
		inner:       prompt.NewStdoutWriter(),
		highlighter: highlighter,
	}
}

// WriteRaw writes raw bytes
func (w *HighlightingWriter) WriteRaw(data []byte) {
	w.inner.WriteRaw(data)
}

// Write writes bytes with control sequence removal
func (w *HighlightingWriter) Write(data []byte) {
	w.inner.Write(data)
}

// WriteRawStr writes raw string
func (w *HighlightingWriter) WriteRawStr(data string) {
	w.inner.WriteRawStr(data)
}

// WriteStr writes string with control sequence removal
func (w *HighlightingWriter) WriteStr(data string) {
	w.inner.WriteStr(data)
}

// Flush flushes the buffer
func (w *HighlightingWriter) Flush() error {
	return w.inner.Flush()
}

// EraseScreen erases the screen
func (w *HighlightingWriter) EraseScreen() {
	w.inner.EraseScreen()
}

// EraseUp erases up
func (w *HighlightingWriter) EraseUp() {
	w.inner.EraseUp()
}

// EraseDown erases down
func (w *HighlightingWriter) EraseDown() {
	w.inner.EraseDown()
}

// EraseStartOfLine erases start of line
func (w *HighlightingWriter) EraseStartOfLine() {
	w.inner.EraseStartOfLine()
}

// EraseEndOfLine erases end of line
func (w *HighlightingWriter) EraseEndOfLine() {
	w.inner.EraseEndOfLine()
}

// EraseLine erases the line
func (w *HighlightingWriter) EraseLine() {
	w.inner.EraseLine()
}

// ShowCursor shows cursor
func (w *HighlightingWriter) ShowCursor() {
	w.inner.ShowCursor()
}

// HideCursor hides cursor
func (w *HighlightingWriter) HideCursor() {
	w.inner.HideCursor()
}

// CursorGoTo moves cursor
func (w *HighlightingWriter) CursorGoTo(row, col int) {
	w.inner.CursorGoTo(row, col)
}

// CursorUp moves cursor up
func (w *HighlightingWriter) CursorUp(n int) {
	w.inner.CursorUp(n)
}

// CursorDown moves cursor down
func (w *HighlightingWriter) CursorDown(n int) {
	w.inner.CursorDown(n)
}

// CursorForward moves cursor forward
func (w *HighlightingWriter) CursorForward(n int) {
	w.inner.CursorForward(n)
}

// CursorBackward moves cursor backward
func (w *HighlightingWriter) CursorBackward(n int) {
	w.inner.CursorBackward(n)
}

// AskForCPR asks for cursor position
func (w *HighlightingWriter) AskForCPR() {
	w.inner.AskForCPR()
}

// SaveCursor saves cursor
func (w *HighlightingWriter) SaveCursor() {
	w.inner.SaveCursor()
}

// UnSaveCursor restores cursor
func (w *HighlightingWriter) UnSaveCursor() {
	w.inner.UnSaveCursor()
}

// ScrollDown scrolls down
func (w *HighlightingWriter) ScrollDown() {
	w.inner.ScrollDown()
}

// ScrollUp scrolls up
func (w *HighlightingWriter) ScrollUp() {
	w.inner.ScrollUp()
}

// SetTitle sets title
func (w *HighlightingWriter) SetTitle(title string) {
	w.inner.SetTitle(title)
}

// ClearTitle clears title
func (w *HighlightingWriter) ClearTitle() {
	w.inner.ClearTitle()
}

// SetColor sets color
func (w *HighlightingWriter) SetColor(fg, bg prompt.Color, bold bool) {
	w.inner.SetColor(fg, bg, bold)
}

// HighlightSQL applies syntax highlighting to SQL text
func (sh *SyntaxHighlighter) HighlightSQL(sql string) string {
	if sh.lexer == nil {
		return sql // No highlighting if lexer not available
	}

	// Tokenize the SQL
	iterator, err := sh.lexer.Tokenise(nil, sql)
	if err != nil {
		return sql // Return original text on error
	}

	// Format with ANSI colors
	var buf strings.Builder
	err = sh.formatter.Format(&buf, sh.style, iterator)
	if err != nil {
		return sql // Return original text on error
	}

	return buf.String()
}

// HighlightPromptDocument highlights a go-prompt document
func (sh *SyntaxHighlighter) HighlightPromptDocument(doc *prompt.Document) string {
	text := doc.Text

	// Only highlight if we have a lexer
	if sh.lexer == nil {
		return text
	}

	// Get the current line being edited
	currentLine := doc.CurrentLine()

	// Highlight the current line
	highlighted := sh.HighlightSQL(currentLine)

	// For multi-line input, we need to handle the entire buffer
	// But for now, let's focus on the current line for performance
	return highlighted
}

// GetLexerNames returns available lexer names for debugging
func (sh *SyntaxHighlighter) GetLexerNames() []string {
	return lexers.Names(false)
}

// GetStyleNames returns available style names for debugging
func (sh *SyntaxHighlighter) GetStyleNames() []string {
	return styles.Names()
}

// TestHighlighting tests the highlighting functionality
func (sh *SyntaxHighlighter) TestHighlighting() {
	testSQL := "SELECT id, name FROM users WHERE age > 18 ORDER BY name;"

	fmt.Println("Original SQL:")
	fmt.Println(testSQL)
	fmt.Println("\nHighlighted SQL:")
	highlighted := sh.HighlightSQL(testSQL)
	fmt.Println(highlighted)

	// Debug: check if highlighting actually added ANSI codes
	if highlighted == testSQL {
		fmt.Println("WARNING: No highlighting applied - output identical to input")

		// Try to debug tokenization
		if sh.lexer != nil {
			fmt.Println("Lexer available, trying to tokenize...")
			iterator, err := sh.lexer.Tokenise(nil, testSQL)
			if err != nil {
				fmt.Println("Tokenize error:", err)
			} else {
				fmt.Println("Tokens:")
				for {
					token := iterator()
					if token == (chroma.Token{}) {
						break
					}
					fmt.Printf("  %s: %q\n", token.Type, token.Value)
				}
			}
		} else {
			fmt.Println("ERROR: Lexer is nil")
		}

		if sh.formatter == nil {
			fmt.Println("ERROR: Formatter is nil")
		}
	}

	fmt.Println("\nAvailable lexers:")
	for _, name := range sh.GetLexerNames() {
		if strings.Contains(strings.ToLower(name), "sql") || strings.Contains(strings.ToLower(name), "mysql") {
			fmt.Printf("  - %s\n", name)
		}
	}

	fmt.Println("\nAvailable styles:")
	for _, name := range sh.GetStyleNames() {
		fmt.Printf("  - %s\n", name)
	}
}

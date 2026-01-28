package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	primaryColor   = lipgloss.Color("#7C3AED") // Purple
	successColor   = lipgloss.Color("#10B981") // Green
	warningColor   = lipgloss.Color("#F59E0B") // Amber
	errorColor     = lipgloss.Color("#EF4444") // Red
	mutedColor     = lipgloss.Color("#6B7280") // Gray
	borderColor    = lipgloss.Color("#374151") // Dark gray
)

// Styles
var (
	// Text styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	successStyle = lipgloss.NewStyle().
			Foreground(successColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	boldStyle = lipgloss.NewStyle().
			Bold(true)

	// Table styles
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(primaryColor).
				Padding(0, 1)

	tableCellStyle = lipgloss.NewStyle().
			Padding(0, 1)

	tableRowStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(borderColor)

	// Status badges
	passingBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(successColor).
			Padding(0, 1).
			SetString("PASSING")

	failingBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(errorColor).
			Padding(0, 1).
			SetString("FAILING")

	pausedBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(warningColor).
			Padding(0, 1).
			SetString("PAUSED")

	// Box styles
	infoBox = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(0, 1)
)

// Icons
const (
	iconCheck    = "✓"
	iconCross    = "✗"
	iconWarning  = "⚠"
	iconInfo     = "ℹ"
	iconArrow    = "→"
	iconDot      = "•"
	iconSpinner  = "◐"
)

// UITable represents a styled table
type UITable struct {
	Headers []string
	Rows    [][]string
	MaxWidth int
}

// Render renders the table with beautiful styling
func (t *UITable) Render() string {
	if len(t.Rows) == 0 {
		return mutedStyle.Render("No results found")
	}

	// Calculate column widths
	colWidths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		colWidths[i] = len(h)
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// Cap column widths
	maxColWidth := 40
	for i := range colWidths {
		if colWidths[i] > maxColWidth {
			colWidths[i] = maxColWidth
		}
	}

	var sb strings.Builder

	// Render header
	var headerCells []string
	for i, h := range t.Headers {
		cell := tableHeaderStyle.Width(colWidths[i]).Render(h)
		headerCells = append(headerCells, cell)
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
	sb.WriteString("\n")

	// Render rows
	for _, row := range t.Rows {
		var cells []string
		for i, cell := range row {
			if i < len(colWidths) {
				// Truncate if needed
				if len(cell) > colWidths[i] {
					cell = cell[:colWidths[i]-1] + "…"
				}
				styledCell := tableCellStyle.Width(colWidths[i]).Render(cell)
				cells = append(cells, styledCell)
			}
		}
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		sb.WriteString("\n")
	}

	return sb.String()
}

// StatusBadge returns a styled status badge
func StatusBadge(passing bool, paused bool) string {
	if paused {
		return pausedBadge.String()
	}
	if passing {
		return passingBadge.String()
	}
	return failingBadge.String()
}

// Success prints a success message
func Success(msg string) {
	fmt.Println(successStyle.Render(iconCheck + " " + msg))
}

// Error prints an error message
func Error(msg string) {
	fmt.Println(errorStyle.Render(iconCross + " " + msg))
}

// Warning prints a warning message
func Warning(msg string) {
	fmt.Println(warningStyle.Render(iconWarning + " " + msg))
}

// Info prints an info message
func Info(msg string) {
	fmt.Println(mutedStyle.Render(iconInfo + " " + msg))
}

// Title prints a title
func Title(msg string) {
	fmt.Println(titleStyle.Render(msg))
}

// Muted prints muted text
func Muted(msg string) {
	fmt.Println(mutedStyle.Render(msg))
}

// FormatJSON formats JSON with syntax highlighting
func FormatJSON(data []byte) string {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "  "); err != nil {
		return string(data)
	}
	return prettyJSON.String()
}

// RenderKeyValue renders a key-value pair
func RenderKeyValue(key, value string) string {
	return fmt.Sprintf("%s %s",
		mutedStyle.Render(key+":"),
		value)
}

// RenderList renders a list of items
func RenderList(title string, items []string) string {
	var sb strings.Builder
	sb.WriteString(boldStyle.Render(title))
	sb.WriteString("\n")
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("  %s %s\n", mutedStyle.Render(iconDot), item))
	}
	return sb.String()
}

// Box wraps content in a styled box
func Box(content string) string {
	return infoBox.Render(content)
}

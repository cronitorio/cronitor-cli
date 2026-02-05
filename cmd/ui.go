package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cronitorio/cronitor-cli/lib"
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

	// Calculate column widths using visual width (handles ANSI codes)
	colWidths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		colWidths[i] = lipgloss.Width(h)
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			cellWidth := lipgloss.Width(cell)
			if i < len(colWidths) && cellWidth > colWidths[i] {
				colWidths[i] = cellWidth
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

	// Render header (add 2 for padding on each side)
	var headerCells []string
	for i, h := range t.Headers {
		cell := tableHeaderStyle.Width(colWidths[i] + 2).Render(h)
		headerCells = append(headerCells, cell)
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
	sb.WriteString("\n")

	// Render rows
	for _, row := range t.Rows {
		var cells []string
		for i, cell := range row {
			if i < len(colWidths) {
				// Truncate if needed (only for plain text cells)
				cellWidth := lipgloss.Width(cell)
				if cellWidth > colWidths[i] {
					// Simple truncation for cells without ANSI codes
					if cellWidth == len(cell) {
						cell = cell[:colWidths[i]-1] + "…"
					}
					// For styled cells, just let them overflow slightly
				}
				styledCell := tableCellStyle.Width(colWidths[i] + 2).Render(cell)
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

// FetchAllPages fetches all pages from a paginated API endpoint.
// It returns all response bodies as a slice, stopping when a page returns
// an empty items array (identified by itemsKey in the JSON response).
func FetchAllPages(client *lib.APIClient, endpoint string, params map[string]string, itemsKey string) ([][]byte, error) {
	var bodies [][]byte
	page := 1
	for {
		p := make(map[string]string)
		for k, v := range params {
			p[k] = v
		}
		p["page"] = fmt.Sprintf("%d", page)

		resp, err := client.GET(endpoint, p)
		if err != nil {
			return bodies, err
		}
		if !resp.IsSuccess() {
			return nil, fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
		}
		bodies = append(bodies, resp.Body)

		// Check if there are items in this page
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(resp.Body, &raw); err != nil {
			break
		}
		if items, ok := raw[itemsKey]; ok {
			var arr []json.RawMessage
			if err := json.Unmarshal(items, &arr); err != nil || len(arr) == 0 {
				break
			}
		} else {
			break
		}

		page++
		if page > 200 { // safety limit
			break
		}
	}
	return bodies, nil
}

// MergePagedJSON merges multiple paginated API responses into a single JSON array.
// It extracts items from each page using the specified key and combines them.
func MergePagedJSON(responses [][]byte, key string) []byte {
	var allItems []json.RawMessage
	for _, body := range responses {
		var page map[string]json.RawMessage
		if err := json.Unmarshal(body, &page); err != nil {
			continue
		}
		if items, ok := page[key]; ok {
			var arr []json.RawMessage
			if err := json.Unmarshal(items, &arr); err == nil {
				allItems = append(allItems, arr...)
			}
		}
	}
	result, _ := json.MarshalIndent(allItems, "", "  ")
	return result
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

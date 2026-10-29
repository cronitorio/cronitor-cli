package lib

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/viper"
)

// Read-only MCP tools over the Cronitor read API: error groups, error
// events and log records. Unlike the cron job tools, which talk to the
// dashboard instance, these call the Cronitor API with CRONITOR_API_KEY
// and return the API's JSON. Arguments mirror the flags of cronitor errors
// and cronitor logs.

func readToolOptions(description string) []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	}
}

func withReadFilters(opts []mcp.ToolOption, series bool) []mcp.ToolOption {
	opts = append(opts,
		mcp.WithString("env", mcp.Description("Environment key")),
		mcp.WithString("monitor", mcp.Description("Job monitor key or code")),
		mcp.WithString("service", mcp.Description("Service monitor key or code")),
	)
	if series {
		opts = append(opts, mcp.WithString("series", mcp.Description("Run series")))
	}
	return opts
}

func withTimeRange(opts []mcp.ToolOption) []mcp.ToolOption {
	return append(opts,
		mcp.WithString("since", mcp.Description("Lookback such as 30m, 24h, 7d or 2w; sets start to now minus it. Do not combine with start")),
		mcp.WithString("start", mcp.Description("Range start: unix seconds or ISO 8601")),
		mcp.WithString("end", mcp.Description("Range end: unix seconds or ISO 8601")),
	)
}

func (h *CronitorMCPHandler) registerReadTools(s *server.MCPServer) {
	opts := readToolOptions("Search error groups (exceptions grouped by fingerprint) sent by cronitor exec --capture-errors, cronitor-python or Sentry SDKs. Returns {page, page_count, page_size, total_count, data: [group]}, most recently seen first by default.")
	opts = withTimeRange(withReadFilters(opts, false))
	opts = append(opts,
		mcp.WithString("level", mcp.Description("Levels, comma-separated: fatal, error, warning, info, debug")),
		mcp.WithString("search", mcp.Description("Text to match in titles and culprits")),
		mcp.WithString("order_by", mcp.Description("Sort order"), mcp.Enum("-last_seen", "last_seen", "-count", "count", "-first_seen", "first_seen")),
		mcp.WithNumber("page", mcp.Description("Page number, from 1")),
		mcp.WithNumber("page_size", mcp.Description("Groups per page (default 50, max 200)")),
	)
	s.AddTool(mcp.NewTool("search_errors", opts...), h.handleSearchErrors)

	opts = readToolOptions("Get one error group by its full 40-character hex id: the group fields, a 24-point hourly sparkline (oldest first) and latest_event with its full payload (stack trace, breadcrumbs, contexts).")
	opts = append(opts, mcp.WithString("group", mcp.Required(), mcp.Description("Full 40-character hex group id, as returned by search_errors")))
	s.AddTool(mcp.NewTool("get_error_group", opts...), h.handleGetErrorGroup)

	opts = readToolOptions("Get one event of an error group with its full payload when event_id is given. Without event_id, list the group's events newest first as {data, has_more, next}; pass next as before for the page after.")
	opts = append(opts,
		mcp.WithString("group", mcp.Required(), mcp.Description("Full 40-character hex group id")),
		mcp.WithString("event_id", mcp.Description("32-character hex event id; omit to list events")),
	)
	opts = withReadFilters(opts, true)
	opts = append(opts,
		mcp.WithString("before", mcp.Description("Cursor: events before this time (unix seconds, ISO 8601, or a previous next value)")),
		mcp.WithNumber("limit", mcp.Description("Events to return (default 50, max 200)")),
	)
	s.AddTool(mcp.NewTool("get_error_event", opts...), h.handleGetErrorEvent)

	opts = readToolOptions("Search log records sent by cronitor exec --ship-logs, cronitor-python or Sentry SDKs. Returns {data: [record], has_more, next}, newest first; pass next as before for older records.")
	opts = withTimeRange(withReadFilters(opts, true))
	opts = append(opts,
		mcp.WithString("level", mcp.Description("Minimum level"), mcp.Enum("trace", "debug", "info", "warn", "error", "fatal")),
		mcp.WithString("search", mcp.Description("Whole tokens the body must contain, separated by spaces; all must match")),
		mcp.WithString("trace_id", mcp.Description("Trace id")),
		mcp.WithString("before", mcp.Description("Cursor: records before this time (unix seconds, ISO 8601, or a previous next value)")),
		mcp.WithNumber("limit", mcp.Description("Records to return (default 200, max 1000)")),
	)
	s.AddTool(mcp.NewTool("search_logs", opts...), h.handleSearchLogs)
}

// toolStart resolves the since/start arguments into start=.
func toolStart(req mcp.CallToolRequest) (string, error) {
	since := req.GetString("since", "")
	start := req.GetString("start", "")
	if since == "" {
		return start, nil
	}
	if start != "" {
		return "", fmt.Errorf("pass since or start, not both")
	}
	s, err := LookbackStart(since, time.Now())
	if err != nil {
		return "", fmt.Errorf("invalid since: %v", err)
	}
	return s, nil
}

func (h *CronitorMCPHandler) handleSearchErrors(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	start, err := toolStart(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	q := ErrorGroupQuery{
		Env:      req.GetString("env", ""),
		Monitor:  req.GetString("monitor", ""),
		Service:  req.GetString("service", ""),
		Levels:   strings.Split(req.GetString("level", ""), ","),
		Start:    start,
		End:      req.GetString("end", ""),
		Search:   req.GetString("search", ""),
		OrderBy:  req.GetString("order_by", ""),
		Page:     req.GetInt("page", 0),
		PageSize: req.GetInt("page_size", 0),
	}
	return h.readAPIGet(ErrorGroupsPath, q.Params())
}

func (h *CronitorMCPHandler) handleGetErrorGroup(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	group, err := NormalizeErrorGroupID(req.GetString("group", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return h.readAPIGet(ErrorGroupPath(group), nil)
}

func (h *CronitorMCPHandler) handleGetErrorEvent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	group, err := NormalizeErrorGroupID(req.GetString("group", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if raw := req.GetString("event_id", ""); raw != "" {
		eventID, err := NormalizeErrorEventID(raw)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return h.readAPIGet(ErrorEventPath(group, eventID), nil)
	}
	q := ErrorEventQuery{
		Env:     req.GetString("env", ""),
		Monitor: req.GetString("monitor", ""),
		Service: req.GetString("service", ""),
		Series:  req.GetString("series", ""),
		Before:  req.GetString("before", ""),
		Limit:   req.GetInt("limit", 0),
	}
	return h.readAPIGet(ErrorEventsPath(group), q.Params())
}

func (h *CronitorMCPHandler) handleSearchLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	start, err := toolStart(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	q := LogQuery{
		Env:     req.GetString("env", ""),
		Monitor: req.GetString("monitor", ""),
		Service: req.GetString("service", ""),
		Series:  req.GetString("series", ""),
		Level:   req.GetString("level", ""),
		Start:   start,
		End:     req.GetString("end", ""),
		Search:  req.GetString("search", ""),
		TraceID: req.GetString("trace_id", ""),
		Before:  req.GetString("before", ""),
		Limit:   req.GetInt("limit", 0),
	}
	return h.readAPIGet(LogsPath, q.Params())
}

// readAPIGet calls the Cronitor API and returns its JSON body, or the API's
// error as a tool error.
func (h *CronitorMCPHandler) readAPIGet(path string, params map[string]string) (*mcp.CallToolResult, error) {
	if len(viper.GetString("CRONITOR_API_KEY")) < 10 {
		return mcp.NewToolResultError("CRONITOR_API_KEY is not configured. Run 'cronitor configure' with an API key that has the monitor:read scope, or set CRONITOR_API_KEY in the MCP server's environment."), nil
	}
	client := NewAPIClient(h.UseDev, nil)
	client.UserAgent = "CronitorCLI MCP"
	resp, err := client.GET(path, params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Cronitor API request failed: %v", err)), nil
	}
	if !resp.IsSuccess() {
		msg := fmt.Sprintf("Cronitor API error %d: %s", resp.StatusCode, resp.ParseError())
		if resp.StatusCode == 403 {
			msg += " (errors and logs need an API key with the monitor:read scope)"
		}
		return mcp.NewToolResultError(msg), nil
	}
	return mcp.NewToolResultText(string(resp.Body)), nil
}

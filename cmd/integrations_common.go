package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"golang.org/x/term"
)

// Test hooks. Production uses the real implementations; tests override these.
var (
	exitFn        = os.Exit
	sleepFn       = time.Sleep
	openBrowserFn = openBrowser
	readSecretFn  = readSecretFromTerminal
	nowFn         = time.Now
)

var integrationSecretValues []string

func resetSecretRedaction() {
	integrationSecretValues = nil
}

func rememberSecret(val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	integrationSecretValues = append(integrationSecretValues, val)
}

func newIntegrationAPIClient() *lib.APIClient {
	return lib.NewAPIClient(dev, redactSecretsLog)
}

func redactSecretsLog(msg string) {
	log(redactIntegrationLogMessage(msg))
}

func redactIntegrationLogMessage(msg string) string {
	for _, secret := range integrationSecretValues {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[REDACTED]")
		}
	}
	if idx := strings.Index(msg, "{"); idx >= 0 {
		if redacted, ok := redactSecretJSON(msg[idx:]); ok {
			msg = msg[:idx] + redacted
		}
	}
	return msg
}

func redactSecretJSON(s string) (string, bool) {
	var v interface{}
	if json.Unmarshal([]byte(s), &v) != nil {
		return s, false
	}
	redactSecretWalk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return s, false
	}
	return string(out), true
}

func redactSecretWalk(v interface{}) {
	switch node := v.(type) {
	case map[string]interface{}:
		for k, child := range node {
			if strings.EqualFold(k, "fields") {
				redactRequestFieldsObject(child)
				continue
			}
			if isSecretFieldKey(k) && isScalar(child) {
				node[k] = "[REDACTED]"
				continue
			}
			redactSecretWalk(child)
		}
	case []interface{}:
		for _, child := range node {
			redactSecretWalk(child)
		}
	}
}

// redactRequestFieldsObject redacts secret values in a request-body fields map
// ({"api_key":"secret"}). Catalogue field metadata objects
// ({"api_key":{"label":"...","required":true,"secret":true}}) are left intact.
func redactRequestFieldsObject(v interface{}) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return
	}
	for k, child := range m {
		switch child.(type) {
		case map[string]interface{}, []interface{}:
			// Public catalogue metadata (label, required, secret flags).
			continue
		default:
			m[k] = "[REDACTED]"
		}
	}
}

func isScalar(v interface{}) bool {
	switch v.(type) {
	case map[string]interface{}, []interface{}:
		return false
	default:
		return true
	}
}

func isSecretFieldKey(k string) bool {
	switch strings.ToLower(k) {
	case "api_key", "apikey", "token", "secret", "password", "authorization", "access_token", "bot_token":
		return true
	}
	lower := strings.ToLower(k)
	return strings.HasSuffix(lower, "_secret") || strings.HasSuffix(lower, "_token") || strings.HasSuffix(lower, "_password")
}

type catalogueService struct {
	Service      string          `json:"service"`
	ServiceName  string          `json:"service_name"`
	Type         string          `json:"type"`
	Method       string          `json:"method"`
	Fields       json.RawMessage `json:"fields"`
	Available    bool            `json:"available"`
	Instructions string          `json:"instructions"`
	Message      string          `json:"message"`
	HelpText     string          `json:"help_text"`
}

type catalogueField struct {
	Key      string
	Label    string
	Secret   bool
	Required bool
	Help     string
}

type integrationRecord struct {
	ID          string          `json:"id"`
	Service     string          `json:"service"`
	ServiceName string          `json:"service_name"`
	Method      string          `json:"method"`
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Identifier  string          `json:"identifier"`
	Available   bool            `json:"available"`
	Metadata    json.RawMessage `json:"metadata"`
	Created     string          `json:"created"`
}

func readSecretFromTerminal(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		fmt.Fprintln(os.Stderr)
		return "", fmt.Errorf("not a terminal")
	}
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

func parseFieldFlags(fields []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, f := range fields {
		key, val, ok := strings.Cut(f, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --field %q (expected key=value)", f)
		}
		out[strings.TrimSpace(key)] = val
		rememberSecret(val)
	}
	return out, nil
}

func parseCatalogueFields(raw json.RawMessage) []catalogueField {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" || string(raw) == "[]" {
		return nil
	}

	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err == nil {
		keys := make([]string, 0, len(asMap))
		for k := range asMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fields := make([]catalogueField, 0, len(keys))
		for _, k := range keys {
			f := catalogueField{Key: k, Label: k, Required: true, Secret: true}
			var obj struct {
				Label    string `json:"label"`
				Name     string `json:"name"`
				Secret   *bool  `json:"secret"`
				Required *bool  `json:"required"`
				Help     string `json:"help"`
				Prompt   string `json:"prompt"`
			}
			if json.Unmarshal(asMap[k], &obj) == nil && (obj.Label != "" || obj.Name != "" || obj.Secret != nil || obj.Required != nil || obj.Help != "" || obj.Prompt != "") {
				if obj.Label != "" {
					f.Label = obj.Label
				} else if obj.Name != "" {
					f.Label = obj.Name
				} else if obj.Prompt != "" {
					f.Label = obj.Prompt
				}
				if obj.Secret != nil {
					f.Secret = *obj.Secret
				}
				if obj.Required != nil {
					f.Required = *obj.Required
				}
				f.Help = obj.Help
			} else {
				var s string
				if json.Unmarshal(asMap[k], &s) == nil && s != "" {
					f.Label = s
				}
			}
			fields = append(fields, f)
		}
		return fields
	}

	var asArr []struct {
		Key      string `json:"key"`
		Name     string `json:"name"`
		Label    string `json:"label"`
		Secret   *bool  `json:"secret"`
		Required *bool  `json:"required"`
		Help     string `json:"help"`
	}
	if err := json.Unmarshal(raw, &asArr); err == nil {
		fields := make([]catalogueField, 0, len(asArr))
		for _, item := range asArr {
			key := item.Key
			if key == "" {
				key = item.Name
			}
			if key == "" {
				continue
			}
			f := catalogueField{Key: key, Label: key, Required: true, Secret: true}
			if item.Label != "" {
				f.Label = item.Label
			} else if item.Name != "" && item.Key != "" {
				f.Label = item.Name
			}
			if item.Secret != nil {
				f.Secret = *item.Secret
			}
			if item.Required != nil {
				f.Required = *item.Required
			}
			f.Help = item.Help
			fields = append(fields, f)
		}
		return fields
	}

	return nil
}

func fetchCatalogue(client *lib.APIClient) ([]catalogueService, []byte, error) {
	resp, err := client.GET("/integrations/services", nil)
	if err != nil {
		return nil, nil, err
	}
	if !resp.IsSuccess() {
		return nil, resp.Body, fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
	}

	var wrapper struct {
		Services []catalogueService `json:"services"`
	}
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, resp.Body, fmt.Errorf("failed to parse services catalogue: %w", err)
	}
	return wrapper.Services, resp.Body, nil
}

func findCatalogueService(services []catalogueService, key string) (catalogueService, bool) {
	for _, s := range services {
		if s.Service == key {
			return s, true
		}
	}
	return catalogueService{}, false
}

func catalogueServiceKeys(services []catalogueService) []string {
	keys := make([]string, 0, len(services))
	for _, s := range services {
		keys = append(keys, s.Service)
	}
	return keys
}

func connectFlowFor(svc catalogueService) string {
	method := strings.ToLower(strings.TrimSpace(svc.Method))
	if method == "" {
		method = strings.ToLower(strings.TrimSpace(svc.Type))
	}
	switch svc.Service {
	case "telegram":
		return "telegram"
	case "slack", "pagerduty":
		if method == "" {
			return "oauth"
		}
	}
	switch method {
	case "oauth", "connect", "browser", "authorization":
		return "oauth"
	case "telegram":
		return "telegram"
	default:
		return "apikey"
	}
}

func renderServicesTable(services []catalogueService) string {
	table := &UITable{
		Headers: []string{"SERVICE", "METHOD", "AVAILABLE"},
	}
	for _, s := range services {
		method := s.Method
		if method == "" {
			method = s.Type
		}
		table.Rows = append(table.Rows, []string{
			s.Service,
			method,
			strconv.FormatBool(s.Available),
		})
	}
	return table.Render()
}

func listIntegrations(client *lib.APIClient, params map[string]string) ([]integrationRecord, []byte, error) {
	resp, err := client.GET("/integrations", params)
	if err != nil {
		return nil, nil, err
	}
	if !resp.IsSuccess() {
		return nil, resp.Body, fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
	}

	records, err := parseIntegrationList(resp.Body)
	if err != nil {
		return nil, resp.Body, err
	}
	return records, resp.Body, nil
}

func parseIntegrationList(body []byte) ([]integrationRecord, error) {
	var wrapper struct {
		Integrations []integrationRecord `json:"integrations"`
		Results      []integrationRecord `json:"results"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil {
		if len(wrapper.Integrations) > 0 {
			return wrapper.Integrations, nil
		}
		if len(wrapper.Results) > 0 {
			return wrapper.Results, nil
		}
		// Distinguish empty list object from a single record.
		var probe map[string]json.RawMessage
		if json.Unmarshal(body, &probe) == nil {
			if _, ok := probe["integrations"]; ok {
				return []integrationRecord{}, nil
			}
			if _, ok := probe["results"]; ok {
				return []integrationRecord{}, nil
			}
		}
	}

	var arr []integrationRecord
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}

	var single integrationRecord
	if err := json.Unmarshal(body, &single); err == nil && single.ID != "" {
		return []integrationRecord{single}, nil
	}

	return nil, fmt.Errorf("failed to parse integrations list")
}

func writeCLIOutput(outputPath, content string) {
	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", outputPath, err))
			exitFn(1)
			return
		}
		Info(fmt.Sprintf("Output written to %s", outputPath))
		return
	}
	fmt.Println(content)
}

func parseTimeoutFlag(s string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 15 * time.Minute, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	return 0, fmt.Errorf("invalid --timeout %q (use a duration like 15m or 30s)", s)
}

func parseExpiresAt(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n)
		}
		return time.Unix(n, 0)
	}
	return time.Time{}
}

func durationFromJSON(raw json.RawMessage) time.Duration {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		return time.Duration(n * float64(time.Second))
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
		if i, err := strconv.Atoi(s); err == nil && i > 0 {
			return time.Duration(i) * time.Second
		}
	}
	return 0
}

func defaultPollInterval(d time.Duration) time.Duration {
	if d <= 0 {
		return 2 * time.Second
	}
	return d
}

func promptCatalogueFields(fields []catalogueField, provided map[string]string) (map[string]string, error) {
	out := make(map[string]string)
	for k, v := range provided {
		out[k] = v
	}
	for _, field := range fields {
		if _, ok := out[field.Key]; ok {
			continue
		}
		label := field.Label
		if label == "" {
			label = field.Key
		}
		if field.Help != "" {
			fmt.Fprintln(os.Stderr, field.Help)
		}
		val, err := readSecretFn(fmt.Sprintf("%s: ", label))
		if err != nil {
			if field.Required {
				return nil, fmt.Errorf("required field %q not provided (use --field %s=<value> or run in a terminal)", field.Key, field.Key)
			}
			continue
		}
		val = strings.TrimSpace(val)
		if val == "" {
			if field.Required {
				return nil, fmt.Errorf("required field %q cannot be empty", field.Key)
			}
			continue
		}
		out[field.Key] = val
		rememberSecret(val)
	}
	return out, nil
}

func integrationDisplayName(rec integrationRecord) string {
	if rec.Label != "" {
		return rec.Label
	}
	if rec.Name != "" {
		return rec.Name
	}
	return rec.ID
}

func extractConnectIdentity(body []byte) (id, label, service string) {
	var rec struct {
		ID          string `json:"id"`
		Label       string `json:"label"`
		Name        string `json:"name"`
		Service     string `json:"service"`
		Integration *struct {
			ID      string `json:"id"`
			Label   string `json:"label"`
			Name    string `json:"name"`
			Service string `json:"service"`
		} `json:"integration"`
	}
	if err := json.Unmarshal(body, &rec); err != nil {
		return "", "", ""
	}
	id = rec.ID
	label = rec.Label
	if label == "" {
		label = rec.Name
	}
	service = rec.Service
	if rec.Integration != nil {
		if id == "" {
			id = rec.Integration.ID
		}
		if label == "" {
			label = rec.Integration.Label
		}
		if label == "" {
			label = rec.Integration.Name
		}
		if service == "" {
			service = rec.Integration.Service
		}
	}
	return id, label, service
}

func isAmbiguousLabelError(resp *lib.APIResponse) bool {
	if resp == nil {
		return false
	}
	if resp.IsSuccess() {
		return false
	}
	msg := strings.ToLower(resp.ParseError())
	if strings.Contains(msg, "ambiguous") {
		return true
	}
	body := strings.ToLower(string(resp.Body))
	return strings.Contains(body, "ambiguous")
}

func toStringSlice(v interface{}) []string {
	switch typed := v.(type) {
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		if typed == "" {
			return []string{}
		}
		return []string{typed}
	default:
		return []string{}
	}
}

func notificationChannelKey(service string) string {
	switch service {
	case "webhook":
		return "webhooks"
	default:
		return service
	}
}

func parseNotificationListObject(body []byte) (map[string]interface{}, error) {
	var list map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	if _, ok := list["notifications"]; ok {
		return list, nil
	}
	if inner, ok := list["template"].(map[string]interface{}); ok {
		return inner, nil
	}
	return list, nil
}

func writableNotificationListBody(list map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, k := range []string{"key", "name", "notifications"} {
		if v, ok := list[k]; ok {
			out[k] = v
		}
	}
	return out
}

func notificationChannelContains(body []byte, channel, value string) bool {
	list, err := parseNotificationListObject(body)
	if err != nil {
		return false
	}
	notifications, _ := list["notifications"].(map[string]interface{})
	if notifications == nil {
		return false
	}
	for _, item := range toStringSlice(notifications[channel]) {
		if item == value {
			return true
		}
	}
	return false
}

func notificationEntryExists(existing []string, value, id string) bool {
	for _, item := range existing {
		if item == value || (id != "" && item == id) {
			return true
		}
	}
	return false
}

func verifyNotificationUpdate(client *lib.APIClient, listKey, channel, value string, putResp *lib.APIResponse) bool {
	if putResp != nil && notificationChannelContains(putResp.Body, channel, value) {
		return true
	}
	resp, err := client.GET(fmt.Sprintf("/notifications/%s", listKey), nil)
	if err != nil || !resp.IsSuccess() {
		return false
	}
	return notificationChannelContains(resp.Body, channel, value)
}

func addToNotificationList(client *lib.APIClient, listKey, service, label, id string) error {
	if listKey == "" {
		return nil
	}
	if service == "" {
		return fmt.Errorf("cannot add to notification list: missing service")
	}

	resp, err := client.GET(fmt.Sprintf("/notifications/%s", listKey), nil)
	if err != nil {
		return fmt.Errorf("failed to get notification list %s: %w", listKey, err)
	}
	if resp.IsNotFound() {
		return fmt.Errorf("notification list '%s' not found", listKey)
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
	}

	list, err := parseNotificationListObject(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to parse notification list: %w", err)
	}

	notifications, _ := list["notifications"].(map[string]interface{})
	if notifications == nil {
		notifications = map[string]interface{}{}
		list["notifications"] = notifications
	}

	channel := notificationChannelKey(service)
	value := label
	if value == "" {
		value = id
	}
	if value == "" {
		return fmt.Errorf("cannot add to notification list: missing integration label")
	}

	existing := toStringSlice(notifications[channel])
	if notificationEntryExists(existing, value, id) {
		return nil
	}
	existing = append(existing, value)
	notifications[channel] = existing

	putBody, err := json.Marshal(writableNotificationListBody(list))
	if err != nil {
		return err
	}

	putResp, err := client.PUT(fmt.Sprintf("/notifications/%s", listKey), putBody, nil)
	if err != nil {
		return fmt.Errorf("failed to update notification list %s: %w", listKey, err)
	}
	if putResp.IsSuccess() {
		if verifyNotificationUpdate(client, listKey, channel, value, putResp) {
			return nil
		}
		return fmt.Errorf("notification list update was not applied")
	}
	if isAmbiguousLabelError(putResp) && id != "" && id != value {
		existing[len(existing)-1] = id
		notifications[channel] = existing
		retryBody, marshalErr := json.Marshal(writableNotificationListBody(list))
		if marshalErr != nil {
			return marshalErr
		}
		retryResp, retryErr := client.PUT(fmt.Sprintf("/notifications/%s", listKey), retryBody, nil)
		if retryErr != nil {
			return fmt.Errorf("failed to update notification list %s: %w", listKey, retryErr)
		}
		if retryResp.IsSuccess() {
			if verifyNotificationUpdate(client, listKey, channel, id, retryResp) {
				return nil
			}
			return fmt.Errorf("notification list update was not applied")
		}
		return fmt.Errorf("API Error (%d): %s", retryResp.StatusCode, retryResp.ParseError())
	}
	return fmt.Errorf("API Error (%d): %s", putResp.StatusCode, putResp.ParseError())
}

func printConnected(id, label, service string, raw []byte, format, outputPath string) {
	if format == "json" && len(raw) > 0 {
		writeCLIOutput(outputPath, FormatJSON(raw))
		return
	}
	display := label
	if display == "" {
		display = id
	}
	if id != "" && label != "" && id != label {
		Success(fmt.Sprintf("Connected %s %s (%s)", service, label, id))
	} else if display != "" {
		Success(fmt.Sprintf("Connected %s", display))
	} else {
		Success("Integration connected")
	}
	if format == "json" && len(raw) > 0 {
		writeCLIOutput(outputPath, FormatJSON(raw))
	}
}

func failAndExit(msg string) {
	Error(msg)
	exitFn(1)
}

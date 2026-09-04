package cmd

import (
	"bufio"
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
	readLineFn    = readLineFromTerminal
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

// redactIntegrationLogMessage scrubs one client log line. Values the user
// typed or passed with --field are replaced wherever they appear. In a
// "Request Body" line every value under "fields" is a credential, so the whole
// map is blanked. Everywhere else, including the catalogue response whose
// "fields" map holds labels, only secret-named keys are redacted.
func redactIntegrationLogMessage(msg string) string {
	for _, secret := range integrationSecretValues {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[REDACTED]")
		}
	}
	idx := strings.Index(msg, "{")
	if idx < 0 {
		return msg
	}
	var v interface{}
	if json.Unmarshal([]byte(msg[idx:]), &v) != nil {
		return msg
	}
	redactSecretWalk(v, strings.HasPrefix(msg, "Request Body:"))
	out, err := json.Marshal(v)
	if err != nil {
		return msg
	}
	return msg[:idx] + string(out)
}

func redactSecretWalk(v interface{}, requestBody bool) {
	switch node := v.(type) {
	case map[string]interface{}:
		for k, child := range node {
			if requestBody && strings.EqualFold(k, "fields") {
				redactAllValues(child)
				continue
			}
			if isSecretFieldKey(k) && isScalar(child) {
				node[k] = "[REDACTED]"
				continue
			}
			redactSecretWalk(child, requestBody)
		}
	case []interface{}:
		for _, child := range node {
			redactSecretWalk(child, requestBody)
		}
	}
}

// redactAllValues blanks every scalar under a request-body fields map,
// including nested objects and arrays.
func redactAllValues(v interface{}) {
	switch node := v.(type) {
	case map[string]interface{}:
		for k, child := range node {
			if isScalar(child) {
				node[k] = "[REDACTED]"
			} else {
				redactAllValues(child)
			}
		}
	case []interface{}:
		for i, child := range node {
			if isScalar(child) {
				node[i] = "[REDACTED]"
			} else {
				redactAllValues(child)
			}
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
	Service     string          `json:"service"`
	ServiceName string          `json:"service_name"`
	Type        string          `json:"type"`
	Method      string          `json:"method"`
	Fields      json.RawMessage `json:"fields"`
	Available   bool            `json:"available"`
}

type catalogueField struct {
	Key    string
	Label  string
	Secret bool
}

type integrationRecord struct {
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

// readLineFromTerminal prompts on stderr and reads one echoed line from stdin.
// It is used for catalogue fields that are not secrets, such as a username.
func readLineFromTerminal(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// catalogueFieldIsSecret decides the prompt style when the catalogue gives no
// explicit flag. Everything is hidden except a short allowlist of plain
// identifiers; "key" is always a webhook URL or API key on this API, and
// datadog-on-call's webhook_url is an intake host that may not carry credentials.
func catalogueFieldIsSecret(key string) bool {
	switch strings.ToLower(key) {
	case "username", "user", "oncall_team", "team", "channel", "name", "type", "email", "webhook_url":
		return false
	default:
		return true
	}
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

// parseCatalogueFields reads the catalogue's fields map, which is
// {"<key>": "<label>"}. Keys are returned in sorted order.
func parseCatalogueFields(raw json.RawMessage) []catalogueField {
	var asMap map[string]string
	if len(raw) == 0 || json.Unmarshal(raw, &asMap) != nil {
		return nil
	}
	keys := make([]string, 0, len(asMap))
	for k := range asMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fields := make([]catalogueField, 0, len(keys))
	for _, k := range keys {
		label := asMap[k]
		if label == "" {
			label = k
		}
		fields = append(fields, catalogueField{Key: k, Label: label, Secret: catalogueFieldIsSecret(k)})
	}
	return fields
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

// connectFlowFor maps the catalogue method to a connect flow. The API uses
// oauth (slack, pagerduty), link (telegram), and apikey for everything else.
func connectFlowFor(svc catalogueService) string {
	switch strings.ToLower(strings.TrimSpace(svc.Method)) {
	case "oauth":
		return "oauth"
	case "link":
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
		table.Rows = append(table.Rows, []string{
			s.Service,
			s.Method,
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
		Integrations *[]integrationRecord `json:"integrations"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil || wrapper.Integrations == nil {
		return nil, fmt.Errorf("failed to parse integrations list")
	}
	return *wrapper.Integrations, nil
}

// integrationNotFoundError is returned when no visible integration matches a label.
type integrationNotFoundError struct{ label string }

func (e integrationNotFoundError) Error() string {
	return fmt.Sprintf("Integration '%s' not found", e.label)
}

// integrationAmbiguousError is returned when a label exists on more than one service.
type integrationAmbiguousError struct {
	label    string
	services []string
}

func (e integrationAmbiguousError) Error() string {
	return fmt.Sprintf("Integration '%s' exists on multiple services (%s). Pass --service to choose one", e.label, strings.Join(e.services, ", "))
}

// rawIntegrationItems returns the raw JSON of each list item so a single row
// can be printed without losing fields the typed record does not model.
func rawIntegrationItems(body []byte) []json.RawMessage {
	var wrapper struct {
		Integrations []json.RawMessage `json:"integrations"`
	}
	if json.Unmarshal(body, &wrapper) != nil {
		return nil
	}
	return wrapper.Integrations
}

// resolveIntegrationByLabel finds exactly one integration through the list
// filter (GET /integrations?service=&label=). The API has no path-key Get.
func resolveIntegrationByLabel(client *lib.APIClient, label, service string) (integrationRecord, json.RawMessage, error) {
	params := map[string]string{"label": label}
	if service != "" {
		params["service"] = service
	}
	records, body, err := listIntegrations(client, params)
	if err != nil {
		return integrationRecord{}, nil, err
	}
	raws := rawIntegrationItems(body)

	var matched []int
	for i, rec := range records {
		if integrationPublicLabel(rec) != label {
			continue
		}
		if service != "" && rec.Service != "" && rec.Service != service {
			continue
		}
		matched = append(matched, i)
	}

	switch len(matched) {
	case 0:
		return integrationRecord{}, nil, integrationNotFoundError{label: label}
	case 1:
		i := matched[0]
		var raw json.RawMessage
		if len(raws) == len(records) {
			raw = raws[i]
		} else if encoded, err := json.Marshal(records[i]); err == nil {
			raw = encoded
		}
		return records[i], raw, nil
	default:
		services := make([]string, 0, len(matched))
		for _, i := range matched {
			services = append(services, records[i].Service)
		}
		return integrationRecord{}, nil, integrationAmbiguousError{label: label, services: services}
	}
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

// parseExpiresAt reads the ISO 8601 timestamp the API returns for expires_at.
func parseExpiresAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}

func secondsToDuration(n float64) time.Duration {
	if n <= 0 {
		return 0
	}
	return time.Duration(n * float64(time.Second))
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
		read := readSecretFn
		if !field.Secret {
			read = readLineFn
		}
		val, err := read(fmt.Sprintf("%s: ", field.Label))
		if err != nil {
			return nil, fmt.Errorf("field %q not provided (use --field %s=<value> or run in a terminal)", field.Key, field.Key)
		}
		val = strings.TrimSpace(val)
		if val == "" {
			return nil, fmt.Errorf("field %q cannot be empty", field.Key)
		}
		out[field.Key] = val
		rememberSecret(val)
	}
	return out, nil
}

func integrationPublicLabel(rec integrationRecord) string {
	return publicLabel(rec.Label, rec.Name)
}

// publicLabel is the only public identity of an integration. The API returns
// label on every row; name is kept as a fallback for older responses.
func publicLabel(label, name string) string {
	if label != "" {
		return label
	}
	return name
}

func extractPublicIdentity(body []byte) (label, service string) {
	var rec struct {
		Label       string `json:"label"`
		Name        string `json:"name"`
		Service     string `json:"service"`
		Integration *struct {
			Label   string `json:"label"`
			Name    string `json:"name"`
			Service string `json:"service"`
		} `json:"integration"`
	}
	if err := json.Unmarshal(body, &rec); err != nil {
		return "", ""
	}
	label = publicLabel(rec.Label, rec.Name)
	service = rec.Service
	if rec.Integration != nil {
		if label == "" {
			label = publicLabel(rec.Integration.Label, rec.Integration.Name)
		}
		if service == "" {
			service = rec.Integration.Service
		}
	}
	return label, service
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

func notificationEntryExists(existing []string, value string) bool {
	for _, item := range existing {
		if item == value {
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

func addToNotificationList(client *lib.APIClient, listKey, service, label string) error {
	if listKey == "" {
		return nil
	}
	if service == "" {
		return fmt.Errorf("cannot add to notification list: missing service")
	}
	if label == "" {
		return fmt.Errorf("cannot add to notification list: missing integration label")
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
	existing := toStringSlice(notifications[channel])
	if notificationEntryExists(existing, label) {
		return nil
	}
	existing = append(existing, label)
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
		if verifyNotificationUpdate(client, listKey, channel, label, putResp) {
			return nil
		}
		return fmt.Errorf("notification list update was not applied")
	}
	return fmt.Errorf("API Error (%d): %s", putResp.StatusCode, putResp.ParseError())
}

func printConnected(label, service string, raw []byte, format, outputPath string) {
	if format == "json" && len(raw) > 0 {
		writeCLIOutput(outputPath, FormatJSON(raw))
		return
	}
	if label != "" && service != "" {
		Success(fmt.Sprintf("Connected %s %s", service, label))
		return
	}
	if label != "" {
		Success(fmt.Sprintf("Connected %s", label))
		return
	}
	Success("Integration connected")
}

func failAndExit(msg string) {
	Error(msg)
	exitFn(1)
}

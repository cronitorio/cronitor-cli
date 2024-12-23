package lib

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getsentry/raven-go"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type RuleValue string

type Rule struct {
	RuleType     string    `json:"rule_type"`
	Value        RuleValue `json:"value"`
	TimeUnit     string    `json:"time_unit,omitempty"`
	GraceSeconds uint      `json:"grace_seconds,omitempty"`
}

type Platform string

const (
	CRON       Platform = "cron"
	WINDOWS    Platform = "windows"
	KUBERNETES Platform = "kubernetes"
	JVM        Platform = "jvm"
	LARAVEL    Platform = "laravel"
	MAGENTO    Platform = "magento"
	SIDEKIQ    Platform = "sidekiq"
	CELERY     Platform = "celery"
	JENKINS    Platform = "jenkins"
	QUARTZ     Platform = "quartz"
	SPRING     Platform = "spring"
	CLOUDWATCH Platform = "cloudwatch"
	NODECRON   Platform = "node-cron"
)

type Monitor struct {
	Attributes struct {
		GroupName string `json:"group_name"`
		Key       string `json:"key"`
		Code      string `json:"code"`
	} `json:"attributes,omitempty"`
	Name             string   `json:"name,omitempty"`
	DefaultName      string   `json:"defaultName"`
	Key              string   `json:"key"`
	Schedule         string   `json:"schedule,omitempty"`
	Platform         Platform `json:"platform,omitempty"`
	Tags             []string `json:"tags"`
	Type             string   `json:"type"`
	Code             string   `json:"code,omitempty"`
	Timezone         string   `json:"timezone,omitempty"`
	Note             string   `json:"defaultNote,omitempty"`
	Notify           []string `json:"notify,omitempty"`
	NoStdoutPassthru bool     `json:"-"`
}

type MonitorSummary struct {
	Name        string `json:"name,omitempty"`
	DefaultName string `json:"defaultName"`
	Key         string `json:"key"`
	Code        string `json:"attributes.code,omitempty"`
	Attributes  struct {
		GroupName string `json:"group_name"`
		Key       string `json:"key"`
		Code      string `json:"code"`
	} `json:"attributes,omitempty"`
}

type CronitorApi struct {
	IsDev          bool
	IsAutoDiscover bool
	ApiKey         string
	UserAgent      string
	Logger         func(string)
}

type SignupResponse struct {
	ApiKey     string `json:"api_key"`
	PingApiKey string `json:"ping_api_key"`
}

func (fi *RuleValue) UnmarshalJSON(b []byte) error {
	if b[0] == '"' {
		return json.Unmarshal(b, (*string)(fi))
	}

	var i int
	if err := json.Unmarshal(b, &i); err != nil {
		return err
	}
	s := strconv.Itoa(i)

	*fi = RuleValue(s)
	return nil
}

func (api CronitorApi) PutMonitors(monitors map[string]*Monitor) (map[string]*Monitor, error) {
	url := api.Url()
	if api.IsAutoDiscover {
		url = url + "?auto-discover=1"
	}

	monitorsArray := make([]Monitor, 0, len(monitors))
	for _, v := range monitors {
		monitorsArray = append(monitorsArray, *v)
	}

	jsonBytes, _ := json.Marshal(monitorsArray)
	jsonString := string(jsonBytes)

	buf := new(bytes.Buffer)
	json.Indent(buf, jsonBytes, "", "  ")
	api.Logger("\nRequest:")
	api.Logger(buf.String() + "\n")

	response, err, _ := api.send("PUT", url, jsonString)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Request to %s failed: %s", url, err))
	}

	buf.Truncate(0)
	json.Indent(buf, response, "", "  ")
	api.Logger("\nResponse:")
	api.Logger(buf.String() + "\n")

	responseMonitors := []Monitor{}
	if err = json.Unmarshal(response, &responseMonitors); err != nil {
		return nil, errors.New(fmt.Sprintf("Error from %s: %s", url, response))
	}

	for _, value := range responseMonitors {
		if _, ok := monitors[value.Attributes.Key]; ok {
			monitors[value.Attributes.Key].Attributes = value.Attributes
		}
	}

	return monitors, nil
}

func (api CronitorApi) GetMonitors() ([]MonitorSummary, error) {
	url := api.Url()
	page := 1
	monitors := []MonitorSummary{}

	for {
		response, err, _ := api.send("GET", fmt.Sprintf("%s?page=%d", url, page), "")
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Request to %s failed: %s", url, err))
		}

		type ExpectedResponse struct {
			TotalMonitorCount int              `json:"total_monitor_count"`
			PageSize          int              `json:"page_size"`
			Monitors          []MonitorSummary `json:"monitors"`
		}

		responseMonitors := ExpectedResponse{}
		if err = json.Unmarshal(response, &responseMonitors); err != nil {
			return nil, errors.New(fmt.Sprintf("Error from %s: %s", url, err.Error()))
		}

		monitors = append(monitors, responseMonitors.Monitors...)
		if page*responseMonitors.PageSize >= responseMonitors.TotalMonitorCount {
			break
		}

		page += 1
	}

	return monitors, nil
}

func (api CronitorApi) GetRawResponse(url string) ([]byte, error) {
	response, err, _ := api.send("GET", url, "")
	return response, err
}

func (api CronitorApi) Url() string {
	if api.IsDev {
		return "http://dev.cronitor.io/api/monitors"
	} else {
		return "https://cronitor.io/api/monitors"
	}
}

func (api CronitorApi) send(method string, url string, body string) ([]byte, error, int) {
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	request.SetBasicAuth(viper.GetString(api.ApiKey), "")

	if strings.HasSuffix(url, "/signup") || strings.HasSuffix(url, "/sign-up") {
		request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	} else {
		request.Header.Add("Content-Type", "application/json")
	}

	request.Header.Add("User-Agent", api.UserAgent)
	request.Header.Add("Cronitor-Version", "2020-10-01")
	request.ContentLength = int64(len(body))
	response, err := client.Do(request)
	if err != nil {
		return nil, err, 0
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		return nil, err, 0
	}

	return contents, nil, response.StatusCode
}

func gzipLogData(logData string) *bytes.Buffer {
	var b bytes.Buffer
	if len(logData) < 1 {
		return &b
	}

	gz := gzip.NewWriter(&b)
	if _, err := gz.Write([]byte(logData)); err != nil {
		//log("error writing gzip")
		return nil
	}
	if err := gz.Close(); err != nil {
		//log("error closing gzip")
		return nil
	}
	return &b
}

func getPresignedUrl(apiKey string, postBody []byte) ([]byte, error) {
	url := "https://cronitor.io/api/logs/presign"

	api := CronitorApi{
		ApiKey:    apiKey,
		UserAgent: "cronitor-cli",
	}

	response, err, _ := api.send("POST", url, string(postBody))
	if err != nil {
		return nil, errors.Wrap(err, "error requesting presigned url")
	}

	return response, nil
}

func SendLogData(apiKey string, monitorKey string, seriesID string, outputLogs string) ([]byte, error) {
	gzippedLogs := gzipLogData(outputLogs)
	jsonBytes, err := json.Marshal(map[string]string{
		"job_key": monitorKey,
		"series":  seriesID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "couldn't encode job and series IDs to JSON")
	}

	var responseJson struct {
		Url string `json:"url"`
	}
	response, err := getPresignedUrl(apiKey, jsonBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error generating presign url for log uploading")
	}
	if err := json.Unmarshal(response, &responseJson); err != nil {
		return nil, err
	}

	s3LogPutUrl := responseJson.Url
	if len(s3LogPutUrl) == 0 {
		return nil, errors.New("no presigned S3 url returned. Something is wrong")
	}
	req, err := http.NewRequest("PUT", s3LogPutUrl, gzippedLogs)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

func (api CronitorApi) Signup(name string, email string, password string) (*SignupResponse, error) {
	payload := fmt.Sprintf("fullname=%s&email=%s&password=%s",
		url.QueryEscape(name),
		url.QueryEscape(email),
		url.QueryEscape(password))

	url := "https://cronitor.io/sign-up"
	if api.IsDev {
		url = "http://dev.cronitor.io/sign-up"
	}

	response, err, statusCode := api.send("POST", url, payload)
	if err != nil {
		return nil, err
	}

	if statusCode != 200 {
		return nil, fmt.Errorf("sign up failed (status %d): %s", statusCode, string(response))
	}

	if statusCode != 200 {
		return nil, fmt.Errorf("sign up failed: %d", statusCode)
	}

	var signupResp SignupResponse
	if err := json.Unmarshal(response, &signupResp); err != nil {
		return nil, fmt.Errorf("failed to parse signup response: %s", err)
	}

	return &signupResp, nil
}

package lib

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/getsentry/raven-go"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
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
	Code        string `json:"code,omitempty"`
}

type CronitorApi struct {
	IsDev          bool
	IsAutoDiscover bool
	ApiKey         string
	UserAgent      string
	Logger         func(string)
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

	response, err := api.sendHttpPut(url, jsonString)
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
		// We only need to update the Monitor struct with a code if this is a new monitor.
		// For updates the monitor code is sent as well as the key and that takes precedence.
		if _, ok := monitors[value.Key]; ok {
			monitors[value.Key].Code = value.Code
		}

	}

	return monitors, nil
}

func (api CronitorApi) GetMonitors() ([]MonitorSummary, error) {
	url := api.Url()
	page := 1
	monitors := []MonitorSummary{}

	for {
		response, err := api.sendHttpGet(fmt.Sprintf("%s?page=%d", url, page))
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
	client := &http.Client{}
	request, err := http.NewRequest("GET", url, nil)
	request.SetBasicAuth(viper.GetString(api.ApiKey), "")
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("User-Agent", api.UserAgent)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("Unexpected %d API response", response.StatusCode))
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		return nil, err
	}

	return contents, nil
}

func (api CronitorApi) Url() string {
	if api.IsDev {
		return "http://dev.cronitor.io/api/monitors"
	} else {
		return "https://cronitor.io/api/monitors"
	}
}

func (api CronitorApi) sendHttpPut(url string, body string) ([]byte, error) {
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	request, err := http.NewRequest("PUT", url, strings.NewReader(body))
	request.SetBasicAuth(viper.GetString(api.ApiKey), "")
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("User-Agent", api.UserAgent)
	request.Header.Add("Cronitor-Version", "2020-10-01")
	request.ContentLength = int64(len(body))
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		return nil, err
	}

	return contents, nil
}

func (api CronitorApi) sendHttpGet(url string) ([]byte, error) {
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	request, err := http.NewRequest("GET", url, nil)
	request.SetBasicAuth(viper.GetString(api.ApiKey), "")
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("User-Agent", api.UserAgent)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		return nil, err
	}

	return contents, nil
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

	client := &http.Client{Timeout: 120 * time.Second}
	request, err := http.NewRequest("POST", url, strings.NewReader(string(postBody)))
	if err != nil {
		return nil, errors.Wrap(err, "could not create request for URL presign")
	}
	request.SetBasicAuth(apiKey, "")
	request.Header.Add("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "error requesting presigned url")
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, fmt.Errorf("error response code %d returned", response.StatusCode)
	}

	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	response.Body = ioutil.NopCloser(bytes.NewBuffer(contents))
	return contents, nil
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
	response2, err := client.Do(req)
	if err != nil || response == nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error putting logs: %v", response2))
	}
	if response2.StatusCode < 200 || response2.StatusCode >= 300 {
		return nil, fmt.Errorf("error response code %d returned", response2.StatusCode)
	}
	body, err := ioutil.ReadAll(response2.Body)
	if err != nil {
		return nil, err
	}
	defer response2.Body.Close()
	//log(fmt.Sprintf("logs shipped for series %s", seriesID))
	return body, nil
}

package lib

type Rule struct {
	RuleType     string `json:"rule_type"`
	Value        string `json:"value"`
	TimeUnit     string `json:"time_unit,omitempty"`
	GraceSeconds uint   `json:"grace_seconds,omitempty"`
}

type Monitor struct {
	Name  			string   `json:"name,omitempty"`
	DefaultName		string   `json:"defaultName"`
	Key   			string   `json:"key"`
	Rules 			[]Rule   `json:"rules"`
	Tags  			[]string `json:"tags"`
	Type  			string   `json:"type"`
	Code			string   `json:"code,omitempty"`
	Timezone		string	 `json:"timezone,omitempty"`
	Note  			string   `json:"defaultNote,omitempty"`
	Notifications	map[string][]string `json:"notifications,omitempty"`
	NoStdoutPassthru bool	 `json:"-"`
}


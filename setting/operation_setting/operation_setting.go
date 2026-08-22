package operation_setting

import "strings"

var DemoSiteEnabled = false
var SelfUseModeEnabled = false

var AutomaticDisableKeywords = []string{
	"Your credit balance is too low",
	"This organization has been disabled.",
	"You exceeded your current quota",
	"Permission denied",
	"The security token included in the request is invalid",
	"Operation not allowed",
	"Your account is not authorized",
}

var RuntimeAutomaticDisableKeywords = append([]string(nil), AutomaticDisableKeywords...)

func AutomaticDisableKeywordsToString() string {
	return strings.Join(AutomaticDisableKeywords, "\n")
}

func AutomaticDisableKeywordsFromString(s string) {
	AutomaticDisableKeywords = []string{}
	ak := strings.Split(s, "\n")
	for _, k := range ak {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			AutomaticDisableKeywords = append(AutomaticDisableKeywords, k)
		}
	}
}

func RuntimeAutomaticDisableKeywordsToString() string {
	return strings.Join(RuntimeAutomaticDisableKeywords, "\n")
}

func RuntimeAutomaticDisableKeywordsFromString(s string) {
	RuntimeAutomaticDisableKeywords = []string{}
	for _, keyword := range strings.Split(s, "\n") {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" {
			RuntimeAutomaticDisableKeywords = append(RuntimeAutomaticDisableKeywords, keyword)
		}
	}
}

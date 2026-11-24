package auth

import "testing"

func TestDescribeDevice(t *testing.T) {
	for ua, want := range map[string]DeviceInfo{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36":                         {"桌機", "Windows", "Chrome 131"},
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0":               {"桌機", "Windows", "Edge 131"},
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1": {"手機", "iOS 18", "Safari 18.0"},
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:133.0) Gecko/20100101 Firefox/133.0":                                                    {"桌機", "macOS", "Firefox 133"},
		"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Mobile Safari/537.36":                                {"手機", "Android 14", "Chrome 131"},
		"Mozilla/5.0 (Linux; Android 14; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36":                              {"平板", "Android 14", "Chrome 131"},
		"curl/8.4.0": {},
	} {
		if got := DescribeDevice(ua); got != want {
			t.Errorf("%s\n got %+v, want %+v", ua, got, want)
		}
	}
}

func TestLocation(t *testing.T) {
	for c, want := range map[Client]string{
		{Country: "TW", City: "Taipei"}:                              "臺灣・臺北",
		{Country: "TW", City: "Taoyuan"}:                             "臺灣・桃園",
		{Country: "TW", City: "Zhongli District"}:                    "臺灣・桃園中壢",
		{Country: "TW", City: "New Taipei City"}:                     "臺灣・新北",
		{Country: "TW", City: "Somewhere", Region: "Hsinchu County"}: "臺灣・新竹",
		{Country: "TW", City: "Somewhere"}:                           "臺灣・Somewhere",
		{Country: "JP", Region: "Tokyo"}:                             "日本・Tokyo",
		{Country: "US"}:                                              "美國",
		{Country: "XX"}:                                              "",
		{}:                                                           "",
	} {
		if got := c.Location(); got != want {
			t.Errorf("%+v = %q, want %q", c, got, want)
		}
	}
}

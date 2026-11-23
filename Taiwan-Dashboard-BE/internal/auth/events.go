package auth

import (
	"strings"
	"time"
)

const (
	EventSignup    = "signup"
	EventNewDevice = "new_device"
	EventLink      = "link"
	EventUnlink    = "unlink"

	EventAPIKeyCreated  = "api_key_created"
	EventAPIKeyDisabled = "api_key_disabled"
	EventAPIKeyDeleted  = "api_key_deleted"
)

type Event struct {
	Kind          string
	UserID        int64
	Provider      string
	IP            string
	UserAgent     string
	At            time.Time
	Key, ClientID string
}

func (s *Store) emit(e Event) {
	if s.OnEvent == nil {
		return
	}
	if e.At.IsZero() {
		e.At = s.now()
	}
	s.OnEvent(e)
}

func DeviceName(ua string) string {
	os := ""
	switch {
	case strings.Contains(ua, "iPhone"):
		os = "iPhone"
	case strings.Contains(ua, "iPad"):
		os = "iPad"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "CrOS"):
		os = "ChromeOS"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	browser := ""
	switch {
	case strings.Contains(ua, "Edg"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"), strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "SamsungBrowser"):
		browser = "Samsung Internet"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	switch {
	case browser != "" && os != "":
		return browser + "／" + os
	case browser != "" || os != "":
		return browser + os
	case ua == "":
		return "未知的裝置"
	}
	return truncate(ua, 60)
}

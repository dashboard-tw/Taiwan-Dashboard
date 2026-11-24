package auth

import (
	"context"
	"regexp"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

type Client struct {
	UserAgent string
	Country   string
	Region    string
	City      string
}

type clientKey struct{}

func WithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientKey{}, c)
}

func ClientFrom(ctx context.Context) Client {
	c, _ := ctx.Value(clientKey{}).(Client)
	return c
}

var countryNames = display.Regions(language.TraditionalChinese)

func CountryName(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || code == "XX" || code == "T1" {
		return ""
	}
	if code == "TW" {
		return "臺灣"
	}
	r, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	if name := countryNames.Name(r); name != "" {
		return strings.ReplaceAll(name, "台", "臺")
	}
	return code
}

func (c Client) Location() string {
	parts := []string{}
	if name := CountryName(c.Country); name != "" {
		parts = append(parts, name)
	}
	place := c.City
	if strings.EqualFold(c.Country, "TW") {
		if zh := taiwanPlace(c.City); zh != "" {
			place = zh
		} else if zh := taiwanPlace(c.Region); zh != "" {
			place = zh
		}
	}
	if place == "" {
		place = c.Region
	}
	if place != "" {
		parts = append(parts, place)
	}
	return strings.Join(parts, "・")
}

type DeviceInfo struct {
	Type    string
	OS      string
	Browser string
}

var (
	reIOS     = regexp.MustCompile(`OS (\d+)[_.](\d+)`)
	reAndroid = regexp.MustCompile(`Android (\d+(?:\.\d+)?)`)
	reMacOS   = regexp.MustCompile(`Mac OS X (\d+)[_.](\d+)`)
	reBrowser = []struct {
		name string
		re   *regexp.Regexp
	}{
		{"Edge", regexp.MustCompile(`Edg(?:e|A|iOS)?/(\d+)`)},
		{"Opera", regexp.MustCompile(`(?:OPR|Opera)/(\d+)`)},
		{"Samsung Internet", regexp.MustCompile(`SamsungBrowser/(\d+)`)},
		{"Firefox", regexp.MustCompile(`(?:Firefox|FxiOS)/(\d+)`)},
		{"Chrome", regexp.MustCompile(`(?:Chrome|CriOS)/(\d+)`)},
		{"Safari", regexp.MustCompile(`Version/(\d+(?:\.\d+)?).*Safari/`)},
	}
)

func DescribeDevice(ua string) DeviceInfo {
	var d DeviceInfo
	switch {
	case strings.Contains(ua, "iPad"):
		d.Type, d.OS = "平板", "iPadOS"
		if m := reIOS.FindStringSubmatch(ua); m != nil {
			d.OS += " " + m[1]
		}
	case strings.Contains(ua, "iPhone"):
		d.Type, d.OS = "手機", "iOS"
		if m := reIOS.FindStringSubmatch(ua); m != nil {
			d.OS += " " + m[1]
		}
	case strings.Contains(ua, "Android"):
		d.Type, d.OS = "手機", "Android"
		if !strings.Contains(ua, "Mobile") {
			d.Type = "平板"
		}
		if m := reAndroid.FindStringSubmatch(ua); m != nil {
			d.OS += " " + m[1]
		}
	case strings.Contains(ua, "Windows"):
		d.Type, d.OS = "桌機", "Windows"
	case strings.Contains(ua, "CrOS"):
		d.Type, d.OS = "桌機", "ChromeOS"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		d.Type, d.OS = "桌機", "macOS"
		if m := reMacOS.FindStringSubmatch(ua); m != nil && m[1] != "10" {
			d.OS += " " + m[1]
		}
	case strings.Contains(ua, "Linux"):
		d.Type, d.OS = "桌機", "Linux"
	}
	for _, b := range reBrowser {
		if m := b.re.FindStringSubmatch(ua); m != nil {
			d.Browser = b.name + " " + m[1]
			break
		}
	}
	return d
}

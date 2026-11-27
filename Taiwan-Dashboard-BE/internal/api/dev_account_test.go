package api

import (
	"fmt"
	"net/http"
	"testing"

	"dashboard.tw/backend/internal/auth"
)

func TestDevAccount(t *testing.T) {
	e := newOpenEnv(t)
	withQuota(t, 2, 60)
	dev := e.signInAs(t, auth.RoleUser, "active")
	if _, err := e.db.Exec(`UPDATE app.users SET is_dev = true WHERE id = (SELECT max(id) FROM app.users)`); err != nil {
		t.Fatal(err)
	}

	// 不用填使用調查，也不受 3 組憑證的限制
	var clientID, secret string
	for i := 0; i < auth.MaxClients+1; i++ {
		w := e.send("POST", "/api/v1/me/api-clients", dev, map[string]any{"name": fmt.Sprintf("開發 %d", i+1)})
		if w.Code != http.StatusCreated {
			t.Fatalf("create client %d: %d %s", i+1, w.Code, w.Body)
		}
		got := decode[struct {
			Client auth.APIClient `json:"client"`
			Secret string         `json:"client_secret"`
		}](t, w)
		clientID, secret = got.Client.ClientID, got.Secret
	}

	// 不受每日次數限制，也沒有額度標頭
	w := e.token(clientID, secret, false)
	if w.Code != http.StatusOK {
		t.Fatalf("token: %d %s", w.Code, w.Body)
	}
	token := readToken(t, w).AccessToken
	for i := 0; i < 5; i++ {
		w := e.withKey("/api/v1/datasets", token)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: %d %s", i+1, w.Code, w.Body)
		}
		if w.Header().Get("X-RateLimit-Limit") != "" {
			t.Errorf("dev accounts should not get a rate-limit header")
		}
	}

	// 自己看得到實際用量
	state := decode[struct {
		Dev       bool       `json:"dev"`
		Tier      string     `json:"tier"`
		UsedToday auth.Usage `json:"used_today"`
	}](t, e.send("GET", "/api/v1/me/api-clients", dev, nil))
	if !state.Dev || state.Tier != auth.TierDev || state.UsedToday.Requests != 5 {
		t.Errorf("own view = %+v", state)
	}

	// 後台統計不算這個帳號
	admin := e.signInAs(t, auth.RoleAdmin, "active")
	overview := decode[struct {
		Users struct {
			Total int `json:"total"`
		} `json:"users"`
		API struct {
			Clients       int `json:"clients"`
			Accounts      int `json:"accounts"`
			RequestsToday int `json:"requests_today"`
		} `json:"api"`
	}](t, e.send("GET", "/api/v1/admin/overview", admin, nil))
	if overview.Users.Total != 1 || overview.API.Clients != 0 || overview.API.Accounts != 0 || overview.API.RequestsToday != 0 {
		t.Errorf("overview = %+v", overview)
	}
	usage := decode[struct {
		Totals struct {
			Requests int `json:"requests"`
		} `json:"totals"`
		Datasets []struct {
			Dataset string `json:"dataset"`
		} `json:"datasets"`
		Top []usageAccount `json:"top"`
	}](t, e.send("GET", "/api/v1/admin/api-usage?days=7", admin, nil))
	if usage.Totals.Requests != 0 || len(usage.Datasets) != 0 || len(usage.Top) != 0 {
		t.Errorf("api usage = %+v", usage)
	}
}

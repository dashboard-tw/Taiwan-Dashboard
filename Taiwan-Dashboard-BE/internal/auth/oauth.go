package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Identity struct {
	Subject   string
	Name      string
	Email     string
	CreatedAt time.Time
}

type OAuthProvider struct {
	Provider
	ClientID, ClientSecret string
	AuthURL, TokenURL      string
	Scopes                 []string
	ExtraParams            url.Values
	UserURL, EmailsURL     string
	MinAccountAge          time.Duration
	fetch                  func(ctx context.Context, c *http.Client, p *OAuthProvider, token string) (Identity, error)
}

var ErrAccountTooNew = errors.New("provider account too new")

const GitHubMinAccountAge = 30 * 24 * time.Hour

func GitHub(clientID, clientSecret string) *OAuthProvider {
	return &OAuthProvider{
		Provider: Provider{ID: "github", Label: "使用 GitHub 登入"},
		ClientID: clientID, ClientSecret: clientSecret,
		AuthURL:       "https://github.com/login/oauth/authorize",
		TokenURL:      "https://github.com/login/oauth/access_token",
		Scopes:        []string{"read:user", "user:email"},
		UserURL:       "https://api.github.com/user",
		EmailsURL:     "https://api.github.com/user/emails",
		ExtraParams:   url.Values{"allow_signup": {"false"}},
		MinAccountAge: GitHubMinAccountAge,
		fetch:         fetchGitHub,
	}
}

func Google(clientID, clientSecret string) *OAuthProvider {
	return &OAuthProvider{
		Provider: Provider{ID: "google", Label: "使用 Google 登入"},
		ClientID: clientID, ClientSecret: clientSecret,
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    "https://oauth2.googleapis.com/token",
		Scopes:      []string{"openid", "email", "profile"},
		UserURL:     "https://openidconnect.googleapis.com/v1/userinfo",
		ExtraParams: url.Values{"prompt": {"select_account"}},
		fetch:       fetchGoogle,
	}
}

func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (p *OAuthProvider) AuthCodeURL(state, verifier, redirectURI string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(p.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {PKCEChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	for k, v := range p.ExtraParams {
		q[k] = v
	}
	return p.AuthURL + "?" + q.Encode()
}

func (p *OAuthProvider) SignIn(ctx context.Context, c *http.Client, code, verifier, redirectURI string, now time.Time) (Identity, error) {
	token, err := p.exchange(ctx, c, code, verifier, redirectURI)
	if err != nil {
		return Identity{}, err
	}
	id, err := p.fetch(ctx, c, p, token)
	if err != nil {
		return Identity{}, err
	}
	if id.Subject == "" {
		return Identity{}, errors.New("provider returned no user id")
	}
	if p.MinAccountAge > 0 && (id.CreatedAt.IsZero() || now.Sub(id.CreatedAt) < p.MinAccountAge) {
		return Identity{}, ErrAccountTooNew
	}
	id.Name = truncate(strings.TrimSpace(id.Name), 80)
	id.Email = truncate(strings.TrimSpace(id.Email), 200)
	return id, nil
}

func (p *OAuthProvider) exchange(ctx context.Context, c *http.Client, code, verifier, redirectURI string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := doJSON(c, req, &body); err != nil {
		return "", fmt.Errorf("%s token: %w", p.ID, err)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("%s token: no access token (%s)", p.ID, truncate(body.Error, 60))
	}
	return body.AccessToken, nil
}

func fetchGitHub(ctx context.Context, c *http.Client, p *OAuthProvider, token string) (Identity, error) {
	var user struct {
		ID        int64     `json:"id"`
		Login     string    `json:"login"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := getWithToken(ctx, c, p.UserURL, token, &user); err != nil {
		return Identity{}, fmt.Errorf("github user: %w", err)
	}
	if user.ID == 0 {
		return Identity{}, errors.New("github user: no id")
	}
	name := user.Name
	if name == "" {
		name = user.Login
	}
	id := Identity{Subject: strconv.FormatInt(user.ID, 10), Name: name, CreatedAt: user.CreatedAt}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := getWithToken(ctx, c, p.EmailsURL, token, &emails); err == nil {
		for _, e := range emails {
			if e.Primary && e.Verified {
				id.Email = e.Email
			}
		}
	}
	return id, nil
}

func fetchGoogle(ctx context.Context, c *http.Client, p *OAuthProvider, token string) (Identity, error) {
	var info struct {
		Sub           string `json:"sub"`
		Name          string `json:"name"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := getWithToken(ctx, c, p.UserURL, token, &info); err != nil {
		return Identity{}, fmt.Errorf("google userinfo: %w", err)
	}
	id := Identity{Subject: info.Sub, Name: info.Name}
	if info.EmailVerified {
		id.Email = info.Email
	}
	if id.Name == "" {
		id.Name = "Google 使用者"
	}
	return id, nil
}

func getWithToken(ctx context.Context, c *http.Client, u, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dashboard.tw")
	return doJSON(c, req, out)
}

func doJSON(c *http.Client, req *http.Request, out any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	return nil
}

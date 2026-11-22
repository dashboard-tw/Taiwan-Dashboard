package auth

type Provider struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Dev   bool   `json:"dev"`
}

var DevProvider = Provider{ID: "dev", Label: "開發用登入（管理員）", Dev: true}

var DevUserProvider = Provider{ID: "dev-user", Label: "開發用登入（一般使用者）", Dev: true}

func Providers(devLogin, production bool, oauth ...*OAuthProvider) []Provider {
	var out []Provider
	for _, p := range oauth {
		out = append(out, p.Provider)
	}
	if devLogin && !production {
		out = append(out, DevProvider, DevUserProvider)
	}
	return out
}

func ConfiguredOAuth(githubID, githubSecret, googleID, googleSecret string) []*OAuthProvider {
	var out []*OAuthProvider
	if githubID != "" && githubSecret != "" {
		out = append(out, GitHub(githubID, githubSecret))
	}
	if googleID != "" && googleSecret != "" {
		out = append(out, Google(googleID, googleSecret))
	}
	return out
}

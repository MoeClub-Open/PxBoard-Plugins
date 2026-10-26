package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"golang.org/x/oauth2"
)

type identity struct {
	Subject, DisplayName, Email string
	EmailVerified               bool
}

func providerConfig(row p.Record) *oauth2.Config {
	config := &oauth2.Config{ClientID: p.Text(row, "clientId"), ClientSecret: p.Text(row, "clientSecret"), RedirectURL: callbackURL(row)}
	config.Endpoint.AuthStyle = oauth2.AuthStyleInParams
	switch p.Text(row, "kind") {
	case "google":
		config.Endpoint.AuthURL, config.Endpoint.TokenURL = "https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token"
		config.Scopes = []string{"openid", "email", "profile"}
	case "github":
		config.Endpoint.AuthURL, config.Endpoint.TokenURL = "https://github.com/login/oauth/authorize", "https://github.com/login/oauth/access_token"
		config.Scopes = []string{"read:user", "user:email"}
	case "linuxdo":
		config.Endpoint.AuthURL, config.Endpoint.TokenURL = "https://connect.linux.do/oauth2/authorize", "https://connect.linux.do/oauth2/token"
		config.Endpoint.AuthStyle = oauth2.AuthStyleInHeader
		config.Scopes = []string{"openid", "email", "profile"}
	}
	return config
}

func readProfile(ctx context.Context, client *http.Client, endpoint string, out any) error {
	r, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if e != nil {
		return e
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("User-Agent", "PxBoard-OAuth")
	response, e := client.Do(r)
	if e != nil {
		return fmt.Errorf("无法连接登录服务，请稍后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("登录服务暂时无法提供账号信息")
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	decoder.UseNumber()
	if e = decoder.Decode(out); e != nil {
		return fmt.Errorf("登录服务返回的账号信息无效")
	}
	return nil
}

func profileID(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	}
	return ""
}

func fetchIdentity(ctx context.Context, config *oauth2.Config, kind, code, verifier string) (identity, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 20 * time.Second})
	token, e := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if e != nil {
		return identity{}, fmt.Errorf("授权验证失败，请重新登录")
	}
	client := config.Client(ctx, token)
	endpoint := map[string]string{"google": "https://openidconnect.googleapis.com/v1/userinfo", "github": "https://api.github.com/user", "linuxdo": "https://connect.linux.do/api/user"}[kind]
	var profile p.Record
	if e = readProfile(ctx, client, endpoint, &profile); e != nil {
		return identity{}, e
	}
	result := identity{Subject: profileID(profile["id"]), DisplayName: p.Text(profile, "name"), Email: strings.ToLower(strings.TrimSpace(p.Text(profile, "email"))), EmailVerified: p.Bool(profile, "email_verified")}
	if kind == "google" {
		result.Subject = profileID(profile["sub"])
	}
	if kind == "linuxdo" && result.Subject == "" {
		result.Subject = profileID(profile["sub"])
	}
	if result.DisplayName == "" {
		result.DisplayName = p.Text(profile, "login")
	}
	if result.DisplayName == "" {
		result.DisplayName = p.Text(profile, "username")
	}
	if kind == "github" {
		var emails []p.Record
		result.Email, result.EmailVerified = "", false
		_ = readProfile(ctx, client, "https://api.github.com/user/emails", &emails)
		for _, email := range emails {
			if p.Bool(email, "verified") && (result.Email == "" || p.Bool(email, "primary")) {
				result.Email = strings.ToLower(strings.TrimSpace(p.Text(email, "email")))
				result.EmailVerified = true
				if p.Bool(email, "primary") {
					break
				}
			}
		}
	}
	if result.Subject == "" {
		return identity{}, fmt.Errorf("登录服务没有返回稳定的账号标识")
	}
	return result, nil
}

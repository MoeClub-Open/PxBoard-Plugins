package captcha

import (
	"encoding/json"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var verificationHTTP = &http.Client{Timeout: 12 * time.Second}
var endpoints = map[string]string{
	"turnstile": "https://challenges.cloudflare.com/turnstile/v0/siteverify",
	"hcaptcha":  "https://api.hcaptcha.com/siteverify",
	"recaptcha": "https://www.recaptcha.net/recaptcha/api/siteverify",
}

func (a *app) verify(w http.ResponseWriter, r *http.Request) {
	if !p.Service(r, "core") {
		p.Fail(w, 403, "无权验证")
		return
	}
	var b struct{ Action, Token string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	if b.Action == "subscription" && !p.Bool(config, "enabled") {
		p.Fail(w, 503, "人机验证未启用")
		return
	}
	required := p.Bool(config, "enabled") && (b.Action == "subscription" || p.Bool(config, b.Action) || b.Action == "oauth" && (p.Bool(config, "login") || p.Bool(config, "register")))
	if !required {
		p.JSON(w, true)
		return
	}
	if b.Token == "" {
		p.Fail(w, 400, "请完成人机验证")
		return
	}
	provider := p.Text(config, "provider")
	endpoint := endpoints[provider]
	if endpoint == "" || p.Text(config, "secretKey") == "" || p.Text(config, "siteKey") == "" {
		p.Fail(w, 503, "人机验证配置不完整")
		return
	}
	data := url.Values{"secret": {p.Text(config, "secretKey")}, "response": {b.Token}}
	if provider == "hcaptcha" {
		data.Set("sitekey", p.Text(config, "siteKey"))
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		p.Fail(w, 503, "验证服务暂不可用")
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := verificationHTTP.Do(req)
	if err != nil {
		p.Fail(w, 503, "验证服务暂不可用")
		return
	}
	defer res.Body.Close()
	var result struct {
		Success bool `json:"success"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&result) != nil {
		p.Fail(w, 503, "验证服务暂不可用")
		return
	}
	if !result.Success {
		p.Fail(w, 400, "人机验证未通过")
		return
	}
	p.JSON(w, true)
}

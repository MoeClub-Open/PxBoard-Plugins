package subscriptions

import (
	"net/http"
	"regexp"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

var clientProduct = regexp.MustCompile(`(?i)^([a-z][a-z0-9._+-]*)(?:[/\s]+v?([0-9]+))?`)

func browserRequest(r *http.Request) bool {
	ua := strings.ToLower(strings.TrimSpace(r.UserAgent()))
	return strings.HasPrefix(ua, "mozilla/") || strings.EqualFold(r.Header.Get("Sec-Fetch-Mode"), "navigate") || strings.EqualFold(r.Header.Get("Sec-Fetch-Dest"), "document")
}

func clientIdentity(r *http.Request) (string, string) {
	match := clientProduct.FindStringSubmatch(strings.TrimSpace(r.UserAgent()))
	if len(match) == 0 {
		return "", ""
	}
	return strings.ToLower(match[1]), match[2]
}

func disguise(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<!doctype html><html><head><title>404 Not Found</title></head><body><h1>404 Not Found</h1></body></html>"))
}

func (a *app) checkClient(w http.ResponseWriter, r *http.Request, link p.Record) (string, string, bool) {
	if !p.Bool(link, "bindUserAgent") {
		return "", "", true
	}
	if browserRequest(r) {
		disguise(w)
		return "", "", false
	}
	name, major := clientIdentity(r)
	if name == "" {
		p.Fail(w, 403, "无法识别客户端，请使用客户端更新订阅")
		return "", "", false
	}
	if bound := p.Text(link, "boundClient"); bound != "" && (bound != name || p.Text(link, "boundMajor") != "" && p.Text(link, "boundMajor") != major) {
		p.Fail(w, 403, "此订阅已绑定其他客户端，请创建新的订阅链接")
		return "", "", false
	}
	return name, major, true
}

func (a *app) bindClient(w http.ResponseWriter, link p.Record, name, major string) bool {
	if !p.Bool(link, "bindUserAgent") || p.Text(link, "boundClient") != "" {
		return true
	}
	link["boundClient"], link["boundMajor"], link["boundAt"] = name, major, c.Now()
	return !c.Err(w, a.C.Put("links", link))
}

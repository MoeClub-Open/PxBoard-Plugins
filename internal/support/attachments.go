package support

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const maxImageSize = 2 << 20

var attachmentPattern = regexp.MustCompile(`/api/v1/extensions/support/attachments/([a-f0-9]{32})`)
var attachmentID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (a *app) attachment(id string) (p.Record, error) {
	var result struct {
		Items []p.Record `json:"items"`
	}
	err := a.C.Request("POST", "/api/v1/runtime/query", p.Record{"collection": "attachments", "filters": []p.Record{{"field": "id", "value": id}}, "limit": 1}, &result)
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, fmt.Errorf("图片不存在")
	}
	return result.Items[0], nil
}
func messageAttachmentIDs(ticket p.Record, id string) bool {
	messages, _ := ticket["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		ids, _ := message["attachmentIds"].([]any)
		for _, item := range ids {
			if item == id {
				return true
			}
		}
	}
	return false
}
func (a *app) validateAttachments(r *http.Request, content string, ticket p.Record) ([]string, error) {
	ids := []string{}
	seen := map[string]bool{}
	for _, match := range attachmentPattern.FindAllStringSubmatch(content, -1) {
		id := match[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		row, err := a.attachment(id)
		if err != nil {
			return nil, err
		}
		if !p.Admin(r) && p.Text(row, "userId") != p.Subject(r).ID && !messageAttachmentIDs(ticket, id) {
			return nil, fmt.Errorf("无权使用此图片")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
func (a *app) upload(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 3<<20)).Decode(&body); err != nil {
		p.Fail(w, 400, "图片格式无效")
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil || len(data) == 0 || len(data) > maxImageSize {
		p.Fail(w, 400, "图片大小不能超过 2 MB")
		return
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/gif" && mime != "image/webp" {
		p.Fail(w, 400, "请选择 PNG、JPEG、GIF 或 WebP 图片")
		return
	}
	root := filepath.Join(os.Getenv("PX_PLUGIN_STATE_DIR"), "attachments")
	if err = os.MkdirAll(root, 0700); err != nil {
		p.Fail(w, 500, "图片保存失败")
		return
	}
	id := p.ID()
	path := filepath.Join(root, id)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		p.Fail(w, 500, "图片保存失败")
		return
	}
	_, err = file.Write(data)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		p.Fail(w, 500, "图片保存失败")
		return
	}
	row := p.Record{"id": id, "userId": p.Subject(r).ID, "contentType": mime, "size": len(data), "createdAt": c.Now()}
	if err = a.C.Put("attachments", row); err != nil {
		_ = os.Remove(path)
		p.Fail(w, 500, "图片保存失败")
		return
	}
	p.JSON(w, p.Record{"id": id, "url": "/api/v1/extensions/support/attachments/" + id})
}
func (a *app) download(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	id := r.PathValue("id")
	if !attachmentID.MatchString(id) {
		p.Fail(w, 404, "图片不存在")
		return
	}
	row, err := a.attachment(id)
	if err != nil {
		p.Fail(w, 404, "图片不存在")
		return
	}
	allowed := p.Admin(r) || p.Text(row, "userId") == p.Subject(r).ID
	if !allowed {
		tickets, err := a.Filter("tickets", func(ticket p.Record) bool { return p.Text(ticket, "userId") == p.Subject(r).ID })
		if c.Err(w, err) {
			return
		}
		for _, ticket := range tickets {
			if messageAttachmentIDs(ticket, id) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		p.Fail(w, 403, "无权查看此图片")
		return
	}
	path := filepath.Join(os.Getenv("PX_PLUGIN_STATE_DIR"), "attachments", id)
	data, err := os.ReadFile(path)
	if err != nil {
		p.Fail(w, 404, "图片不存在")
		return
	}
	w.Header().Set("Content-Type", p.Text(row, "contentType"))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
func message(r *http.Request, content string, ids []string) p.Record {
	return p.Record{"id": p.ID(), "userId": p.Subject(r).ID, "author": p.Subject(r).Email, "admin": p.Admin(r), "createdAt": c.Now(), "content": strings.TrimSpace(content), "attachmentIds": ids}
}

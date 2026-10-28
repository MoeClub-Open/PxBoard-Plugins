package subscriptionconfig

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed defaults/*
var defaults embed.FS

const Nodes = "${nodes}"

func Default(format string) string {
	file := "mihomo.yaml"
	if format == "sing-box" {
		file = "sing-box.json"
	}
	content, _ := defaults.ReadFile("defaults/" + file)
	return string(content)
}

func parse(format, content string) (map[string]any, error) {
	var config map[string]any
	if format == "sing-box" {
		decoder := json.NewDecoder(strings.NewReader(content))
		if err := decoder.Decode(&config); err != nil {
			return nil, fmt.Errorf("JSON 格式错误：%w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("模板只能包含一个 JSON 对象")
		}
	} else if format == "clash" {
		decoder := yaml.NewDecoder(strings.NewReader(content))
		if err := decoder.Decode(&config); err != nil {
			return nil, fmt.Errorf("YAML 格式错误：%w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("模板只能包含一个 YAML 文档")
		}
	} else {
		return nil, fmt.Errorf("模板仅支持 Mihomo 和 sing-box")
	}
	if config == nil {
		return nil, fmt.Errorf("模板内容不能为空")
	}
	return config, nil
}

func Encode(format string, config map[string]any) ([]byte, error) {
	if format == "clash" {
		return yaml.Marshal(config)
	}
	return json.MarshalIndent(config, "", "  ")
}

func objects(value any) ([]map[string]any, error) {
	if value == nil {
		return []map[string]any{}, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("配置列表格式无效")
	}
	result := []map[string]any{}
	for _, value := range list {
		row, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("配置列表必须包含对象")
		}
		result = append(result, row)
	}
	return result, nil
}
func text(row map[string]any, key string) string { value, _ := row[key].(string); return value }
func object(row map[string]any, key string) map[string]any {
	value, _ := row[key].(map[string]any)
	return value
}
func stringsIn(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("名称列表格式无效")
	}
	result := []string{}
	for _, raw := range list {
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("名称必须为字符串")
		}
		result = append(result, value)
	}
	return result, nil
}

func PreviewNodes(format string) []map[string]any {
	if format == "clash" {
		return []map[string]any{{"name": "香港 · 示例", "type": "trojan", "server": "hk.example.com", "port": 443, "password": "preview-only", "sni": "hk.example.com"}, {"name": "日本 · 示例", "type": "trojan", "server": "jp.example.com", "port": 443, "password": "preview-only", "sni": "jp.example.com"}}
	}
	return []map[string]any{{"tag": "香港 · 示例", "type": "trojan", "server": "hk.example.com", "server_port": 443, "password": "preview-only", "tls": map[string]any{"enabled": true, "server_name": "hk.example.com"}}, {"tag": "日本 · 示例", "type": "trojan", "server": "jp.example.com", "server_port": 443, "password": "preview-only", "tls": map[string]any{"enabled": true, "server_name": "jp.example.com"}}}
}

func Check(format, content string) error {
	_, err := Render(format, content, PreviewNodes(format), "PxBoard", nil)
	return err
}

func replaceTitle(value any, title string) any {
	switch node := value.(type) {
	case string:
		return strings.ReplaceAll(node, "${siteName}", title)
	case []any:
		for i, v := range node {
			node[i] = replaceTitle(v, title)
		}
	case map[string]any:
		for key, v := range node {
			node[key] = replaceTitle(v, title)
		}
	}
	return value
}

package migration

import (
	"encoding/json"
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"maps"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var sourcePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Options struct {
	IncludeNodes      bool   `json:"includeNodes"`
	PasswordMethod    string `json:"passwordMethod"`
	PasswordSalt      string `json:"passwordSalt"`
	SystemResetMethod string `json:"systemResetMethod"`
	Seed              string `json:"-"`
}
type Plan struct {
	SourceID    string         `json:"sourceId"`
	SourceRows  int            `json:"sourceRows"`
	ArchiveRows int            `json:"archiveRows"`
	Mapped      map[string]int `json:"mapped"`
	ArchiveOnly map[string]int `json:"archiveOnly"`
	Warnings    []string       `json:"warnings"`
	Errors      []string       `json:"errors"`
	Rows        []p.ImportRow  `json:"-"`
}
type mapper struct {
	S           *Snapshot
	Options     Options
	Plan        *Plan
	Users       map[string]bool
	Machines    map[string]bool
	MFA         map[string]string
	NodeCounter int64
	UsedTables  map[string]bool
}

func Build(snapshot *Snapshot, pluginID string, options Options) (*Plan, error) {
	if !sourcePattern.MatchString(snapshot.SourceID) {
		return nil, fmt.Errorf("来源标识无效")
	}
	if _, e := time.LoadLocation(snapshot.Timezone); e != nil {
		return nil, e
	}
	plan := &Plan{SourceID: snapshot.SourceID, Mapped: map[string]int{}, ArchiveOnly: map[string]int{}, Warnings: []string{}, Errors: []string{}, Rows: []p.ImportRow{}}
	m := &mapper{S: snapshot, Options: options, Plan: plan, Users: map[string]bool{}, Machines: map[string]bool{}, MFA: map[string]string{}, UsedTables: map[string]bool{}}
	m.checkPendingOrders()
	if snapshot.Source == "sspanel" {
		m.ssUsers()
		m.ssPlans()
		if options.IncludeNodes {
			m.ssNodes()
		}
	} else if snapshot.Source == "v2board" || snapshot.Source == "xboard" {
		m.v2Users()
		m.v2Plans()
		if options.IncludeNodes {
			m.v2Nodes()
		}
	} else {
		return nil, fmt.Errorf("不支持的来源类型：%s", snapshot.Source)
	}
	m.warning("历史订单、支付、优惠、审计和旧系统设置仅保存原始记录，不自动启用；用户余额和套餐权益单独转换。订阅令牌保持不变，订阅地址需使用 PxBoard 新接口。")
	for _, name := range snapshot.Names() {
		table := snapshot.Tables[name]
		plan.SourceRows += len(table.Rows)
		if !m.UsedTables[name] {
			plan.ArchiveOnly[name] = len(table.Rows)
		}
		for index, row := range table.Rows {
			m.doc(pluginID, "archive", m.id("archive", name+":"+strconv.Itoa(index)), p.Record{"runSource": snapshot.SourceID, "table": name, "row": row, "rowIndex": index})
			plan.ArchiveRows++
		}
		m.doc(pluginID, "schemas", m.id("schema", name), p.Record{"sourceId": snapshot.SourceID, "table": name, "columns": table.Columns})
	}
	return plan, nil
}

func (m *mapper) checkPendingOrders() {
	processing, pending := 0, 0
	if m.S.Source == "sspanel" {
		for _, order := range m.S.Rows("order") {
			switch text(order, "status") {
			case "pending_activation":
				processing++
			case "pending_payment":
				pending++
			}
		}
	} else {
		for _, order := range m.S.Rows("v2_order") {
			switch integer(order, "status") {
			case 1:
				processing++
			case 0:
				pending++
			}
		}
	}
	if processing > 0 {
		m.fail("源站有 %d 笔订单正在开通，现有账户权益尚未稳定；请在源站处理完毕后重新导出", processing)
	}
	if pending > 0 {
		m.warning("源站有 %d 笔待付款订单，仅保留历史原记录，不迁移为可继续付款的 PxBoard 订单", pending)
	}
}
func (m *mapper) id(kind, id string) string { return m.S.SourceID + ":" + kind + ":" + id }
func (m *mapper) ref(row Row, table string) p.Record {
	return p.Record{"system": m.S.Source, "sourceId": m.S.SourceID, "table": table, "recordId": text(row, "id")}
}
func (m *mapper) doc(namespace, collection, id string, data p.Record) {
	data["id"] = id
	raw, e := json.Marshal(data)
	if e != nil {
		m.fail("%s/%s/%s：%v", namespace, collection, id, e)
		return
	}
	m.Plan.Rows = append(m.Plan.Rows, p.ImportRow{Kind: "document", Namespace: namespace, Collection: collection, ID: id, Data: raw})
	if collection != "archive" && collection != "schemas" {
		m.Plan.Mapped[namespace+"/"+collection]++
	}
}
func (m *mapper) fail(format string, args ...any) {
	m.Plan.Errors = append(m.Plan.Errors, fmt.Sprintf(format, args...))
}
func (m *mapper) warning(format string, args ...any) {
	m.Plan.Warnings = append(m.Plan.Warnings, fmt.Sprintf(format, args...))
}
func (m *mapper) table(names ...string) (string, []Row) {
	for _, name := range names {
		if t, ok := m.S.Tables[name]; ok {
			m.UsedTables[name] = true
			return name, t.Rows
		}
	}
	return names[0], nil
}
func text(row Row, key string) string {
	switch v := row[key].(type) {
	case string:
		return v
	case json.Number:
		return string(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return ""
	}
}
func integer(row Row, key string) int64  { v, _ := strconv.ParseInt(text(row, key), 10, 64); return v }
func number(row Row, key string) float64 { v, _ := strconv.ParseFloat(text(row, key), 64); return v }
func truth(row Row, key string) bool {
	return row[key] == true || text(row, key) == "1" || text(row, key) == "true"
}
func object(row Row, key string) Row {
	if value, ok := row[key].(map[string]any); ok {
		return maps.Clone(Row(value))
	}
	if value, ok := row[key].(Row); ok {
		return maps.Clone(value)
	}
	raw := text(row, key)
	if raw == "" || raw == "null" {
		return Row{}
	}
	var out Row
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&out) != nil {
		return Row{}
	}
	return out
}
func (m *mapper) timeValue(row Row, key string) string {
	raw := text(row, key)
	if raw == "" || raw == "0" || strings.HasPrefix(raw, "0000-") {
		return ""
	}
	if value, e := strconv.ParseInt(raw, 10, 64); e == nil {
		if value > 1000000000000 {
			return time.UnixMilli(value).UTC().Format(time.RFC3339)
		}
		return time.Unix(value, 0).UTC().Format(time.RFC3339)
	}
	if value, e := time.Parse(time.RFC3339, raw); e == nil {
		return value.UTC().Format(time.RFC3339)
	}
	zone, _ := time.LoadLocation(m.S.Timezone)
	for _, format := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", "2006-01-02"} {
		if value, e := time.ParseInLocation(format, raw, zone); e == nil {
			return value.UTC().Format(time.RFC3339)
		}
	}
	m.fail("无法解析时间 %s=%q", key, raw)
	return ""
}
func (m *mapper) created(row Row, keys ...string) string {
	for _, key := range keys {
		if result := m.timeValue(row, key); result != "" {
			return result
		}
	}
	return m.S.ExportedAt
}
func (m *mapper) money(row Row, key string, yuan bool) int64 {
	raw := text(row, key)
	if raw == "" {
		if row[key] != nil {
			m.fail("金额字段无法识别：%s", key)
		}
		return 0
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		m.fail("金额无效：%s=%q", key, raw)
		return 0
	}
	if yuan {
		value.Mul(value, big.NewRat(100, 1))
	}
	if !value.IsInt() || !value.Num().IsInt64() {
		m.fail("金额无法无损转换为整数分：%s=%q", key, raw)
		return 0
	}
	return value.Num().Int64()
}
func (m *mapper) userID(row Row, key string) string {
	id := text(row, key)
	if id == "" || id == "0" {
		return ""
	}
	if !m.Users[id] {
		m.warning("旧记录引用的用户已不存在，关系仅保留在原始数据中：%s=%s", key, id)
		return ""
	}
	return m.id("user", id)
}
func (m *mapper) group(id string) string { return m.S.SourceID + ":group:" + id }
func groupIDs(value any) []string {
	raw := Row{"v": value}
	textValue := text(raw, "v")
	var list []any
	if values, ok := value.([]any); ok {
		list = values
	} else if json.Unmarshal([]byte(textValue), &list) != nil {
		if textValue == "" || textValue == "null" {
			return nil
		}
		return strings.Split(textValue, ",")
	}
	out := []string{}
	for _, v := range list {
		out = append(out, text(Row{"v": v}, "v"))
	}
	return out
}

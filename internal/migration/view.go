package migration

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"sort"
)

func sortedKeys(values map[string]int) []string {
	out := []string{}
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可执行数据迁移")
	}
	title := map[string]string{"sspanel": "SSPanel", "v2board": "V2Board", "xboard": "Xboard"}[a.Source]
	fields := []p.Field{
		c.Field("name", "任务名称", "text", true),
		{Key: "sourceType", Label: "数据来源", Type: "select", Default: "snapshot", Required: true, Options: []p.Option{{Label: "快照文件", Value: "snapshot"}, {Label: "MySQL 数据库", Value: "database"}}},
		{Key: "sourcePath", Label: "快照文件路径", Type: "text", Required: true, Hint: "填写面板服务器上快照文件的绝对路径", When: map[string][]string{"sourceType": {"snapshot"}}},
		{Key: "dsn", Label: "只读 MySQL 连接", Type: "password", Required: true, When: map[string][]string{"sourceType": {"database"}}},
		{Key: "sourceId", Label: "来源标识", Type: "text", Required: true, Hint: "使用小写字母、数字、下划线或连字符，为每个源站设置不同标识", When: map[string][]string{"sourceType": {"database"}}},
		{Key: "timezone", Label: "源数据库时区", Type: "text", Default: "Asia/Shanghai"},
		{Key: "passwordMethod", Label: "旧密码算法", Type: "select", Default: "auto", Options: []p.Option{{Label: "自动识别编码密码 / 使用源字段", Value: "auto"}, {Label: "bcrypt", Value: "bcrypt"}, {Label: "Argon2id", Value: "argon2id"}, {Label: "Argon2i", Value: "argon2i"}, {Label: "MD5", Value: "md5"}, {Label: "MD5 + 盐", Value: "md5salt"}, {Label: "SHA256", Value: "sha256"}, {Label: "SHA256 + 盐", Value: "sha256salt"}, {Label: "SHA3-256 + 盐", Value: "sha3salt"}}},
		{Key: "passwordSalt", Label: "旧密码盐（如适用）", Type: "password", When: map[string][]string{"passwordMethod": {"auto", "md5salt", "sha256", "sha256salt", "sha3salt"}}},
	}
	fields = append(fields, p.Field{Key: "includeNodes", Label: "同时导入节点和机器", Type: "boolean", Default: false})
	fields = append(fields, p.Field{Key: "includeAffiliate", Label: "同时导入推广关系与佣金（需启用推广插件）", Type: "boolean", Default: false})
	if a.Source != "sspanel" {
		fields = append(fields, p.Field{Key: "includeTemplates", Label: "导入订阅模板", Type: "boolean", Default: true})
		fields = append(fields, p.Field{Key: "includeRoutes", Label: "导入后台入口和订阅地址", Type: "boolean", Default: true, Hint: "已有不同配置时会阻止导入；订阅域名需要指向新面板"})
		fields = append(fields, p.Field{Key: "systemResetMethod", Label: "源面板系统流量重置方式", Type: "select", Default: "auto", Options: []p.Option{{Label: "套餐各自指定（不跟随系统）", Value: "auto"}, {Label: "每月 1 日", Value: "0"}, {Label: "到期日按月重置", Value: "1"}, {Label: "不重置", Value: "2"}, {Label: "每年 1 月 1 日", Value: "3"}, {Label: "到期日按年重置", Value: "4"}}})
	}
	return p.View{Title: "从 " + title + " 导入", Resources: []p.Resource{{Key: "jobs", Title: "迁移任务", Endpoint: "/jobs", CreateLabel: "创建任务", Fields: fields, PollIntervalSeconds: 5, Columns: []p.Field{c.Field("name", "任务", "text", false), c.Field("sourceId", "来源标识", "text", false), c.Field("status", "状态", "status", false), c.Field("sourceRows", "原始记录", "number", false), c.Field("archiveRows", "归档记录", "number", false), c.Field("mappedRows", "转换记录", "number", false), c.Field("warningCount", "需核对", "number", false), c.Field("errorCount", "阻断", "number", false), c.Field("updatedAt", "更新时间", "datetime", false)}, Actions: []p.Action{{Key: "preview", Label: "预览", When: map[string][]string{"status": {"created", "failed", "blocked", "preparing", "importing"}}}, {Key: "report", Label: "迁移报告"}, {Key: "apply", Label: "提交导入", Confirm: "将导入报告中的用户与套餐等记录。请确认已核对报告，并已备份目标数据库；遇到冲突会整体回滚。", When: map[string][]string{"status": {"ready"}}}}}}}, nil
}

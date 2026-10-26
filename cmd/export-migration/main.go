package main

import (
	"context"
	"flag"
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/migration"
	"log"
	"os"
)

func main() {
	source := flag.String("source", "", "sspanel、v2board 或 xboard")
	id := flag.String("source-id", "", "稳定且唯一的来源标识，如 old-production")
	zone := flag.String("timezone", "Asia/Shanghai", "源数据库 datetime 字段的时区")
	out := flag.String("out", "migration.json", "输出文件（不覆盖已有文件）")
	flag.Parse()
	switch *source {
	case "sspanel", "v2board", "xboard":
	default:
		log.Fatal("--source 应为 sspanel、v2board 或 xboard")
	}
	dsn := os.Getenv("PX_MIGRATION_DSN")
	if dsn == "" {
		log.Fatal("请使用 PX_MIGRATION_DSN 环境变量提供只读 MySQL 连接")
	}
	snapshot, e := migration.Export(context.Background(), dsn, *source, *id, *zone)
	if e != nil {
		log.Fatal(e)
	}
	if e = migration.SaveSnapshot(*out, snapshot); e != nil {
		log.Fatal(e)
	}
	rows := 0
	for _, t := range snapshot.Tables {
		rows += len(t.Rows)
	}
	log.Printf("已导出 %d 张表、%d 条原始记录到 %s", len(snapshot.Tables), rows, *out)
}

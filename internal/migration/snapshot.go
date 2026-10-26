package migration

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"os"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

const SnapshotFormat = "pxboard.migration/v1"

type Row map[string]any
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}
type Table struct {
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
}
type Snapshot struct {
	Format     string           `json:"format"`
	Source     string           `json:"source"`
	SourceID   string           `json:"sourceId"`
	ExportedAt string           `json:"exportedAt"`
	Timezone   string           `json:"timezone"`
	Tables     map[string]Table `json:"tables"`
}

func LoadSnapshot(path string) (*Snapshot, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var snapshot Snapshot
	decoder := json.NewDecoder(f)
	decoder.UseNumber()
	if e = decoder.Decode(&snapshot); e != nil {
		return nil, e
	}
	if snapshot.Format != SnapshotFormat || snapshot.SourceID == "" || snapshot.ExportedAt == "" || len(snapshot.Tables) == 0 {
		return nil, fmt.Errorf("不是完整的 PxBoard 迁移快照，请使用 export-migration 导出")
	}
	if _, e = time.LoadLocation(snapshot.Timezone); e != nil {
		return nil, fmt.Errorf("源时区无效：%w", e)
	}
	return &snapshot, nil
}
func SaveSnapshot(path string, snapshot *Snapshot) error {
	file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(snapshot)
}
func Export(ctx context.Context, dsn, source, sourceID, timezone string) (*Snapshot, error) {
	if !sourcePattern.MatchString(sourceID) {
		return nil, fmt.Errorf("来源标识只能使用小写字母、数字、下划线和连字符")
	}
	if _, e := time.LoadLocation(timezone); e != nil {
		return nil, e
	}
	config, e := mysql.ParseDSN(dsn)
	if e != nil {
		return nil, e
	}
	config.ParseTime = false
	config.MultiStatements = false
	db, e := sql.Open("mysql", config.FormatDSN())
	if e != nil {
		return nil, e
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	meta, e := tx.QueryContext(ctx, "SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME")
	if e != nil {
		return nil, e
	}
	tables := []string{}
	for meta.Next() {
		var name, engine string
		if e = meta.Scan(&name, &engine); e != nil {
			meta.Close()
			return nil, e
		}
		if !strings.EqualFold(engine, "InnoDB") {
			meta.Close()
			return nil, fmt.Errorf("%s 使用 %s，无法保证一致快照；请先将备份恢复到独立的 InnoDB 数据库", name, engine)
		}
		tables = append(tables, name)
	}
	e = meta.Err()
	meta.Close()
	if e != nil {
		return nil, e
	}
	snapshot := &Snapshot{Format: SnapshotFormat, Source: source, SourceID: sourceID, ExportedAt: time.Now().UTC().Format(time.RFC3339), Timezone: timezone, Tables: map[string]Table{}}
	for _, name := range tables {
		rows, e := tx.QueryContext(ctx, "SELECT * FROM `"+strings.ReplaceAll(name, "`", "``")+"`")
		if e != nil {
			return nil, e
		}
		columns, e := rows.ColumnTypes()
		if e != nil {
			rows.Close()
			return nil, e
		}
		table := Table{Rows: []Row{}}
		for _, column := range columns {
			nullable, _ := column.Nullable()
			table.Columns = append(table.Columns, Column{Name: column.Name(), Type: column.DatabaseTypeName(), Nullable: nullable})
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		for rows.Next() {
			if e = rows.Scan(pointers...); e != nil {
				rows.Close()
				return nil, e
			}
			row := Row{}
			for i, value := range values {
				switch v := value.(type) {
				case []byte:
					if utf8.Valid(v) {
						row[columns[i].Name()] = string(v)
					} else {
						row[columns[i].Name()] = map[string]any{"base64": base64.StdEncoding.EncodeToString(v)}
					}
				case int64:
					row[columns[i].Name()] = fmt.Sprint(v)
				case uint64:
					row[columns[i].Name()] = fmt.Sprint(v)
				case float64:
					row[columns[i].Name()] = fmt.Sprint(v)
				case time.Time:
					row[columns[i].Name()] = v.Format("2006-01-02 15:04:05.999999")
				default:
					row[columns[i].Name()] = v
				}
			}
			table.Rows = append(table.Rows, row)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		snapshot.Tables[name] = table
	}
	return snapshot, tx.Commit()
}
func (s *Snapshot) Names() []string {
	names := []string{}
	for name := range s.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (s *Snapshot) Rows(names ...string) []Row {
	for _, name := range names {
		if table, ok := s.Tables[name]; ok {
			return table.Rows
		}
	}
	return nil
}

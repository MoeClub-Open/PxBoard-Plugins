package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/agent"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type state struct {
	Counters       map[string]int64 `json:"counters"`
	Pending        map[string]any   `json:"pending,omitempty"`
	LogOffset      int64            `json:"logOffset"`
	LogInitialized bool             `json:"logInitialized"`
}

func main() {
	core := flag.String("panel", "", "面板 HTTPS 地址")
	node := flag.String("node", "", "节点 ID")
	token := flag.String("token", os.Getenv("PX_NODE_TOKEN"), "接入令牌，建议使用 PX_NODE_TOKEN")
	engine := flag.String("engine", "xray", "代理内核可执行文件")
	stats := flag.String("stats-cli", "xray", "查询流量使用的 Xray 命令，可用于 sing-box 的 V2Ray API")
	dir := flag.String("data", "agent-data", "状态与配置目录")
	accessLog := flag.String("access-log", "", "可选 Xray 访问日志路径，用于流量审计")
	flag.Parse()
	if *core == "" || *node == "" || *token == "" {
		log.Fatal("需要 --panel、--node 和 PX_NODE_TOKEN")
	}
	if err := os.MkdirAll(*dir, 0700); err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(*dir, "state.json")
	st := state{Counters: map[string]int64{}}
	if raw, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(raw, &st); e != nil {
			log.Fatal(e)
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var process *exec.Cmd
	var config []byte
	var wait chan error
	lastError := ""
	offset := st.LogOffset
	if !st.LogInitialized {
		offset = -1
	}
	stop := func() {
		if process != nil {
			_ = process.Process.Kill()
			if wait != nil {
				<-wait
			}
			process = nil
			wait = nil
		}
	}
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if process != nil && wait != nil {
			select {
			case err := <-wait:
				lastError = fmt.Sprint(err)
				process = nil
				wait = nil
			default:
			}
		}
		if st.Pending == nil {
			usage := []p.Record{}
			if process != nil {
				counters, e := query(ctx, *stats)
				if e != nil {
					lastError = e.Error()
				} else {
					for user, total := range counters {
						delta := total - st.Counters[user]
						if delta < 0 {
							delta = total
						}
						if delta > 0 {
							usage = append(usage, p.Record{"userId": user, "bytes": delta})
						}
						st.Counters[user] = total
					}
				}
			}
			flows := []p.Record{}
			if *accessLog != "" {
				var err error
				flows, offset, err = agent.ReadFlows(*accessLog, offset)
				if err == nil {
					st.LogOffset = offset
					st.LogInitialized = true
				}
				if err != nil {
					lastError = err.Error()
				}
			}
			st.Pending = map[string]any{"nodeId": *node, "reportId": p.ID(), "usage": usage, "connections": flows, "error": lastError}
			save(path, st)
		}
		body, _ := json.Marshal(st.Pending)
		req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(*core, "/")+"/api/v1/extensions/nodes/agent/sync", bytes.NewReader(body))
		if e != nil {
			log.Fatal(e)
		}
		req.Header.Set("X-Node-Token", *token)
		req.Header.Set("Content-Type", "application/json")
		res, e := (&http.Client{Timeout: 25 * time.Second}).Do(req)
		if e != nil {
			log.Println(e)
		} else {
			raw, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
			res.Body.Close()
			if readErr != nil {
				log.Println(readErr)
			} else if res.StatusCode >= 400 {
				log.Printf("sync HTTP %d: %s", res.StatusCode, raw)
			} else {
				var envelope struct {
					Data struct {
						Enabled bool       `json:"enabled"`
						Node    p.Record   `json:"node"`
						Clients []p.Record `json:"clients"`
						Rules   []p.Record `json:"rules"`
					} `json:"data"`
				}
				if e = json.Unmarshal(raw, &envelope); e != nil {
					log.Println(e)
				} else {
					st.Pending = nil
					save(path, st)
					if !envelope.Data.Enabled {
						stop()
						config = nil
					} else {
						next, e := agent.Config(envelope.Data.Node, envelope.Data.Clients, envelope.Data.Rules, *accessLog)
						if e != nil {
							lastError = e.Error()
							log.Println(e)
						} else if !bytes.Equal(config, next) || process == nil {
							configPath := filepath.Join(*dir, "config.json")
							if e = os.WriteFile(configPath, next, 0600); e != nil {
								log.Println(e)
							} else {
								args := []string{"run", "-config", configPath}
								if p.Text(envelope.Data.Node, "engine") == "sing-box" {
									args = []string{"run", "-c", configPath}
								}
								checkArgs := []string{"run", "-test", "-config", configPath}
								if p.Text(envelope.Data.Node, "engine") == "sing-box" {
									checkArgs = []string{"check", "-c", configPath}
								}
								check := exec.CommandContext(ctx, *engine, checkArgs...)
								if out, e := check.CombinedOutput(); e != nil {
									lastError = string(out)
									log.Println("invalid config:", string(out))
								} else {
									stop()
									process = exec.Command(*engine, args...)
									process.Stdout = os.Stdout
									process.Stderr = os.Stderr
									if e = process.Start(); e != nil {
										lastError = e.Error()
										log.Println(e)
										process = nil
									} else {
										wait = make(chan error, 1)
										running := process
										go func() { wait <- running.Wait() }()
										config = next
										st.Counters = map[string]int64{}
										save(path, st)
										lastError = ""
									}
								}
							}
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}
func save(path string, s state) {
	b, e := json.Marshal(s)
	if e != nil {
		log.Fatal(e)
	}
	if e = os.WriteFile(path+".tmp", b, 0600); e != nil {
		log.Fatal(e)
	}
	if e = os.Rename(path+".tmp", path); e != nil {
		log.Fatal(e)
	}
}
func query(parent context.Context, binary string) (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, binary, "api", "statsquery", "--server=127.0.0.1:10085", "-pattern", "user>>>", "-reset=false").Output()
	if e != nil {
		return nil, e
	}
	var result struct {
		Stat []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"stat"`
	}
	if e = json.Unmarshal(out, &result); e != nil {
		return nil, e
	}
	totals := map[string]int64{}
	for _, s := range result.Stat {
		parts := strings.Split(s.Name, ">>>")
		if len(parts) != 4 {
			continue
		}
		v, e := strconv.ParseInt(strings.Trim(string(s.Value), "\""), 10, 64)
		if e != nil {
			return nil, e
		}
		totals[parts[1]] += v
	}
	return totals, nil
}

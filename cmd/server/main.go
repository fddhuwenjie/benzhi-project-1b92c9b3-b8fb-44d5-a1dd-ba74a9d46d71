package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"museum-showcase/internal/cases"
	"museum-showcase/internal/dispatch"
	"museum-showcase/internal/httpui"
	"museum-showcase/internal/storage"
	"net/http"
	"os"
	"strings"
	"time"
)

func address(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if p := os.Getenv("PORT"); p != "" {
		return loopbackAddress(p)
	}
	return defaultListenAddress
}
func build() (*httpui.Server, *storage.Store, error) {
	dir := os.Getenv("MUSEUM_DATA_DIR")
	if dir == "" {
		dir = ".museum-data"
	}
	st, err := storage.New(dir)
	if err != nil {
		return nil, nil, err
	}
	cs := cases.New(st)
	ds := dispatch.New(st, cs)
	return httpui.New(cs, ds), st, nil
}
func smoke(addr string) error {
	tmp, err := os.MkdirTemp("", "museum-smoke-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	old := os.Getenv("MUSEUM_DATA_DIR")
	if err := os.Setenv("MUSEUM_DATA_DIR", tmp); err != nil {
		return err
	}
	defer os.Setenv("MUSEUM_DATA_DIR", old)
	s, _, err := build()
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	go srv.ListenAndServe()
	client := &http.Client{Timeout: 3 * time.Second}
	ready := false
	for n := 0; n < smokeProbeAttempts; n++ {
		r, e := client.Get("http://" + addr + "/")
		if e == nil {
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			if r.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		return fmt.Errorf("服务未就绪")
	}
	post := func(path, body string) (map[string]any, error) {
		req, e := http.NewRequest(http.MethodPost, "http://"+addr+path, strings.NewReader(body))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("smoke-%d", time.Now().UnixNano()))
		resp, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("%s 返回 %d: %v", path, resp.StatusCode, out)
		}
		return out, nil
	}
	inc, e := post("/api/incidents", `{"showcase":"S-01","artifact":"A-01","metric":"相对湿度","reading":80,"lower":45,"upper":60,"discovery":"自检告警","sensitivity":"高"}`)
	if e != nil {
		return e
	}
	id, _ := inc["id"].(string)
	rev := int(inc["revision"].(float64))
	task, e := post("/api/incidents/dispatch", fmt.Sprintf(`{"incident_id":%q,"assignee":"技术员甲","revision":%d}`, id, rev))
	if e != nil {
		return e
	}
	tid, _ := task["id"].(string)
	if _, e = post("/api/tasks/receive", fmt.Sprintf(`{"task_id":%q,"revision":%d}`, tid, int(task["revision"].(float64)))); e != nil {
		return e
	}
	if _, e = post("/api/tasks/action", fmt.Sprintf(`{"task_id":%q,"operator":"技术员甲","note":"调整空调并隔离展柜","categories":{"隔离":["隔离展柜"],"设备调整":["调整设备"],"保护措施":["保护文物"]},"attachments":["photo-smoke-001"]}`, tid)); e != nil {
		return e
	}
	if _, e = post("/api/tasks/verify", fmt.Sprintf(`{"task_id":%q,"reading":52}`, tid)); e != nil {
		return e
	}
	if _, e = post("/api/tasks/verify", fmt.Sprintf(`{"task_id":%q,"reading":53}`, tid)); e != nil {
		return e
	}
	if _, e = post("/api/tasks/review", fmt.Sprintf(`{"task_id":%q,"decision":"批准","note":"证据完整，批准关闭"}`, tid)); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
func main() {
	addrFlag := flag.String("addr", "", "监听地址")
	smokeFlag := flag.Bool("smoke", false, "执行端到端自检")
	flag.Parse()
	addr := address(*addrFlag)
	if *smokeFlag {
		if err := smoke(addr); err != nil {
			fmt.Fprintln(os.Stderr, "自检失败:", err)
			os.Exit(1)
		}
		fmt.Println("自检通过:", addr)
		return
	}
	s, _, err := build()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	fmt.Println("服务监听", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

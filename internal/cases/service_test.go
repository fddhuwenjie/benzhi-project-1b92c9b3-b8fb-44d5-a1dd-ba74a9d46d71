package cases

import (
	"museum-showcase/internal/storage"
	"testing"
)

func TestRiskAndRevision(t *testing.T) {
	dir := t.TempDir()
	st, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st)
	i, err := svc.Create("柜1", "文物1", "温度", 30, 18, 24, "巡检发现偏高", "高", "值班员")
	if err != nil {
		t.Fatal(err)
	}
	if i.Risk != "高" || i.Status != "新建" {
		t.Fatalf("风险或状态错误: %+v", i)
	}
	if _, err = svc.Transition(i.ID, "处置中", "技术员", "非法跳转", i.Revision); err == nil {
		t.Fatal("应拒绝非法状态跳转")
	}
	if _, err = svc.Transition(i.ID, "已分派", "主管", "分派", i.Revision+1); err == nil {
		t.Fatal("应拒绝旧修订号")
	}
}

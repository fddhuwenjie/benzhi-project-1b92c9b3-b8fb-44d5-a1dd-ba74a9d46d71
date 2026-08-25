package dispatch

import (
	"museum-showcase/internal/cases"
	"museum-showcase/internal/storage"
	"testing"
)

func TestFullWorkflow(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cs := cases.New(st)
	ds := New(st, cs)
	i, err := cs.Create("柜2", "文物2", "湿度", 75, 40, 60, "告警", "中", "值班员")
	if err != nil {
		t.Fatal(err)
	}
	task, err := ds.Assign(i.ID, "技术员乙", "主管", i.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ds.Act(task.ID, "技术员乙", "隔离并调整除湿设备", []string{"隔离展柜", "调整设备"}, []string{"photo-001"}, "技术员乙"); err != nil {
		t.Fatal(err)
	}
	if _, err = ds.Verify(task.ID, 50, "技术员乙"); err != nil {
		t.Fatal(err)
	}
	if _, err = ds.Review(task.ID, "批准", "复测合格，证据完整", "主管"); err != nil {
		t.Fatal(err)
	}
	closed, _, err := cs.Get(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != "已关闭" || closed.ClosedAt == nil {
		t.Fatalf("未关闭: %+v", closed)
	}
}

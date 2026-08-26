package dispatch

import (
	"errors"
	"fmt"
	"math"
	"museum-showcase/internal/cases"
	"museum-showcase/internal/storage"
	"sort"
	"strings"
	"sync"
	"time"
)

type Service struct {
	store       *storage.Store
	cases       *cases.Service
	mu          sync.Mutex
	loadScratch LoadSummary
}

const maxOpenTasks = 3

type LoadSummary struct {
	Assignee string         `json:"assignee"`
	Active   int            `json:"active"`
	Capacity int            `json:"capacity"`
	Tasks    []storage.Task `json:"tasks"`
}

type Candidate struct {
	Assignee       string     `json:"assignee"`
	Active         int        `json:"active"`
	Capacity       int        `json:"capacity"`
	Available      int        `json:"available"`
	Status         string     `json:"status"`
	LastReceivedAt *time.Time `json:"last_received_at,omitempty"`
}

func (s *Service) Load(assignee string) LoadSummary {
	s.loadScratch.Assignee = strings.TrimSpace(assignee)
	s.loadScratch.Active = 0
	s.loadScratch.Capacity = maxOpenTasks
	s.loadScratch.Tasks = s.loadScratch.Tasks[:0]
	for _, t := range s.store.Tasks() {
		if t.Assignee == s.loadScratch.Assignee {
			if i, ok := s.store.Incident(t.IncidentID); ok && i.Status != "已关闭" {
				s.loadScratch.Active++
				s.loadScratch.Tasks = append(s.loadScratch.Tasks, t)
			}
		}
	}
	out := s.loadScratch
	out.Tasks = append([]storage.Task(nil), s.loadScratch.Tasks...)
	return out
}
func (s *Service) Loads() []LoadSummary {
	seen := map[string]bool{}
	var out []LoadSummary
	for _, t := range s.store.Tasks() {
		if !seen[t.Assignee] {
			seen[t.Assignee] = true
			out = append(out, s.Load(t.Assignee))
		}
	}
	return out
}

func (s *Service) Candidates() []Candidate {
	seen := map[string]bool{}
	for _, t := range s.store.Tasks() {
		if strings.TrimSpace(t.Assignee) != "" {
			seen[t.Assignee] = true
		}
	}
	out := make([]Candidate, 0, len(seen))
	for name := range seen {
		load := s.Load(name)
		var latest *time.Time
		for _, t := range load.Tasks {
			if t.ReceivedAt != nil && (latest == nil || t.ReceivedAt.After(*latest)) {
				v := *t.ReceivedAt
				latest = &v
			}
		}
		status := "可接收"
		if load.Active >= load.Capacity {
			status = "已满"
		} else if load.Active+1 >= load.Capacity {
			status = "接近容量"
		}
		out = append(out, Candidate{Assignee: name, Active: load.Active, Capacity: load.Capacity, Available: load.Capacity - load.Active, Status: status, LastReceivedAt: latest})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active < out[j].Active
		}
		if out[i].LastReceivedAt == nil && out[j].LastReceivedAt == nil {
			return out[i].Assignee < out[j].Assignee
		}
		if out[i].LastReceivedAt == nil {
			return true
		}
		if out[j].LastReceivedAt == nil {
			return false
		}
		return out[i].LastReceivedAt.Before(*out[j].LastReceivedAt)
	})
	return out
}

func (s *Service) withTiming(task storage.Task) storage.Task {
	if task.RemediationDeadline != nil && task.RemediationOpen {
		r := time.Until(*task.RemediationDeadline)
		if r <= 0 {
			task.RemediationStatus = "逾期"
			task.RemediationOverdueSeconds = int64((-r) / time.Second)
			task.RemediationRemainingSeconds = 0
		} else {
			task.RemediationStatus = "正常"
			task.RemediationRemainingSeconds = int64(r / time.Second)
			task.RemediationOverdueSeconds = 0
			if r <= 2*time.Hour {
				task.RemediationStatus = "即将到期"
			}
		}
	}
	if task.AssignedAt == nil {
		task.ReceiveStatus = "未知"
		return task
	}
	if task.ReceivedAt != nil {
		task.ReceiveStatus = "已接收"
		return task
	}
	i, ok := s.store.Incident(task.IncidentID)
	if !ok {
		return task
	}
	window := receiveWindow(i.Risk)
	deadline := task.AssignedAt.Add(window)
	if deadline.After(i.Deadline) {
		deadline = i.Deadline
	}
	task.ReceiveDeadline = &deadline
	remaining := time.Until(deadline)
	if remaining <= 0 {
		task.ReceiveStatus = "超时"
		task.ReceiveOverdueSeconds = int64((-remaining) / time.Second)
		task.ReceiveReason = fmt.Sprintf("负责人%s尚未确认接收，已超过接收时限", task.Assignee)
	} else {
		task.ReceiveStatus = "待接收"
		task.ReceiveRemainingSeconds = int64(remaining / time.Second)
		task.ReceiveReason = "等待负责人确认接收"
	}
	return task
}

func (s *Service) TaskWithTiming(id string) (storage.Task, bool) {
	t, ok := s.store.Task(id)
	if !ok {
		return t, false
	}
	return s.withTiming(t), true
}

func New(s *storage.Store, c *cases.Service) *Service { return &Service{store: s, cases: c} }
func (s *Service) TaskForIncident(id string) (storage.Task, bool) {
	t, ok := s.store.TaskForIncident(id)
	if !ok {
		return t, false
	}
	return s.withTiming(t), true
}
func (s *Service) Task(id string) (storage.Task, bool) {
	t, ok := s.store.Task(id)
	if !ok {
		return t, false
	}
	return s.withTiming(t), true
}
func (s *Service) Assign(incidentID, assignee, actor string, expected int) (storage.Task, error) {
	return s.AssignWithKey(incidentID, assignee, actor, expected, "")
}

// AssignPending 用于公开接口的首次分派：任务必须由技术员显式确认接收。
func (s *Service) AssignPending(incidentID, assignee, actor string, expected int, key string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if t, found := s.store.Task(id); found {
				return t, nil
			}
		}
	}
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		return storage.Task{}, errors.New("负责人不能为空")
	}
	_, ok := s.store.Incident(incidentID)
	if !ok {
		return storage.Task{}, errors.New("异常不存在")
	}
	if _, exists := s.store.TaskForIncident(incidentID); exists {
		return storage.Task{}, errors.New("异常已存在分派任务")
	}
	load := s.Load(assignee)
	if load.Active >= load.Capacity {
		return storage.Task{}, fmt.Errorf("负责人%s负载已满（%d/%d）", assignee, load.Active, load.Capacity)
	}
	if _, err := s.cases.Transition(incidentID, "已分派", actor, "已分派给"+assignee, expected); err != nil {
		return storage.Task{}, err
	}
	now := time.Now().UTC()
	task := storage.Task{ID: fmt.Sprintf("TASK-%s", incidentID), IncidentID: incidentID, Assignee: assignee, AssignedAt: &now, AssignedBy: actor, Revision: 1, Measures: []string{}, Attachments: []string{}, UpdatedAt: now, StrictWorkflow: true, MeasureCategories: map[string][]string{}}
	t := storage.Timeline{ID: task.ID + "-T1", IncidentID: incidentID, Action: "分派", Actor: actor, Summary: fmt.Sprintf("负责人：%s，待接收；分派前负载 %d/%d，分派后 %d/%d", assignee, load.Active, load.Capacity, load.Active+1, load.Capacity), At: now, RequestKey: key}
	if err := s.store.SaveTask(task, t); err != nil {
		return storage.Task{}, err
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}

func (s *Service) Receive(taskID, actor string, expected int, key string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.store.Task(taskID)
	if !ok {
		return task, errors.New("任务不存在")
	}
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if t, found := s.store.Task(id); found {
				return t, nil
			}
		}
	}
	if task.ReceivedAt != nil {
		if expected > 0 && task.Revision != expected {
			return task, errors.New("修订号冲突，请刷新后重试")
		}
		return task, nil
	}
	if expected > 0 && task.Revision != expected {
		return task, errors.New("修订号冲突，请刷新后重试")
	}
	now := time.Now().UTC()
	task.ReceivedAt = &now
	task.Revision++
	task.UpdatedAt = now
	if err := s.store.UpdateTask(task, storage.Timeline{ID: fmt.Sprintf("%s-RC%d", task.ID, task.Revision), IncidentID: task.IncidentID, Action: "接收", Actor: actor, Summary: "技术员确认接收任务", At: now, RequestKey: key}); err != nil {
		return task, err
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}

func (s *Service) Transfer(taskID, assignee, reason, actor string, expected int, key string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.store.Task(taskID)
	if !ok {
		return task, errors.New("任务不存在")
	}
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if t, found := s.store.Task(id); found {
				return t, nil
			}
		}
	}
	assignee = strings.TrimSpace(assignee)
	reason = strings.TrimSpace(reason)
	if assignee == "" {
		return task, errors.New("新负责人不能为空")
	}
	if assignee == task.Assignee {
		return task, errors.New("不能转派给当前负责人")
	}
	load := s.Load(assignee)
	if load.Active >= load.Capacity {
		return task, fmt.Errorf("负责人%s负载已满（%d/%d）", assignee, load.Active, load.Capacity)
	}
	if len([]rune(reason)) < 2 {
		return task, errors.New("转派原因至少两字")
	}
	if expected > 0 && task.Revision != expected {
		return task, errors.New("修订号冲突，请刷新后重试")
	}
	i, ok := s.store.Incident(task.IncidentID)
	if !ok {
		return task, errors.New("异常不存在")
	}
	if i.Status != "已分派" {
		return task, errors.New("处置开始后不允许转派")
	}
	now := time.Now().UTC()
	old := task.Assignee
	task.Assignee = assignee
	task.TransferCount++
	task.TransferReason = reason
	task.Revision++
	task.UpdatedAt = now
	i.Revision++
	oldLoad := s.Load(old)
	if err := s.store.UpdateIncident(i, storage.Timeline{ID: fmt.Sprintf("%s-T%d", i.ID, i.Revision), IncidentID: i.ID, Action: "转派", Actor: actor, Summary: fmt.Sprintf("原负责人：%s；新负责人：%s；原因：%s；负载 %d/%d -> %d/%d", old, assignee, reason, oldLoad.Active-1, oldLoad.Capacity, load.Active+1, load.Capacity), At: now, RequestKey: key}); err != nil {
		return task, err
	}
	task.ReceivedAt = nil
	if err := s.store.UpdateTask(task, storage.Timeline{ID: fmt.Sprintf("%s-D%d", task.ID, task.Revision), IncidentID: i.ID, Action: "转派", Actor: actor, Summary: fmt.Sprintf("原负责人：%s；新负责人：%s；原因：%s；待接收", old, assignee, reason), At: now, RequestKey: key}); err != nil {
		return task, err
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}
func (s *Service) AssignWithKey(incidentID, assignee, actor string, expected int, key string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if t, found := s.store.Task(id); found {
				return t, nil
			}
		}
	}
	if strings.TrimSpace(assignee) == "" {
		return storage.Task{}, errors.New("负责人不能为空")
	}
	_, ok := s.store.Incident(incidentID)
	if !ok {
		return storage.Task{}, errors.New("异常不存在")
	}
	if existing, exists := s.store.TaskForIncident(incidentID); exists {
		i, _ := s.store.Incident(incidentID)
		if i.Status != "已分派" || expected > 0 && i.Revision != expected {
			return existing, errors.New("当前任务不允许转派或修订号冲突")
		}
		now := time.Now()
		i.Revision++
		if err := s.store.UpdateIncident(i, storage.Timeline{ID: incidentID + fmt.Sprintf("-T%d", i.Revision), IncidentID: incidentID, Action: "转派", Actor: actor, Summary: "转派责任人", At: now}); err != nil {
			return existing, err
		}
		existing.Assignee = strings.TrimSpace(assignee)
		existing.ReceivedAt = &now
		existing.AssignedAt = &now
		existing.Revision++
		t := storage.Timeline{ID: existing.ID + fmt.Sprintf("-D%d", existing.Revision), IncidentID: incidentID, Action: "转派", Actor: actor, Summary: "转派负责人：" + existing.Assignee, At: now}
		if err := s.store.UpdateTask(existing, t); err != nil {
			return existing, err
		}
		if key != "" {
			s.store.SetIdempotent(key, existing.ID)
		}
		return existing, nil
	}
	if _, err := s.cases.Transition(incidentID, "已分派", actor, "已分派给"+assignee, expected); err != nil {
		return storage.Task{}, err
	}
	now := time.Now()
	task := storage.Task{ID: fmt.Sprintf("TASK-%s", incidentID), IncidentID: incidentID, Assignee: strings.TrimSpace(assignee), ReceivedAt: &now, AssignedAt: &now, AssignedBy: actor, Revision: 1, Measures: []string{}, Attachments: []string{}, UpdatedAt: now}
	t := storage.Timeline{ID: task.ID + "-T1", IncidentID: incidentID, Action: "分派", Actor: actor, Summary: "负责人：" + assignee, At: now}
	if err := s.store.SaveTask(task, t); err != nil {
		return storage.Task{}, err
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}
func (s *Service) Act(taskID, operator, note string, measures, attachments []string, actor string) (storage.Task, error) {
	return s.ActWithKey(taskID, operator, note, measures, attachments, actor, 0, "")
}
func (s *Service) ActWithCategories(taskID, operator, note string, categories map[string][]string, attachments []string, actor string, expected int, key string) (storage.Task, error) {
	if categories == nil {
		categories = map[string][]string{}
	}
	measures := flattenCategories(categories)
	t, err := s.actWithKey(taskID, operator, note, measures, attachments, actor, expected, key, categories)
	return t, err
}
func (s *Service) ActWithKey(taskID, operator, note string, measures, attachments []string, actor string, expected int, key string) (storage.Task, error) {
	return s.actWithKey(taskID, operator, note, measures, attachments, actor, expected, key, nil)
}
func (s *Service) actWithKey(taskID, operator, note string, measures, attachments []string, actor string, expected int, key string, categories map[string][]string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.store.Task(taskID)
	if !ok {
		return task, errors.New("任务不存在")
	}
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if t, found := s.store.Task(id); found {
				return t, nil
			}
		}
	}
	if strings.TrimSpace(operator) == "" || len(strings.TrimSpace(note)) < 2 || len(measures) == 0 {
		return task, errors.New("操作人、措施说明和措施清单不能为空")
	}
	if len([]rune(strings.TrimSpace(operator))) > 40 || strings.ContainsAny(operator, "\r\n") {
		return task, errors.New("操作人格式非法")
	}
	if len([]rune(strings.TrimSpace(note))) > 500 {
		return task, errors.New("措施说明过长")
	}
	for _, r := range strings.TrimSpace(note) {
		if r < 0x20 || r == 0x7f {
			return task, errors.New("措施说明包含控制字符")
		}
	}
	if expected > 0 && task.Revision != expected {
		return task, errors.New("修订号冲突，请刷新后重试")
	}
	if task.ReceivedAt == nil {
		return task, errors.New("任务尚未被负责人接收")
	}
	if task.StrictWorkflow && categories != nil {
		for _, cat := range []string{"隔离", "设备调整", "保护措施"} {
			if len(categories[cat]) == 0 {
				return task, fmt.Errorf("缺少%s措施", cat)
			}
		}
	}
	clean := append([]string(nil), task.Measures...)
	seen := map[string]bool{}
	for _, m := range clean {
		seen[m] = true
	}
	requestSeen := map[string]bool{}
	for _, m := range measures {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if len([]rune(m)) > 80 {
			return task, errors.New("措施长度不能超过80字")
		}
		for _, r := range m {
			if r < 0x20 || r == 0x7f {
				return task, errors.New("措施包含控制字符")
			}
		}
		if requestSeen[m] {
			return task, errors.New("措施不能重复")
		}
		requestSeen[m] = true
		if seen[m] {
			return task, errors.New("措施不能重复")
		}
		seen[m] = true
		clean = append(clean, m)
	}
	if len(clean) == 0 {
		return task, errors.New("至少保留一项有效措施")
	}
	cleanA := append([]string(nil), task.Attachments...)
	seenA := map[string]bool{}
	for _, a := range cleanA {
		seenA[a] = true
	}
	requestSeenA := map[string]bool{}
	for _, a := range attachments {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if len(a) > 120 || strings.ContainsAny(a, " <>\"'") {
			return task, errors.New("附件索引格式非法")
		}
		for _, r := range a {
			if r < 0x20 || r == 0x7f {
				return task, errors.New("附件索引格式非法")
			}
		}
		if requestSeenA[a] {
			return task, errors.New("附件索引不能重复")
		}
		requestSeenA[a] = true
		if seenA[a] {
			return task, errors.New("附件索引不能重复")
		}
		seenA[a] = true
		cleanA = append(cleanA, a)
	}
	i, ok := s.store.Incident(task.IncidentID)
	if !ok {
		return task, errors.New("异常不存在")
	}
	if i.Status != "已分派" && i.Status != "处置中" {
		return task, fmt.Errorf("当前状态%s不能记录现场处置", i.Status)
	}
	if i.Status == "已分派" {
		if _, err := s.cases.Transition(i.ID, "处置中", actor, "开始现场处置", i.Revision); err != nil {
			return task, err
		}
	}
	task.Operator = strings.TrimSpace(operator)
	task.OperationNote = strings.TrimSpace(note)
	if task.Approval == "退回" {
		now := time.Now().UTC()
		task.RemediationCompletedAt = &now
	}
	task.Measures = clean
	task.Attachments = cleanA
	if categories != nil {
		task.MeasureCategories = categories
	}
	nowMeasure := time.Now().UTC()
	task.LastMeasureAt = &nowMeasure
	task.RemediationOpen = false
	remediationWindow := 24 * time.Hour
	if i.Risk == "高" {
		remediationWindow = 2 * time.Hour
	} else if i.Risk == "中" {
		remediationWindow = 8 * time.Hour
	}
	remediationDeadline := nowMeasure.Add(remediationWindow)
	task.RemediationDeadline = &remediationDeadline
	task.RemediationStatus = "正常"
	task.RemediationRemainingSeconds = 0
	task.RemediationOverdueSeconds = 0
	task.Revision++
	task.UpdatedAt = nowMeasure
	t := storage.Timeline{ID: task.ID + "-A" + task.UpdatedAt.Format("150405.000000000"), IncidentID: i.ID, Action: "现场处置", Actor: actor, Summary: note, Evidence: cleanA, At: task.UpdatedAt, RequestKey: key}
	if err := s.store.UpdateTask(task, t); err != nil {
		return task, err
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}

// ReviseAction 在首次复测前替换现场处置快照，并把旧值写入撤销时间线。
func (s *Service) ReviseAction(taskID, reason, operator, note string, categories map[string][]string, attachments []string, actor string, expected int, key string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.store.Task(taskID)
	if !ok {
		return task, errors.New("任务不存在")
	}
	if key != "" {
		if id, found := s.store.Idempotent(key, ""); found && id == task.ID {
			return task, nil
		}
	}
	if expected <= 0 || task.Revision != expected {
		return task, errors.New("修订号冲突，请刷新后重试")
	}
	if strings.TrimSpace(reason) == "" || len([]rune(strings.TrimSpace(reason))) < 2 {
		return task, errors.New("修订原因至少两字")
	}
	if task.LastRetestAt != nil || len(task.Retests) > 0 || task.Approval != "" {
		return task, errors.New("已有复测或主管复核，不允许修订现场记录")
	}
	i, ok := s.store.Incident(task.IncidentID)
	if !ok {
		return task, errors.New("异常不存在")
	}
	if i.Status != "处置中" {
		return task, fmt.Errorf("当前状态%s不能修订现场记录", i.Status)
	}
	if strings.TrimSpace(operator) == "" || len([]rune(strings.TrimSpace(note))) < 2 {
		return task, errors.New("操作人和措施说明不能为空")
	}
	if categories == nil {
		return task, errors.New("必须提供措施分类")
	}
	for _, cat := range []string{"隔离", "设备调整", "保护措施"} {
		if len(categories[cat]) == 0 {
			return task, fmt.Errorf("缺少%s措施", cat)
		}
	}
	cleanA := make([]string, 0, len(attachments))
	seen := map[string]bool{}
	for _, a := range attachments {
		a = strings.TrimSpace(a)
		if a == "" || len(a) > 120 || strings.ContainsAny(a, " <>\"'") || seen[a] {
			return task, errors.New("附件索引必须非空、唯一且格式合法")
		}
		seen[a] = true
		cleanA = append(cleanA, a)
	}
	if len(cleanA) == 0 {
		return task, errors.New("附件索引不能为空")
	}
	measures := make([]string, 0)
	seenMeasure := map[string]bool{}
	for _, list := range categories {
		for _, m := range list {
			m = strings.TrimSpace(m)
			if m == "" || seenMeasure[m] {
				return task, errors.New("措施必须非空且唯一")
			}
			seenMeasure[m] = true
			measures = append(measures, m)
		}
	}
	now := time.Now().UTC()
	oldMeasures := append([]string(nil), task.Measures...)
	oldAttachments := append([]string(nil), task.Attachments...)
	oldOperator := task.Operator
	task.Measures, task.Attachments, task.Operator = measures, cleanA, strings.TrimSpace(operator)
	task.OperationNote = strings.TrimSpace(note)
	task.MeasureCategories = categories
	task.Revision++
	task.UpdatedAt = now
	t := storage.Timeline{ID: fmt.Sprintf("%s-X%d", task.ID, task.Revision), IncidentID: task.IncidentID, Action: "现场处置修订", Actor: actor, Summary: fmt.Sprintf("修订原因：%s；已撤销旧现场措施并替换为新记录", strings.TrimSpace(reason)), Evidence: cleanA, At: now, RequestKey: key, PreviousMeasures: oldMeasures, PreviousAttachments: oldAttachments, PreviousOperator: oldOperator, Reason: strings.TrimSpace(reason)}
	if err := s.store.UpdateTask(task, t); err != nil {
		return task, err
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}

func (s *Service) Verify(taskID string, reading float64, actor string) (storage.Task, error) {
	return s.VerifyWithOptions(taskID, reading, actor, 0, "")
}
func (s *Service) VerifyWithKey(taskID string, reading float64, actor, key string) (storage.Task, error) {
	return s.VerifyWithOptions(taskID, reading, actor, 0, key)
}
func (s *Service) VerifyWithOptions(taskID string, reading float64, actor string, expected int, key string) (storage.Task, error) {
	return s.VerifyWithMeasuredAt(taskID, reading, nil, actor, expected, key)
}

func (s *Service) VerifyWithMeasuredAt(taskID string, reading float64, measuredAt *time.Time, actor string, expected int, key string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.store.Task(taskID)
	if !ok {
		return task, errors.New("任务不存在")
	}
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if t, found := s.store.Task(id); found {
				return t, nil
			}
		}
	}
	i, ok := s.store.Incident(task.IncidentID)
	if !ok {
		return task, errors.New("异常不存在")
	}
	if i.Status != "处置中" {
		return task, fmt.Errorf("当前状态%s不能复测", i.Status)
	}
	if math.IsNaN(reading) || math.IsInf(reading, 0) {
		return task, errors.New("复测读数必须是有限数值")
	}
	if task.StrictWorkflow && task.RemediationOpen {
		return task, errors.New("存在未完成再处置提醒，请先补充新增的隔离、设备调整和保护措施")
	}
	if expected > 0 && task.Revision != expected {
		return task, errors.New("修订号冲突，请刷新后重试")
	}
	passed := reading >= i.Lower && reading <= i.Upper
	diff := 0.0
	if reading < i.Lower {
		diff = i.Lower - reading
	}
	if reading > i.Upper {
		diff = reading - i.Upper
	}
	task.Retest = &reading
	task.RetestPassed = passed
	now := time.Now().UTC()
	if measuredAt != nil {
		now = measuredAt.UTC()
		if now.After(time.Now().UTC()) {
			return task, errors.New("采样时间不能晚于当前时间")
		}
	}
	if measuredAt != nil && task.LastRetestAt != nil {
		if !now.After(*task.LastRetestAt) {
			return task, fmt.Errorf("采样时间必须晚于上次采样时间 %s", task.LastRetestAt.Format(time.RFC3339))
		}
		interval := 15 * time.Minute
		if i.Risk == "高" {
			interval = time.Hour
		} else if i.Risk == "中" {
			interval = 30 * time.Minute
		}
		if now.Sub(*task.LastRetestAt) < interval {
			return task, fmt.Errorf("复测间隔不足，至少需要%s；上次采样时间 %s", interval, task.LastRetestAt.Format(time.RFC3339))
		}
	}
	trend := "持平"
	if len(task.Retests) > 0 {
		prev := task.Retests[len(task.Retests)-1].Difference
		if diff < prev {
			trend = "改善"
		} else if diff > prev {
			trend = "恶化"
		}
	}
	if len(task.Retests) >= 2 {
		p1, p2 := task.Retests[len(task.Retests)-2], task.Retests[len(task.Retests)-1]
		if p1.Passed != p2.Passed && p2.Passed != passed {
			task.FluctuationRisk = true
		}
	}
	task.RetestTrend = trend
	task.Retests = append(task.Retests, storage.Retest{Reading: reading, Passed: passed, Difference: diff, At: now, Trend: trend})
	task.LastRetestAt = &now
	if passed {
		task.StableCount++
		task.RemediationOpen = false
	} else {
		task.StableCount = 0
		if task.StrictWorkflow {
			task.RemediationOpen = true
		}
	}
	task.Revision++
	task.UpdatedAt = now
	t := storage.Timeline{ID: task.ID + "-V" + task.UpdatedAt.Format("150405.000000000"), IncidentID: i.ID, Action: "复测", Actor: actor, Summary: fmt.Sprintf("复测读数 %.2f，结论：%s", reading, map[bool]string{true: "合格", false: "不合格"}[passed]), At: task.UpdatedAt, RequestKey: key}
	if err := s.store.UpdateTask(task, t); err != nil {
		return task, err
	}
	if !passed && task.StrictWorkflow {
		direction := "低于下限"
		if reading > i.Upper {
			direction = "高于上限"
		}
		_ = s.store.UpdateTask(task, storage.Timeline{ID: task.ID + "-REM" + now.Format("150405.000000"), IncidentID: i.ID, Action: "再处置提醒", Actor: actor, Summary: fmt.Sprintf("复测不合格：%s，偏差 %.2f；请补充现场措施", direction, diff), At: now, RequestKey: key})
	}
	if passed && task.FluctuationRisk {
		task.StableCount = 0
	}
	if passed && (!task.StrictWorkflow || (task.StableCount >= 2 && !task.FluctuationRisk && trend != "恶化")) {
		if _, err := s.cases.Transition(i.ID, "待复核", actor, "复测合格，等待主管复核", i.Revision); err != nil {
			return task, err
		}
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}
func (s *Service) Review(taskID, decision, note, actor string) (storage.Task, error) {
	return s.ReviewWithOptions(taskID, decision, note, actor, 0, "")
}
func (s *Service) ReviewWithKey(taskID, decision, note, actor, key string) (storage.Task, error) {
	return s.ReviewWithOptions(taskID, decision, note, actor, 0, key)
}
func (s *Service) ReviewWithOptions(taskID, decision, note, actor string, expected int, key string) (storage.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.store.Task(taskID)
	if !ok {
		return task, errors.New("任务不存在")
	}
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if t, found := s.store.Task(id); found {
				return t, nil
			}
		}
	}
	i, ok := s.store.Incident(task.IncidentID)
	if !ok {
		return task, errors.New("异常不存在")
	}
	if i.Status != "待复核" {
		return task, fmt.Errorf("当前状态%s不能复核", i.Status)
	}
	if expected > 0 && task.Revision != expected {
		return task, errors.New("修订号冲突，请刷新后重试")
	}
	if !validReviewDecision(decision) {
		return task, errors.New("审批状态必须为批准或退回")
	}
	if strings.TrimSpace(note) == "" {
		return task, errors.New("复核意见不能为空")
	}
	if decision == "退回" && len([]rune(strings.TrimSpace(note))) < 2 {
		return task, errors.New("退回整改意见至少两字")
	}
	if decision == "批准" && (!task.RetestPassed || len(task.Measures) == 0 || len(task.Attachments) == 0) {
		return task, errors.New("批准前必须具备合格复测、措施记录和附件索引")
	}
	if decision == "批准" && task.StrictWorkflow && task.StableCount < 2 {
		return task, errors.New("最近两次复测尚未连续合格")
	}
	if decision == "批准" && task.RemediationCount > 0 && (len(task.Measures) <= task.LastReviewMeasureCount || len(task.Retests) <= task.LastReviewRetestCount) {
		return task, errors.New("退回后必须新增措施或附件且更新复测读数")
	}
	if decision == "批准" {
		seen := map[string]bool{}
		for _, a := range task.Attachments {
			if a == "" || seen[a] {
				return task, errors.New("归档附件索引必须非空且唯一")
			}
			seen[a] = true
		}
		ts := s.store.Timelines(i.ID)
		have := map[string]bool{}
		last := time.Time{}
		evidence := map[string]bool{}
		for _, ev := range ts {
			if !last.IsZero() && ev.At.Before(last) {
				return task, errors.New("归档时间线顺序不单调")
			}
			last = ev.At
			have[ev.Action] = true
			for _, a := range ev.Evidence {
				evidence[a] = true
			}
		}
		have["主管"+decision] = true
		for _, want := range []string{"登记", "分派", "现场处置", "复测", "主管批准"} {
			if !have[want] {
				return task, fmt.Errorf("归档缺少%s时间线", want)
			}
		}
		for _, a := range task.Attachments {
			if !evidence[a] {
				return task, fmt.Errorf("附件索引%s无法在时间线中对应", a)
			}
		}
	}
	task.ReviewNote = note
	task.Approval = decision
	if decision == "退回" {
		task.RemediationNote = note
		task.RemediationCount++
		task.LastReviewMeasureCount = len(task.Measures)
		task.LastReviewRetestCount = len(task.Retests)
		task.StableCount = 0
		task.RemediationOpen = true
		task.FluctuationRisk = false
		now := time.Now().UTC()
		window := 24 * time.Hour
		if i.Risk == "高" {
			window = 2 * time.Hour
		} else if i.Risk == "中" {
			window = 8 * time.Hour
		}
		deadline := now.Add(window)
		task.RemediationDeadline = &deadline
		task.RemediationStatus = "正常"
		task.RemediationRevision = task.RemediationCount
	}
	task.Revision++
	task.UpdatedAt = time.Now()
	summary := note
	if decision == "退回" {
		summary = fmt.Sprintf("整改轮次%d；附件%d项；%s", task.RemediationCount, len(task.Attachments), note)
	} else {
		summary = fmt.Sprintf("整改轮次%d；最后整改操作者%s；%s", task.RemediationCount, task.Operator, note)
	}
	t := storage.Timeline{ID: task.ID + "-R" + task.UpdatedAt.Format("150405.000000000"), IncidentID: i.ID, Action: "主管" + decision, Actor: actor, Summary: summary, At: task.UpdatedAt}
	if err := s.store.UpdateTask(task, t); err != nil {
		return task, err
	}
	target := "已关闭"
	if decision == "退回" {
		target = "处置中"
	}
	if decision == "批准" {
		archive := fmt.Sprintf("ARC-%s-%s", time.Now().Format("20060102"), strings.TrimPrefix(task.ID, "TASK-"))
		task.ArchiveID = archive
		task.ArchiveConsistency = "通过：登记、分派、现场处置、复测、主管批准和附件索引一致"
		if _, err := s.cases.Close(i.ID, actor, note, archive, i.Revision); err != nil {
			return task, err
		}
	} else if _, err := s.cases.Transition(i.ID, target, actor, note, i.Revision); err != nil {
		return task, err
	}
	if decision == "批准" {
		_ = s.store.UpdateTask(task, storage.Timeline{ID: task.ID + "-ARCH", IncidentID: i.ID, Action: "归档", Actor: actor, Summary: "归档编号：" + task.ArchiveID + "；一致性校验通过", Evidence: task.Attachments, At: time.Now()})
	}
	if key != "" {
		s.store.SetIdempotent(key, task.ID)
	}
	return task, nil
}

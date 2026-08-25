package cases

import (
	"errors"
	"fmt"
	"math"
	"museum-showcase/internal/storage"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var allowed = map[string]map[string]bool{statusNew: {statusAssigned: true}, statusAssigned: {statusActive: true}, statusActive: {statusReview: true}, statusReview: {statusClosed: true, statusActive: true}}

type Service struct {
	store *storage.Store
	mu    sync.Mutex
	seq   int
}

type IncidentInput struct {
	Showcase, Artifact, Metric string
	Reading, Lower, Upper      float64
	Discovery, Sensitivity     string
}

type BatchResult struct {
	BatchID   string             `json:"batch_id"`
	Incidents []storage.Incident `json:"incidents"`
}

type BatchIncident struct {
	Row      int              `json:"row"`
	Incident storage.Incident `json:"incident"`
}

type BatchSummary struct {
	BatchID      string          `json:"batch_id"`
	Total        int             `json:"total"`
	RiskCounts   map[string]int  `json:"risk_counts"`
	StatusCounts map[string]int  `json:"status_counts"`
	Incidents    []BatchIncident `json:"incidents"`
}

type ArchiveSummary struct {
	ArchiveID   string             `json:"archive_id"`
	Incident    storage.Incident   `json:"incident"`
	Task        storage.Task       `json:"task"`
	Timeline    []storage.Timeline `json:"timeline"`
	Attachments []string           `json:"attachments"`
	Durations   map[string]any     `json:"durations"`
	Consistency string             `json:"consistency"`
}

func New(s *storage.Store) *Service {
	seq := 0
	for _, i := range s.Incidents() {
		if p := strings.LastIndex(i.ID, "-"); p >= 0 {
			if n, err := strconv.Atoi(i.ID[p+1:]); err == nil && n > seq {
				seq = n
			}
		}
	}
	return &Service{store: s, seq: seq}
}
func risk(reading, lower, upper float64, sensitivity string) (string, time.Duration) {
	d := 0.0
	if reading < lower {
		d = lower - reading
	}
	if reading > upper {
		d = reading - upper
	}
	ratio := d / (upper - lower)
	if ratio < 0 {
		ratio = 0
	}
	if sensitivity == "高" {
		ratio *= 1.5
	}
	for _, band := range riskBands {
		threshold := band.minimum
		if sensitivity == "高" && band.level == "高" {
			threshold = 0.5
		}
		if ratio >= threshold {
			return band.level, band.deadline
		}
	}
	return "低", 24 * time.Hour
}

func (s *Service) Create(showcase, artifact, metric string, reading, lower, upper float64, discovery, sensitivity, actor string) (storage.Incident, error) {
	return s.CreateWithKey(showcase, artifact, metric, reading, lower, upper, discovery, sensitivity, actor, "")
}

// CreateBatch 对整个批次先校验后写入，任何一行失败都不会落库。
func (s *Service) CreateBatch(records []IncidentInput, actor, key string) (BatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(records) == 0 {
		return BatchResult{}, errors.New("至少提交一条指标记录")
	}
	if key != "" {
		for _, old := range s.store.Incidents() {
			if old.IdempotencyKey == key && old.BatchID != "" {
				var all []storage.Incident
				for _, i := range s.store.Incidents() {
					if i.BatchID == old.BatchID {
						all = append(all, i)
					}
				}
				return BatchResult{BatchID: old.BatchID, Incidents: all}, nil
			}
		}
	}
	batchID := fmt.Sprintf("BATCH-%s-%d", time.Now().UTC().Format("20060102150405.000"), s.seq+1)
	incidents := make([]storage.Incident, 0, len(records))
	timelines := make([]storage.Timeline, 0, len(records))
	for n, in := range records {
		showcase, err := normalize(in.Showcase, "展柜编号", 100)
		if err != nil {
			return BatchResult{}, fmt.Errorf("第%d行：%w", n+1, err)
		}
		artifact, err := normalize(in.Artifact, "文物编号", 100)
		if err != nil {
			return BatchResult{}, fmt.Errorf("第%d行：%w", n+1, err)
		}
		metric, err := normalize(in.Metric, "指标", 100)
		if err != nil {
			return BatchResult{}, fmt.Errorf("第%d行：%w", n+1, err)
		}
		discovery, err := normalize(in.Discovery, "发现说明", 1000)
		if err != nil {
			return BatchResult{}, fmt.Errorf("第%d行：%w", n+1, err)
		}
		if !finite(in.Reading) || !finite(in.Lower) || !finite(in.Upper) {
			return BatchResult{}, fmt.Errorf("第%d行：reading、lower、upper 必须是有限数值", n+1)
		}
		if in.Lower >= in.Upper {
			return BatchResult{}, fmt.Errorf("第%d行：允许下限必须小于上限", n+1)
		}
		if in.Reading >= in.Lower && in.Reading <= in.Upper {
			return BatchResult{}, fmt.Errorf("第%d行：reading 不能位于允许区间内", n+1)
		}
		if in.Sensitivity != "高" && in.Sensitivity != "中" && in.Sensitivity != "低" {
			return BatchResult{}, fmt.Errorf("第%d行：sensitivity 必须为高、中或低", n+1)
		}
		now := time.Now().UTC()
		riskLevel, ttl := risk(in.Reading, in.Lower, in.Upper, in.Sensitivity)
		deviation := 0.0
		if in.Reading < in.Lower {
			deviation = in.Lower - in.Reading
		}
		if in.Reading > in.Upper {
			deviation = in.Reading - in.Upper
		}
		s.seq++
		id := incidentIdentifier(now.Format("20060102"), s.seq)
		percent := deviation / (in.Upper - in.Lower) * 100
		i := storage.Incident{ID: id, ShowcaseID: showcase, ArtifactID: artifact, Metric: metric, Reading: in.Reading, LatestReading: in.Reading, OriginalReading: in.Reading, Lower: in.Lower, Upper: in.Upper, Discovery: discovery, OriginalDiscovery: discovery, Sensitivity: in.Sensitivity, Risk: riskLevel, Deviation: deviation, DeviationPercent: percent, RiskExplain: fmt.Sprintf("偏差 %.2f，占允许区间 %.2f%%；敏感度%s，%s风险阈值", deviation, percent, in.Sensitivity, riskLevel), Deadline: now.Add(ttl), Status: "新建", Revision: 1, CreatedAt: now, IdempotencyKey: key, BatchID: batchID, BatchRow: n + 1, DeadlineStatus: "正常", DeadlineCheckedAt: &now}
		i.History = s.history(showcase, artifact, metric)
		t := storage.Timeline{ID: id + "-T1", IncidentID: id, Action: "登记", Actor: actor, Summary: "批量登记环境异常，风险等级 " + riskLevel, At: now, RequestKey: key, BatchID: batchID}
		incidents = append(incidents, i)
		timelines = append(timelines, t)
	}
	if err := s.store.SaveIncidents(incidents, timelines); err != nil {
		return BatchResult{}, err
	}
	return BatchResult{BatchID: batchID, Incidents: incidents}, nil
}

func (s *Service) history(showcase, artifact, metric string) []storage.HistorySummary {
	all := s.store.Incidents()
	out := make([]storage.HistorySummary, 0)
	for _, i := range all {
		if i.ShowcaseID == showcase && i.ArtifactID == artifact && i.Metric == metric && i.Status == "已关闭" && i.ClosedAt != nil {
			out = append(out, storage.HistorySummary{IncidentID: i.ID, Risk: i.Risk, Deviation: i.Deviation, ClosedAt: *i.ClosedAt})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ClosedAt.After(out[b].ClosedAt) })
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func (s *Service) RelatedHistory(showcase, artifact, metric string) []storage.HistorySummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history(showcase, artifact, metric)
}
func (s *Service) CreateWithKey(showcase, artifact, metric string, reading, lower, upper float64, discovery, sensitivity, actor, key string) (storage.Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		if id, ok := s.store.Idempotent(key, ""); ok {
			if i, found := s.store.Incident(id); found {
				return i, nil
			}
		}
	}
	var err error
	showcase, err = normalize(showcase, "展柜编号", 100)
	if err != nil {
		return storage.Incident{}, err
	}
	artifact, err = normalize(artifact, "文物编号", 100)
	if err != nil {
		return storage.Incident{}, err
	}
	metric, err = normalize(metric, "指标", 100)
	if err != nil {
		return storage.Incident{}, err
	}
	discovery, err = normalize(discovery, "发现说明", 1000)
	if err != nil {
		return storage.Incident{}, err
	}
	if showcase == "" || artifact == "" || metric == "" || discovery == "" {
		return storage.Incident{}, errors.New("展柜、文物、指标和发现说明不能为空")
	}
	if !finite(reading) || !finite(lower) || !finite(upper) {
		return storage.Incident{}, errors.New("reading、lower、upper 必须是有限数值")
	}
	if lower >= upper {
		return storage.Incident{}, errors.New("允许下限必须小于上限")
	}
	if reading >= lower && reading <= upper {
		return storage.Incident{}, errors.New("reading 不能位于允许区间内")
	}
	if sensitivity != "高" && sensitivity != "中" && sensitivity != "低" {
		return storage.Incident{}, errors.New("sensitivity 必须为高、中或低")
	}
	for _, old := range s.store.Incidents() {
		if old.ShowcaseID == showcase && old.ArtifactID == artifact && old.Metric == metric && old.Status != "已关闭" {
			if old.LatestReading == 0 && old.Reading != 0 {
				old.LatestReading = old.Reading
			}
			old.Revision++
			now := time.Now().UTC()
			previousDeviation := old.Deviation
			previousDirection := deviationDirection(old.LatestReading, old.Lower, old.Upper)
			change := storage.ReadingChange{OriginalReading: old.LatestReading, Reading: reading, Discovery: discovery, At: now, Actor: actor, RequestKey: key}
			old.ReadingChanges = append(old.ReadingChanges, change)
			old.Reading = reading
			old.Discovery = discovery
			old.LatestReading = reading
			old.LatestChange = &change
			old.RelatedIncidentID = old.ID
			newRisk, ttl := risk(reading, old.Lower, old.Upper, old.Sensitivity)
			deviation := 0.0
			if reading < old.Lower {
				deviation = old.Lower - reading
			}
			if reading > old.Upper {
				deviation = reading - old.Upper
			}
			old.Risk, old.Deviation = newRisk, deviation
			old.DeviationPercent = deviation / (old.Upper - old.Lower) * 100
			currentDirection := deviationDirection(reading, old.Lower, old.Upper)
			change.Deviation = deviation
			change.Worsening = deviation > previousDeviation && currentDirection == previousDirection && currentDirection != ""
			old.ReadingChanges[len(old.ReadingChanges)-1] = change
			old.EscalationSuggested = change.Worsening
			old.DeviationTrend = "改善"
			if deviation > previousDeviation {
				old.DeviationTrend = "恶化"
			} else if deviation == previousDeviation {
				old.DeviationTrend = "持平"
			}
			old.EscalationReason = ""
			if change.Worsening {
				old.EscalationReason = fmt.Sprintf("同一指标偏差由 %.2f 扩大至 %.2f，建议升级处置", previousDeviation, deviation)
			}
			old.RiskExplain = fmt.Sprintf("偏差 %.2f，占允许区间 %.2f%%；敏感度%s，%s风险阈值；趋势%s", deviation, old.DeviationPercent, old.Sensitivity, newRisk, old.DeviationTrend)
			old.Deadline = now.Add(ttl)
			original, current := change.OriginalReading, reading
			t := storage.Timeline{ID: fmt.Sprintf("%s-T%d", old.ID, old.Revision), IncidentID: old.ID, Action: "重复告警", Actor: actor, Summary: fmt.Sprintf("原读数 %.2f，本次读数 %.2f；%s", change.OriginalReading, reading, discovery), At: now, RequestKey: key, OriginalReading: &original, Reading: &current, Discovery: discovery}
			if err := s.store.UpdateIncident(old, t); err != nil {
				return storage.Incident{}, err
			}
			if key != "" {
				s.store.SetIdempotent(key, old.ID)
			}
			return old, nil
		}
	}
	now := time.Now().UTC()
	riskLevel, ttl := risk(reading, lower, upper, sensitivity)
	deviation := 0.0
	if reading < lower {
		deviation = lower - reading
	}
	if reading > upper {
		deviation = reading - upper
	}
	s.seq++
	id := fmt.Sprintf("INC-%s-%03d", now.Format("20060102"), s.seq)
	percent := deviation / (upper - lower) * 100
	i := storage.Incident{ID: id, ShowcaseID: showcase, ArtifactID: artifact, Metric: metric, Reading: reading, LatestReading: reading, OriginalReading: reading, Lower: lower, Upper: upper, Discovery: discovery, OriginalDiscovery: discovery, Sensitivity: sensitivity, Risk: riskLevel, Deviation: deviation, DeviationPercent: percent, RiskExplain: fmt.Sprintf("偏差 %.2f，占允许区间 %.2f%%；敏感度%s，%s风险阈值", deviation, percent, sensitivity, riskLevel), Deadline: now.Add(ttl), Status: "新建", Revision: 1, CreatedAt: now, IdempotencyKey: key, DeadlineStatus: "正常", DeadlineCheckedAt: &now}
	i.History = s.history(showcase, artifact, metric)
	t := storage.Timeline{ID: id + "-T1", IncidentID: id, Action: "登记", Actor: actor, Summary: "登记环境异常，风险等级 " + riskLevel, At: now, RequestKey: key}
	if err := s.store.SaveIncident(i, t); err != nil {
		return storage.Incident{}, err
	}
	if key != "" {
		s.store.SetIdempotent(key, id)
	}
	return i, nil
}

func deviationDirection(reading, lower, upper float64) string {
	if reading < lower {
		return "低于下限"
	}
	if reading > upper {
		return "高于上限"
	}
	return ""
}

// Correct 保留首次登记快照，只替换活动异常的当前判断值。
func (s *Service) Correct(id string, reading, lower, upper float64, discovery, reason, actor string, expected int, key string) (storage.Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.store.Incident(strings.TrimSpace(id))
	if !ok {
		return i, errors.New("异常不存在")
	}
	if key != "" {
		if oldID, found := s.store.Idempotent(key, ""); found && oldID == i.ID {
			return i, nil
		}
	}
	if i.Status == "已关闭" {
		return i, fmt.Errorf("异常已关闭，不可更正；归档编号：%s", i.ArchiveID)
	}
	if expected <= 0 || i.Revision != expected {
		return i, errors.New("修订号冲突，请刷新后重试")
	}
	discovery, err := normalize(discovery, "发现说明", 1000)
	if err != nil || discovery == "" {
		if err != nil {
			return i, err
		}
		return i, errors.New("发现说明不能为空")
	}
	reason, err = normalize(reason, "更正原因", 500)
	if err != nil || reason == "" {
		if err != nil {
			return i, err
		}
		return i, errors.New("更正原因不能为空")
	}
	if !finite(reading) || !finite(lower) || !finite(upper) {
		return i, errors.New("reading、lower、upper 必须是有限数值")
	}
	if lower >= upper {
		return i, errors.New("允许下限必须小于上限")
	}
	if reading >= lower && reading <= upper {
		return i, errors.New("reading 不能位于允许区间内")
	}
	if i.OriginalDiscovery == "" {
		i.OriginalReading = i.Reading
		i.OriginalDiscovery = i.Discovery
	}
	previous := i.LatestReading
	if previous == 0 && i.Reading != 0 {
		previous = i.Reading
	}
	oldRevision := i.Revision
	riskLevel, ttl := risk(reading, lower, upper, i.Sensitivity)
	deviation := math.Abs(reading - lower)
	if reading > upper {
		deviation = reading - upper
	}
	if reading >= lower && reading <= upper {
		deviation = 0
	}
	now := time.Now().UTC()
	change := storage.ReadingChange{OriginalReading: previous, Reading: reading, Discovery: discovery, At: now, Actor: actor, RequestKey: key, Deviation: deviation, Reason: reason, Revision: oldRevision + 1}
	i.Reading = reading
	i.LatestReading = reading
	i.Lower = lower
	i.Upper = upper
	i.Discovery = discovery
	i.Risk = riskLevel
	i.Deviation = deviation
	i.DeviationPercent = deviation / (upper - lower) * 100
	i.RiskExplain = fmt.Sprintf("偏差 %.2f，占允许区间 %.2f%%；敏感度%s，%s风险阈值", deviation, i.DeviationPercent, i.Sensitivity, riskLevel)
	i.Deadline = now.Add(ttl)
	i.DeadlineStatus = "正常"
	i.DeadlineCheckedAt = &now
	i.Revision++
	i.ReadingChanges = append(i.ReadingChanges, change)
	i.LatestChange = &change
	t := storage.Timeline{ID: fmt.Sprintf("%s-C%d", i.ID, i.Revision), IncidentID: i.ID, Action: "异常更正", Actor: actor, Summary: fmt.Sprintf("读数 %.2f -> %.2f；说明已更新；原因：%s", previous, reading, reason), At: now, RequestKey: key, OriginalReading: &previous, Reading: &reading, Discovery: discovery, Reason: reason}
	if err := s.store.UpdateIncident(i, t); err != nil {
		return i, err
	}
	if key != "" {
		s.store.SetIdempotent(key, i.ID)
	}
	return i, nil
}

func normalize(v, field string, max int) (string, error) {
	v = cleanText(v)
	if v == "" {
		return "", fmt.Errorf("%s不能为空", field)
	}
	if len([]rune(v)) > max {
		return "", fmt.Errorf("%s长度不能超过%d字", field, max)
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%s包含控制字符", field)
		}
	}
	return v, nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (s *Service) Get(id string) (storage.Incident, []storage.Timeline, error) {
	i, ok := s.store.Incident(id)
	if !ok {
		return storage.Incident{}, nil, errors.New("异常不存在")
	}
	return i, s.store.Timelines(id), nil
}

func (s *Service) ByArchive(archive string) (storage.Incident, bool) {
	return s.store.IncidentByArchive(archive)
}

func (s *Service) Batch(id string) (BatchSummary, error) {
	id = strings.TrimSpace(id)
	if id == "" || !strings.HasPrefix(id, "BATCH-") {
		return BatchSummary{}, errors.New("批次编号格式非法")
	}
	result := BatchSummary{BatchID: id, RiskCounts: map[string]int{"高": 0, "中": 0, "低": 0}, StatusCounts: map[string]int{}, Incidents: []BatchIncident{}}
	for _, i := range s.store.IncidentsByBatch(id) {
		row := i.BatchRow
		if row == 0 {
			row = len(result.Incidents) + 1
		}
		result.Incidents = append(result.Incidents, BatchIncident{Row: row, Incident: i})
		result.RiskCounts[i.Risk]++
		result.StatusCounts[i.Status]++
	}
	if len(result.Incidents) == 0 {
		return BatchSummary{}, errors.New("批次不存在")
	}
	sort.Slice(result.Incidents, func(a, b int) bool { return result.Incidents[a].Row < result.Incidents[b].Row })
	result.Total = len(result.Incidents)
	return result, nil
}

func (s *Service) Archive(archive string) (ArchiveSummary, error) {
	i, ok := s.store.IncidentByArchive(strings.TrimSpace(archive))
	if !ok {
		return ArchiveSummary{}, errors.New("归档编号不存在")
	}
	if i.Status != "已关闭" {
		return ArchiveSummary{}, errors.New("归档异常尚未关闭")
	}
	task, ok := s.store.TaskForIncident(i.ID)
	if !ok {
		return ArchiveSummary{}, errors.New("归档缺少任务快照")
	}
	ts := s.store.Timelines(i.ID)
	if len(ts) == 0 {
		return ArchiveSummary{}, errors.New("归档时间线为空")
	}
	last := time.Time{}
	have := map[string]bool{}
	evidence := map[string]bool{}
	for _, ev := range ts {
		if !last.IsZero() && ev.At.Before(last) {
			return ArchiveSummary{}, fmt.Errorf("归档时间线顺序错误：%s", ev.ID)
		}
		last = ev.At
		have[ev.Action] = true
		for _, a := range ev.Evidence {
			evidence[a] = true
		}
	}
	for _, want := range []string{"登记", "分派", "现场处置", "复测", "主管批准", "归档"} {
		if !have[want] {
			return ArchiveSummary{}, fmt.Errorf("归档缺少%s时间线", want)
		}
	}
	attachments := make([]string, 0, len(task.Attachments))
	seen := map[string]bool{}
	for _, a := range task.Attachments {
		if a != "" && !seen[a] {
			seen[a] = true
			attachments = append(attachments, a)
		}
	}
	for _, a := range attachments {
		if !evidence[a] {
			return ArchiveSummary{}, fmt.Errorf("附件索引%s缺少时间线证据", a)
		}
	}
	find := func(action string) time.Time {
		for _, ev := range ts {
			if ev.Action == action {
				return ev.At
			}
		}
		return time.Time{}
	}
	registered, assigned, received, firstRetest, approved, closed := find("登记"), find("分派"), find("接收"), find("复测"), find("主管批准"), i.ClosedAt
	if closed == nil {
		return ArchiveSummary{}, errors.New("归档缺少关闭时间")
	}
	durations := map[string]any{"登记到关闭": closed.Sub(registered).String(), "分派到接收": "缺失", "首次复测到批准": "缺失"}
	if !received.IsZero() {
		durations["分派到接收"] = received.Sub(assigned).String()
	}
	if !firstRetest.IsZero() && !approved.IsZero() {
		durations["首次复测到批准"] = approved.Sub(firstRetest).String()
	}
	return ArchiveSummary{ArchiveID: archive, Incident: i, Task: task, Timeline: ts, Attachments: attachments, Durations: durations, Consistency: "通过"}, nil
}

type ListFilter struct {
	Status, Risk, Showcase, Assignee string
	From, To                         *time.Time
	Priority                         bool
}
type ListResult struct {
	Incidents      []storage.Incident `json:"incidents"`
	Stats          map[string]int     `json:"stats"`
	Counts         map[string]int     `json:"counts,omitempty"`
	Queue          []QueueEntry       `json:"queue,omitempty"`
	PriorityCounts map[string]int     `json:"priority_counts,omitempty"`
}

type QueueEntry struct {
	Incident    storage.Incident `json:"incident"`
	Priority    int              `json:"priority"`
	Basis       string           `json:"basis"`
	Position    int              `json:"position"`
	MissingTask bool             `json:"missing_task"`
}

type QueueFilter struct {
	Status   string
	Assignee string
	Risk     string
	Showcase string
	From     *time.Time
	To       *time.Time
}

type QueueResult struct {
	Items          []QueueEntry   `json:"items"`
	PriorityCounts map[string]int `json:"priority_counts"`
}

func (s *Service) List() []storage.Incident { return s.store.Incidents() }
func (s *Service) ListFiltered(f ListFilter) ListResult {
	out := []storage.Incident{}
	stats := map[string]int{"total": 0, "新建": 0, "已分派": 0, "处置中": 0, "待复核": 0, "已关闭": 0, "高风险": 0, "即将到期": 0, "逾期": 0, "整改正常": 0, "整改即将到期": 0, "整改逾期": 0}
	now := time.Now().UTC()
	for _, i := range s.store.Incidents() {
		if f.Status != "" && i.Status != f.Status || f.Risk != "" && i.Risk != f.Risk || f.Showcase != "" && i.ShowcaseID != f.Showcase || (f.From != nil && i.CreatedAt.Before(*f.From)) || (f.To != nil && i.CreatedAt.After(*f.To)) {
			continue
		}
		status := deadlineStatus(i, now)
		if i.DeadlineStatus != status {
			// 状态是派生值；保存只在实际业务更新时发生，避免查询产生事件。
			i.DeadlineStatus = status
		}
		matched := f.Assignee == ""
		for _, task := range s.store.Tasks() {
			if task.IncidentID != i.ID {
				continue
			}
			i.Assignee = task.Assignee
			if f.Assignee != "" && task.Assignee != f.Assignee {
				matched = false
				break
			}
			if f.Assignee != "" {
				matched = true
			}
			i.ReceiveStatus = "已接收"
			if task.ReceivedAt == nil {
				i.ReceiveStatus = "待接收"
				i.ReceiveReason = "等待负责人确认接收"
				if task.AssignedAt != nil {
					window := 4 * time.Hour
					if i.Risk == "高" {
						window = time.Hour
					} else if i.Risk == "低" {
						window = 8 * time.Hour
					}
					deadline := task.AssignedAt.Add(window)
					if deadline.After(i.Deadline) {
						deadline = i.Deadline
					}
					i.ReceiveDeadline = &deadline
					r := deadline.Sub(now)
					if r <= 0 {
						i.ReceiveStatus = "超时"
						i.ReceiveOverdueSeconds = int64((-r) / time.Second)
					} else {
						i.ReceiveRemainingSeconds = int64(r / time.Second)
					}
				}
			}
			if task.RemediationOpen {
				i.RemediationStatus = task.RemediationStatus
			}
		}
		if !matched {
			continue
		}
		out = append(out, i)
		stats["total"]++
		stats[i.Status]++
		if i.Risk == "高" {
			stats["高风险"]++
		}
		if i.Status != "已关闭" && status == "逾期" {
			stats["逾期"]++
		}
		if i.Status != "已关闭" && status == "即将到期" {
			stats["即将到期"]++
		}
		for _, task := range s.store.Tasks() {
			if task.IncidentID != i.ID || !task.RemediationOpen || task.RemediationDeadline == nil {
				continue
			}
			r := task.RemediationDeadline.Sub(now)
			if r <= 0 {
				stats["整改逾期"]++
			} else if r <= 2*time.Hour {
				stats["整改即将到期"]++
			} else {
				stats["整改正常"]++
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].CreatedAt.Equal(out[b].CreatedAt) {
			return out[a].ID < out[b].ID
		}
		return out[a].CreatedAt.Before(out[b].CreatedAt)
	})
	result := ListResult{Incidents: out, Stats: stats, Counts: stats}
	if f.Priority {
		q := s.Queue(QueueFilter{Status: f.Status, Assignee: f.Assignee, Risk: f.Risk, Showcase: f.Showcase, From: f.From, To: f.To})
		result.Queue = q.Items
		result.PriorityCounts = q.PriorityCounts
		result.Incidents = make([]storage.Incident, 0, len(q.Items))
		for _, item := range q.Items {
			result.Incidents = append(result.Incidents, item.Incident)
		}
	}
	return result
}

// Queue 计算未关闭异常的实时处置优先级，不产生任何持久化写入。
func (s *Service) Queue(f QueueFilter) QueueResult {
	now := time.Now().UTC()
	items := make([]QueueEntry, 0)
	counts := map[string]int{"紧急": 0, "高": 0, "中": 0, "低": 0, "待补全": 0}
	for _, i := range s.store.Incidents() {
		if i.Status == "已关闭" || (f.Status != "" && i.Status != f.Status) || (f.Risk != "" && i.Risk != f.Risk) || (f.Showcase != "" && i.ShowcaseID != f.Showcase) || (f.From != nil && i.CreatedAt.Before(*f.From)) || (f.To != nil && i.CreatedAt.After(*f.To)) {
			continue
		}
		var task storage.Task
		hasTask := false
		if t, ok := s.store.TaskForIncident(i.ID); ok {
			task, hasTask = t, true
		}
		if f.Assignee != "" && (!hasTask || task.Assignee != f.Assignee) {
			continue
		}
		priority := 0
		basis := ""
		if !hasTask || i.Deadline.IsZero() {
			priority, basis = 10, "待补全：缺少处置任务"
			if i.Deadline.IsZero() {
				basis = "待补全：缺少处置截止时间"
			}
		} else if task.RemediationOpen && task.RemediationDeadline != nil && !task.RemediationDeadline.After(now) {
			priority, basis = 100, "整改逾期"
		} else if task.ReceivedAt == nil && task.AssignedAt != nil {
			receiveWindow := 4 * time.Hour
			if i.Risk == "高" {
				receiveWindow = time.Hour
			} else if i.Risk == "低" {
				receiveWindow = 8 * time.Hour
			}
			if !task.AssignedAt.Add(receiveWindow).After(now) {
				priority, basis = 90, "接收超时"
			}
		}
		if priority == 0 {
			remaining := i.Deadline.Sub(now)
			priority = map[string]int{"高": 70, "中": 50, "低": 30}[i.Risk]
			basis = i.Risk + "风险"
			if remaining <= 0 {
				priority += 40
				basis = "处置截止逾期；" + basis
			} else if remaining <= 2*time.Hour {
				priority += 30
				basis = "处置即将逾期；" + basis
			}
		}
		items = append(items, QueueEntry{Incident: i, Priority: priority, Basis: basis, MissingTask: !hasTask})
		label := "低"
		if priority >= 90 {
			label = "紧急"
		} else if priority >= 60 {
			label = "高"
		} else if priority >= 40 {
			label = "中"
		}
		counts[label]++
		if !hasTask {
			counts["待补全"]++
		}
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].Priority != items[b].Priority {
			return items[a].Priority > items[b].Priority
		}
		return items[a].Incident.ID < items[b].Incident.ID
	})
	for n := range items {
		items[n].Position = n + 1
		items[n].Incident.QueuePriority = items[n].Priority
		items[n].Incident.QueueBasis = items[n].Basis
		items[n].Incident.QueuePosition = n + 1
		items[n].Incident.QueueMissingTask = items[n].MissingTask
	}
	return QueueResult{Items: items, PriorityCounts: counts}
}

func deadlineStatus(i storage.Incident, now time.Time) string {
	if i.Status == "已关闭" {
		return "已关闭"
	}
	remaining := i.Deadline.Sub(now)
	if remaining <= 0 {
		return "逾期"
	}
	window := 2 * time.Hour
	if i.Risk == "中" {
		window = 4 * time.Hour
	}
	if i.Risk == "低" {
		window = 8 * time.Hour
	}
	if remaining <= window {
		return "即将到期"
	}
	return "正常"
}

func (s *Service) DeadlineInfo(i storage.Incident) (string, time.Duration, time.Duration) {
	status := deadlineStatus(i, time.Now().UTC())
	d := time.Until(i.Deadline)
	if d >= 0 {
		return status, d, 0
	}
	return status, 0, -d
}
func (s *Service) Transition(id, to, actor, summary string, expected int) (storage.Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.store.Incident(id)
	if !ok {
		return i, errors.New("异常不存在")
	}
	if expected > 0 && i.Revision != expected {
		return i, errors.New("修订号冲突，请刷新后重试")
	}
	if !allowed[i.Status][to] {
		return i, fmt.Errorf("不允许从%s变更为%s", i.Status, to)
	}
	i.Status = to
	i.Revision++
	checked := time.Now().UTC()
	i.DeadlineStatus = deadlineStatus(i, checked)
	i.DeadlineCheckedAt = &checked
	if to == "已关闭" {
		i.ClosedAt = &checked
	}
	t := storage.Timeline{ID: fmt.Sprintf("%s-T%d", id, i.Revision), IncidentID: id, Action: to, Actor: actor, Summary: strings.TrimSpace(summary), At: time.Now().UTC()}
	if err := s.store.UpdateIncident(i, t); err != nil {
		return i, err
	}
	return i, nil
}

func (s *Service) Close(id, actor, summary, archive string, expected int) (storage.Incident, error) {
	i, err := s.Transition(id, "已关闭", actor, summary, expected)
	if err != nil {
		return i, err
	}
	i.ArchiveID = archive
	// persist archive identifier without adding a duplicate transition event
	if err := s.store.UpdateIncident(i, storage.Timeline{ID: id + "-归档", IncidentID: id, Action: "归档", Actor: actor, Summary: "归档编号：" + archive, At: time.Now().UTC()}); err != nil {
		return i, err
	}
	return i, nil
}

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

type Incident struct {
	ID                      string           `json:"id"`
	ShowcaseID              string           `json:"showcase_id"`
	ArtifactID              string           `json:"artifact_id"`
	Metric                  string           `json:"metric"`
	Reading                 float64          `json:"reading"`
	LatestReading           float64          `json:"latest_reading"`
	OriginalReading         float64          `json:"original_reading"`
	OriginalDiscovery       string           `json:"original_discovery"`
	ReadingChanges          []ReadingChange  `json:"reading_changes,omitempty"`
	Lower                   float64          `json:"lower"`
	Upper                   float64          `json:"upper"`
	Discovery               string           `json:"discovery"`
	Sensitivity             string           `json:"sensitivity"`
	Risk                    string           `json:"risk"`
	Deviation               float64          `json:"deviation"`
	DeviationPercent        float64          `json:"deviation_percent"`
	RiskExplain             string           `json:"risk_explain"`
	Deadline                time.Time        `json:"deadline"`
	Status                  string           `json:"status"`
	Revision                int              `json:"revision"`
	CreatedAt               time.Time        `json:"created_at"`
	ClosedAt                *time.Time       `json:"closed_at,omitempty"`
	ArchiveID               string           `json:"archive_id,omitempty"`
	RelatedIncidentID       string           `json:"related_incident_id,omitempty"`
	LatestChange            *ReadingChange   `json:"latest_change,omitempty"`
	IdempotencyKey          string           `json:"idempotency_key,omitempty"`
	DeadlineStatus          string           `json:"deadline_status,omitempty"`
	DeadlineCheckedAt       *time.Time       `json:"deadline_checked_at,omitempty"`
	BatchID                 string           `json:"batch_id,omitempty"`
	History                 []HistorySummary `json:"history,omitempty"`
	EscalationSuggested     bool             `json:"escalation_suggested,omitempty"`
	EscalationReason        string           `json:"escalation_reason,omitempty"`
	DeviationTrend          string           `json:"deviation_trend,omitempty"`
	BatchRow                int              `json:"batch_row,omitempty"`
	Assignee                string           `json:"assignee,omitempty"`
	ReceiveStatus           string           `json:"receive_status,omitempty"`
	ReceiveReason           string           `json:"receive_reason,omitempty"`
	ReceiveDeadline         *time.Time       `json:"receive_deadline,omitempty"`
	ReceiveRemainingSeconds int64            `json:"receive_remaining_seconds,omitempty"`
	ReceiveOverdueSeconds   int64            `json:"receive_overdue_seconds,omitempty"`
	RemediationStatus       string           `json:"remediation_status,omitempty"`
	QueuePriority           int              `json:"queue_priority,omitempty"`
	QueueBasis              string           `json:"queue_basis,omitempty"`
	QueuePosition           int              `json:"queue_position,omitempty"`
	QueueMissingTask        bool             `json:"queue_missing_task,omitempty"`
}

type HistorySummary struct {
	IncidentID string    `json:"incident_id"`
	Risk       string    `json:"risk"`
	Deviation  float64   `json:"deviation"`
	ClosedAt   time.Time `json:"closed_at"`
}

type ReadingChange struct {
	OriginalReading float64   `json:"original_reading"`
	Reading         float64   `json:"reading"`
	Discovery       string    `json:"discovery"`
	At              time.Time `json:"at"`
	Actor           string    `json:"actor,omitempty"`
	RequestKey      string    `json:"request_key,omitempty"`
	Deviation       float64   `json:"deviation,omitempty"`
	Worsening       bool      `json:"worsening,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	Revision        int       `json:"revision,omitempty"`
}

type Retest struct {
	Reading    float64   `json:"reading"`
	Passed     bool      `json:"passed"`
	Difference float64   `json:"difference"`
	At         time.Time `json:"at"`
	Trend      string    `json:"trend,omitempty"`
}

type Task struct {
	ID                          string              `json:"id"`
	IncidentID                  string              `json:"incident_id"`
	Assignee                    string              `json:"assignee"`
	ReceivedAt                  *time.Time          `json:"received_at,omitempty"`
	AssignedAt                  *time.Time          `json:"assigned_at,omitempty"`
	AssignedBy                  string              `json:"assigned_by,omitempty"`
	Revision                    int                 `json:"revision"`
	Measures                    []string            `json:"measures"`
	Operator                    string              `json:"operator"`
	OperationNote               string              `json:"operation_note"`
	Retest                      *float64            `json:"retest,omitempty"`
	RetestPassed                bool                `json:"retest_passed"`
	Retests                     []Retest            `json:"retests,omitempty"`
	ReviewNote                  string              `json:"review_note"`
	Approval                    string              `json:"approval"`
	Attachments                 []string            `json:"attachments"`
	UpdatedAt                   time.Time           `json:"updated_at"`
	ArchiveID                   string              `json:"archive_id,omitempty"`
	IdempotencyKey              string              `json:"idempotency_key,omitempty"`
	TransferCount               int                 `json:"transfer_count,omitempty"`
	TransferReason              string              `json:"transfer_reason,omitempty"`
	RemediationNote             string              `json:"remediation_note,omitempty"`
	RemediationCompletedAt      *time.Time          `json:"remediation_completed_at,omitempty"`
	MeasureCategories           map[string][]string `json:"measure_categories,omitempty"`
	StableCount                 int                 `json:"stable_count,omitempty"`
	RemediationOpen             bool                `json:"remediation_open,omitempty"`
	RemediationCount            int                 `json:"remediation_count,omitempty"`
	StrictWorkflow              bool                `json:"strict_workflow,omitempty"`
	LastMeasureAt               *time.Time          `json:"last_measure_at,omitempty"`
	LastRetestAt                *time.Time          `json:"last_retest_at,omitempty"`
	LastReviewMeasureCount      int                 `json:"last_review_measure_count,omitempty"`
	LastReviewRetestCount       int                 `json:"last_review_retest_count,omitempty"`
	ArchiveConsistency          string              `json:"archive_consistency,omitempty"`
	RetestTrend                 string              `json:"retest_trend,omitempty"`
	FluctuationRisk             bool                `json:"fluctuation_risk,omitempty"`
	ReceiveDeadline             *time.Time          `json:"receive_deadline,omitempty"`
	ReceiveStatus               string              `json:"receive_status,omitempty"`
	ReceiveRemainingSeconds     int64               `json:"receive_remaining_seconds,omitempty"`
	ReceiveOverdueSeconds       int64               `json:"receive_overdue_seconds,omitempty"`
	ReceiveReason               string              `json:"receive_reason,omitempty"`
	RemediationDeadline         *time.Time          `json:"remediation_deadline,omitempty"`
	RemediationStatus           string              `json:"remediation_status,omitempty"`
	RemediationRemainingSeconds int64               `json:"remediation_remaining_seconds,omitempty"`
	RemediationOverdueSeconds   int64               `json:"remediation_overdue_seconds,omitempty"`
	RemediationRevision         int                 `json:"remediation_revision,omitempty"`
}

type Timeline struct {
	ID                  string    `json:"id"`
	IncidentID          string    `json:"incident_id"`
	Action              string    `json:"action"`
	Actor               string    `json:"actor"`
	Summary             string    `json:"summary"`
	Evidence            []string  `json:"evidence,omitempty"`
	At                  time.Time `json:"at"`
	RequestKey          string    `json:"request_key,omitempty"`
	BatchID             string    `json:"batch_id,omitempty"`
	Category            string    `json:"category,omitempty"`
	OriginalReading     *float64  `json:"original_reading,omitempty"`
	Reading             *float64  `json:"reading,omitempty"`
	Discovery           string    `json:"discovery,omitempty"`
	PreviousMeasures    []string  `json:"previous_measures,omitempty"`
	PreviousAttachments []string  `json:"previous_attachments,omitempty"`
	PreviousOperator    string    `json:"previous_operator,omitempty"`
	Reason              string    `json:"reason,omitempty"`
}

type snapshot struct {
	Version     int               `json:"version"`
	Incidents   []Incident        `json:"incidents"`
	Tasks       []Task            `json:"tasks"`
	Timeline    []Timeline        `json:"timeline"`
	Idempotency map[string]string `json:"idempotency,omitempty"`
}

type Store struct {
	mu   sync.RWMutex
	dir  string
	data snapshot
}

func New(dir string) (*Store, error) {
	s := &Store{dir: dir, data: snapshot{Version: snapshotVersion, Incidents: []Incident{}, Tasks: []Task{}, Timeline: []Timeline{}, Idempotency: map[string]string{}}}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(snapshotPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("快照损坏: %w", err)
	}
	if !supportedSnapshotVersion(s.data.Version) {
		return nil, fmt.Errorf("不支持的快照版本: %d", s.data.Version)
	}
	if s.data.Idempotency == nil {
		s.data.Idempotency = map[string]string{}
	}
	normalizeSnapshot(&s.data)
	return s, nil
}

func (s *Store) Idempotent(key, result string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data.Idempotency[key]
	return v, ok
}
func (s *Store) SetIdempotent(key, result string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Idempotency == nil {
		s.data.Idempotency = map[string]string{}
	}
	s.data.Idempotency[key] = result
	_ = s.persistLocked()
}

func (s *Store) persistLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := snapshotTempPath(s.dir)
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, snapshotPath(s.dir)); err != nil {
		return err
	}
	return nil
}
func (s *Store) appendEventLocked(t Timeline) error {
	f, err := os.OpenFile(eventsPath(s.dir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(t)
	_, err = f.Write(append(b, '\n'))
	return err
}
func (s *Store) SaveIncident(i Incident, t Timeline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Incidents = append(s.data.Incidents, i)
	s.data.Timeline = append(s.data.Timeline, t)
	if err := s.appendEventLocked(t); err != nil {
		return err
	}
	return s.persistLocked()
}

// SaveIncidents 原子保存一批异常及其时间线；校验全部完成后才会修改快照。
func (s *Store) SaveIncidents(incidents []Incident, timelines []Timeline) error {
	if len(incidents) != len(timelines) || len(incidents) == 0 {
		return errors.New("批次数据为空或长度不一致")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Incidents = append(s.data.Incidents, incidents...)
	s.data.Timeline = append(s.data.Timeline, timelines...)
	for _, t := range timelines {
		if err := s.appendEventLocked(t); err != nil {
			return err
		}
	}
	return s.persistLocked()
}
func (s *Store) UpdateIncident(i Incident, t Timeline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for n := range s.data.Incidents {
		if s.data.Incidents[n].ID == i.ID {
			s.data.Incidents[n] = i
			found = true
			break
		}
	}
	if !found {
		return ErrIncidentNotFound
	}
	s.data.Timeline = append(s.data.Timeline, t)
	if err := s.appendEventLocked(t); err != nil {
		return err
	}
	return s.persistLocked()
}
func (s *Store) SaveTask(task Task, t Timeline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Tasks = append(s.data.Tasks, task)
	s.data.Timeline = append(s.data.Timeline, t)
	if err := s.appendEventLocked(t); err != nil {
		return err
	}
	return s.persistLocked()
}
func (s *Store) UpdateTask(task Task, t Timeline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for n := range s.data.Tasks {
		if s.data.Tasks[n].ID == task.ID {
			s.data.Tasks[n] = task
			s.data.Timeline = append(s.data.Timeline, t)
			if err := s.appendEventLocked(t); err != nil {
				return err
			}
			return s.persistLocked()
		}
	}
	return ErrTaskNotFound
}
func (s *Store) Incident(id string) (Incident, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, i := range s.data.Incidents {
		if i.ID == id {
			return i, true
		}
	}
	return Incident{}, false
}
func (s *Store) IncidentByArchive(id string) (Incident, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, i := range s.data.Incidents {
		if i.ArchiveID == id {
			return i, true
		}
	}
	return Incident{}, false
}
func (s *Store) Incidents() []Incident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]Incident(nil), s.data.Incidents...)
	return out
}

func (s *Store) IncidentsByBatch(batchID string) []Incident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Incident
	for _, i := range s.data.Incidents {
		if i.BatchID == batchID {
			out = append(out, i)
		}
	}
	return out
}
func (s *Store) Task(id string) (Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.data.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}
func (s *Store) TaskForIncident(id string) (Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.data.Tasks {
		if t.IncidentID == id {
			return t, true
		}
	}
	return Task{}, false
}
func (s *Store) Timelines(id string) []Timeline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Timeline
	for _, t := range s.data.Timeline {
		if t.IncidentID == id {
			out = append(out, t)
		}
	}
	return out
}

func (s *Store) AllTimelines() []Timeline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Timeline(nil), s.data.Timeline...)
}

func (s *Store) Tasks() []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]Task(nil), s.data.Tasks...)
	for n := range out {
		out[n].Measures = cloneStrings(out[n].Measures)
		out[n].Attachments = cloneStrings(out[n].Attachments)
	}
	return out
}

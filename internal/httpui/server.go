package httpui

import (
	"encoding/json"
	"museum-showcase/internal/cases"
	"museum-showcase/internal/dispatch"
	"museum-showcase/internal/storage"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	cases    *cases.Service
	dispatch *dispatch.Service
	mux      *http.ServeMux
}

func New(c *cases.Service, d *dispatch.Service) *Server {
	s := &Server{cases: c, dispatch: d, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.index)
	s.mux.HandleFunc("GET /static/style.css", s.style)
	s.mux.HandleFunc("GET /static/app.js", s.script)
	s.mux.HandleFunc("GET "+incidentsPath, s.listIncidents)
	s.mux.HandleFunc("GET /api/incidents/queue", s.incidentQueue)
	s.mux.HandleFunc("POST "+incidentsPath, s.createIncident)
	s.mux.HandleFunc("GET /api/assignees/load", s.assigneeLoad)
	s.mux.HandleFunc("GET /api/assignees/candidates", s.assigneeCandidates)
	s.mux.HandleFunc("GET /api/batches/", s.batchDetail)
	s.mux.HandleFunc("GET /api/incidents/batch/", s.batchDetailAlias)
	s.mux.HandleFunc("GET /api/archives/", s.archiveDetail)
	s.mux.HandleFunc("GET /api/incidents/archive/", s.archiveDetailAlias)
	s.mux.HandleFunc("GET /api/incidents/", s.incidentDetail)
	s.mux.HandleFunc("POST /api/incidents/dispatch", s.dispatchIncident)
	s.mux.HandleFunc("POST "+tasksPath+"/receive", s.receiveTask)
	s.mux.HandleFunc("POST "+tasksPath+"/transfer", s.transferTask)
	s.mux.HandleFunc("POST "+tasksPath+"/action", s.actionTask)
	s.mux.HandleFunc("POST "+tasksPath+"/action/revise", s.reviseTask)
	s.mux.HandleFunc("POST "+tasksPath+"/verify", s.verifyTask)
	s.mux.HandleFunc("POST "+tasksPath+"/review", s.reviewTask)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if err != nil && (err.Error() == "异常不存在" || err.Error() == "任务不存在" || err.Error() == "批次不存在" || err.Error() == "归档编号不存在") {
		status = http.StatusNotFound
	}
	if err != nil && (strings.Contains(err.Error(), "修订号冲突") || strings.Contains(err.Error(), "负载已满")) {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func decode(r *http.Request, d any) error {
	if err := requireIdempotencyKey(r); err != nil {
		return err
	}
	return json.NewDecoder(r.Body).Decode(d)
}
func actor(r *http.Request) string {
	if v := r.Header.Get("X-Operator"); v != "" {
		return v
	}
	return "值班员"
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	const page = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>文物展柜环境异常处置台</title><link rel="stylesheet" href="/static/style.css"></head><body><header><h1>文物展柜环境异常处置台</h1><span>异常登记、处置、复核与归档</span></header><main><section class="panel"><h2>登记环境异常</h2><p>可填写单条；批量登记时在“批次 JSON”填入 records 数组。</p><form id="incident-form"><input name="showcase" placeholder="展柜编号"><input name="artifact" placeholder="文物编号"><input name="metric" placeholder="指标，如相对湿度"><input name="reading" type="number" step="0.1" placeholder="当前读数"><input name="lower" type="number" step="0.1" placeholder="允许下限"><input name="upper" type="number" step="0.1" placeholder="允许上限"><select name="sensitivity"><option>高</option><option selected>中</option><option>低</option></select><textarea name="discovery" placeholder="发现说明"></textarea><textarea name="batch" placeholder='批次 JSON，例如 [{"showcase":"S1","artifact":"A1","metric":"湿度","reading":80,"lower":40,"upper":60,"discovery":"巡检","sensitivity":"中"}]'></textarea><button>登记异常</button></form><div class="actions"><input id="batch-query" placeholder="BATCH-批次编号"><button id="batch-search">查询批次</button></div><pre id="batch-result"></pre></section><section><div class="section-head"><h2>异常列表</h2><button id="refresh">刷新</button></div><div id="incidents"></div></section><section id="detail" class="panel hidden"></section></main><script src="/static/app.js"></script></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}
func (s *Server) style(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(`:root{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;color:#23313d;background:#f4f6f5}*{box-sizing:border-box}body{margin:0}header{background:#184d4d;color:#fff;padding:22px max(20px,calc((100% - 1120px)/2));display:flex;align-items:baseline;gap:20px}header h1{margin:0;font-size:24px}header span{opacity:.8}main{max-width:1120px;margin:24px auto;padding:0 16px;display:grid;gap:20px}.panel,article{background:#fff;border:1px solid #d8e0dd;border-radius:8px;padding:20px;box-shadow:0 2px 8px #173b3b10}form{display:grid;grid-template-columns:repeat(3,1fr);gap:12px}input,select,textarea,button{font:inherit;padding:10px;border:1px solid #b9c8c3;border-radius:5px}textarea{grid-column:1/-1;min-height:74px}button{background:#1c6b62;color:#fff;border:0;cursor:pointer}button.secondary{background:#e8efed;color:#184d4d}.section-head{display:flex;justify-content:space-between;align-items:center}.card-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:12px}article h3{margin:0 0 10px;color:#184d4d}article p{margin:6px 0}.tag{display:inline-block;padding:3px 8px;border-radius:12px;background:#e5f2ef;color:#156050;font-size:12px}.risk-高{background:#f8ded9;color:#a13728}.risk-中{background:#fff0c9;color:#89600b}.risk-低{background:#e5f2ef}.hidden{display:none}.timeline{border-left:3px solid #b5d7ce;padding-left:12px;margin:10px 0}.actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}@media(max-width:700px){form{grid-template-columns:1fr}header{display:block}header span{display:block;margin-top:6px}}
`))
}
func (s *Server) script(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	js := `const $=s=>document.querySelector(s);async function api(u,o={}){o.headers={"Content-Type":"application/json","Idempotency-Key":crypto.randomUUID(),...(o.headers||{})};const r=await fetch(u,o),d=await r.json();if(!r.ok)throw Error(d.error||"请求失败");return d}function esc(s){return String(s||"").replace(/[&<>\"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]))}async function load(){const d=await api("/api/incidents"),items=d.incidents||d;$("#incidents").innerHTML='<div class="card-grid">'+items.map(i=>{const overdue=i.status!=="已关闭"&&Date.now()>Date.parse(i.deadline);return '<article><h3>'+esc(i.showcase_id)+' · '+esc(i.metric)+'</h3><p>文物：'+esc(i.artifact_id)+'</p><p>原始读数：'+i.original_reading+'；当前读数：'+i.reading+'（'+i.lower+'～'+i.upper+'）</p><p>'+esc(i.risk_explain||"")+'</p><p>截止：'+new Date(i.deadline).toLocaleString()+(overdue?' <span class="tag risk-高">已逾期</span>':'')+'</p><p><span class="tag">'+i.risk+'风险</span> <span class="tag">'+i.status+'</span></p><button onclick="detail(\''+i.id+'\')">查看与处置</button></article>'}).join('')+'</div>'}async function detail(id){const d=await api('/api/incidents/'+id),i=d.incident,t=d.task;let h='<h2>'+esc(i.id)+' 详情</h2><p>原始发现：'+esc(i.original_discovery||i.discovery)+'；当前说明：'+esc(i.discovery)+'</p><p>原始读数：'+i.original_reading+'；当前读数：'+i.reading+'</p><p>状态：'+i.status+' 修订号：'+i.revision+'；'+esc(i.risk_explain||'')+'；'+esc(d.deadline_status||'')+'</p>';if(!t)h+='<div class="actions"><input id="assignee" placeholder="技术员姓名"><button onclick="assign(\''+i.id+'\','+i.revision+')">分派任务</button></div>';else{h+='<p>负责人：'+esc(t.assignee)+'　审批：'+(t.approval||'待处理')+'</p>';if(!t.received_at)h+='<div class="actions"><button onclick="receive(\''+t.id+'\','+t.revision+')">确认接收</button></div>';if(i.status==='已分派'||i.status==='处置中')h+='<div class="actions"><input id="operator" placeholder="操作人"><input id="note" placeholder="措施说明"><input id="measures" placeholder="措施，逗号分隔"><input id="attachments" placeholder="附件索引，逗号分隔"><button onclick="act(\''+t.id+'\')">保存现场处置</button></div>';if(i.status==='处置中')h+='<div class="actions"><input id="retest" type="number" placeholder="复测读数"><button onclick="verify(\''+t.id+'\')">提交复测</button></div>';if(i.status==='待复核')h+='<div class="actions"><input id="review" placeholder="复核意见"><button onclick="review(\''+t.id+'\',\'批准\')">批准关闭</button><button onclick="review(\''+t.id+'\',\'退回\')">退回处置</button></div>'}h+='<h3>时间线</h3>'+d.timeline.map(x=>'<div class="timeline"><b>'+esc(x.action)+'</b> '+esc(x.actor)+'<br>'+esc(x.summary)+'</div>').join('');$("#detail").innerHTML=h;$("#detail").classList.remove('hidden')}async function assign(id,r){try{await api('/api/incidents/dispatch',{method:'POST',body:JSON.stringify({incident_id:id,assignee:$("#assignee").value,revision:r})});await load();await detail(id)}catch(e){alert(e.message)}}async function receive(id,r){try{await api('/api/tasks/receive',{method:'POST',body:JSON.stringify({task_id:id,revision:r})});await load();await detail(id)}catch(e){alert(e.message)}}async function act(id){try{await api('/api/tasks/action',{method:'POST',body:JSON.stringify({task_id:id,operator:$("#operator").value,note:$("#note").value,measures:$("#measures").value.split(','),attachments:$("#attachments").value.split(',')})});await load();await detail(id)}catch(e){alert(e.message)}}async function verify(id){try{await api('/api/tasks/verify',{method:'POST',body:JSON.stringify({task_id:id,reading:Number($("#retest").value)})});await load();await detail(id)}catch(e){alert(e.message)}}async function review(id,d){try{await api('/api/tasks/review',{method:'POST',body:JSON.stringify({task_id:id,decision:d,note:$("#review").value||d})});await load();await detail(id)}catch(e){alert(e.message)}}$("#incident-form").onsubmit=async e=>{e.preventDefault();const o=Object.fromEntries(new FormData(e.target));["reading","lower","upper"].forEach(k=>o[k]=Number(o[k]));try{await api('/api/incidents',{method:'POST',body:JSON.stringify(o)});e.target.reset();load()}catch(x){alert(x.message)}};$("#refresh").onclick=load;load();`
	js += `;(()=>{const f=$("#incident-form");if(f)f.onsubmit=async e=>{e.preventDefault();const o=Object.fromEntries(new FormData(f));let payload=o;if(o.batch&&o.batch.trim()){try{payload={records:JSON.parse(o.batch)}}catch(_){alert("批次 JSON 格式错误");return}}else{["reading","lower","upper"].forEach(k=>payload[k]=Number(payload[k]));delete payload.batch}try{await api("/api/incidents",{method:"POST",body:JSON.stringify(payload)});f.reset();load()}catch(x){alert(x.message)}};window.act=async id=>{try{await api("/api/tasks/action",{method:"POST",body:JSON.stringify({task_id:id,operator:$("#operator").value,note:$("#note").value,categories:{"隔离":[$("#measures").value.split(",")[0]],"设备调整":[$("#measures").value.split(",")[1]||"调整设备"],"保护措施":[$("#measures").value.split(",")[2]||"保护文物"]},attachments:$("#attachments").value.split(",")})});await load();await detail(id)}catch(e){alert(e.message)}}})();`
	js += `;(()=>{const b=$("#batch-search");if(b)b.onclick=async()=>{try{const d=await api("/api/batches/"+encodeURIComponent($("#batch-query").value.trim()));$("#batch-result").textContent=JSON.stringify(d,null,2)}catch(e){alert(e.message)}}})();`
	js += `;(()=>{const old=window.detail;window.detail=async id=>{await old(id);try{const d=await api("/api/incidents/"+id),i=d.incident,t=d.task;if(i.escalation_suggested){const p=document.createElement("p");p.className="tag risk-高";p.textContent="持续恶化："+(i.escalation_reason||"建议升级处置");$("#detail").prepend(p)}if(t&&t.receive_status&&t.receive_status!=="已接收"){const p=document.createElement("p");p.className="tag risk-中";p.textContent=t.receive_status+"："+(t.receive_reason||"");$("#detail").prepend(p)}if(t&&t.fluctuation_risk){const p=document.createElement("p");p.className="tag risk-高";p.textContent="复测波动风险：需继续复测";$("#detail").prepend(p)}}catch(_){}}})();`
	_, _ = w.Write([]byte(js))
}
func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	valid := map[string]bool{"新建": true, "已分派": true, "处置中": true, "待复核": true, "已关闭": true}
	status := q.Get("status")
	if status != "" && !valid[status] {
		writeJSON(w, 400, map[string]string{"error": "非法状态参数"})
		return
	}
	risk := q.Get("risk")
	if risk != "" && risk != "高" && risk != "中" && risk != "低" {
		writeJSON(w, 400, map[string]string{"error": "非法风险参数"})
		return
	}
	assignee := strings.TrimSpace(q.Get("assignee"))
	priority := q.Get("priority")
	priorityMode := priority != ""
	if priorityMode && priority != "true" && priority != "1" && priority != "false" && priority != "0" {
		writeJSON(w, 400, map[string]string{"error": "priority 参数应为 true、false、1 或 0"})
		return
	}
	if priorityMode && assignee != "" && !s.knownAssignee(assignee) {
		writeJSON(w, 400, map[string]string{"error": "负责人不存在"})
		return
	}
	parse := func(v string) (*time.Time, error) {
		if v == "" {
			return nil, nil
		}
		t, e := time.Parse(time.RFC3339, v)
		if e != nil {
			t, e = time.Parse("2006-01-02", v)
		}
		return &t, e
	}
	from, e := parse(q.Get("from"))
	if e != nil {
		writeJSON(w, 400, map[string]string{"error": "from 时间格式应为 RFC3339"})
		return
	}
	to, e := parse(q.Get("to"))
	if e != nil {
		writeJSON(w, 400, map[string]string{"error": "to 时间格式应为 RFC3339"})
		return
	}
	if v := q.Get("to"); len(v) == len("2006-01-02") {
		end := to.Add(24*time.Hour - time.Nanosecond)
		to = &end
	}
	if from != nil && to != nil && from.After(*to) {
		writeJSON(w, 400, map[string]string{"error": "from 不能晚于 to"})
		return
	}
	showcase := strings.TrimSpace(q.Get("showcase"))
	if len([]rune(showcase)) > 100 {
		writeJSON(w, 400, map[string]string{"error": "showcase 参数过长"})
		return
	}
	for _, r := range showcase {
		if r < 0x20 || r == 0x7f {
			writeJSON(w, 400, map[string]string{"error": "showcase 参数包含控制字符"})
			return
		}
	}
	rsl := s.cases.ListFiltered(cases.ListFilter{Status: status, Risk: risk, Showcase: showcase, Assignee: assignee, From: from, To: to, Priority: priorityMode && priority != "false" && priority != "0"})
	writeJSON(w, 200, rsl)
}

func (s *Server) incidentQueue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status, assignee := q.Get("status"), strings.TrimSpace(q.Get("assignee"))
	risk, showcase := q.Get("risk"), strings.TrimSpace(q.Get("showcase"))
	valid := map[string]bool{"新建": true, "已分派": true, "处置中": true, "待复核": true}
	if status != "" && !valid[status] {
		writeJSON(w, 400, map[string]string{"error": "非法状态参数"})
		return
	}
	if risk != "" && risk != "高" && risk != "中" && risk != "低" {
		writeJSON(w, 400, map[string]string{"error": "非法风险参数"})
		return
	}
	if len([]rune(assignee)) > 100 || strings.ContainsAny(assignee, "\r\n") {
		writeJSON(w, 400, map[string]string{"error": "负责人参数格式非法"})
		return
	}
	if assignee != "" && !s.knownAssignee(assignee) {
		writeJSON(w, 400, map[string]string{"error": "负责人不存在"})
		return
	}
	writeJSON(w, 200, s.cases.Queue(cases.QueueFilter{Status: status, Assignee: assignee, Risk: risk, Showcase: showcase}))
}

func (s *Server) knownAssignee(name string) bool {
	for _, candidate := range s.dispatch.Candidates() {
		if candidate.Assignee == name {
			return true
		}
	}
	return false
}
func (s *Server) createIncident(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Correction       bool    `json:"correction"`
		IsCorrection     bool    `json:"is_correction"`
		IncidentID       string  `json:"incident_id"`
		Revision         int     `json:"revision"`
		CorrectionReason string  `json:"correction_reason"`
		Showcase         string  `json:"showcase"`
		Artifact         string  `json:"artifact"`
		Metric           string  `json:"metric"`
		Reading          float64 `json:"reading"`
		Lower            float64 `json:"lower"`
		Upper            float64 `json:"upper"`
		Discovery        string  `json:"discovery"`
		Sensitivity      string  `json:"sensitivity"`
		Records          []struct {
			Showcase    string  `json:"showcase"`
			Artifact    string  `json:"artifact"`
			Metric      string  `json:"metric"`
			Reading     float64 `json:"reading"`
			Lower       float64 `json:"lower"`
			Upper       float64 `json:"upper"`
			Discovery   string  `json:"discovery"`
			Sensitivity string  `json:"sensitivity"`
		} `json:"records"`
		Items []struct {
			Showcase    string  `json:"showcase"`
			Artifact    string  `json:"artifact"`
			Metric      string  `json:"metric"`
			Reading     float64 `json:"reading"`
			Lower       float64 `json:"lower"`
			Upper       float64 `json:"upper"`
			Discovery   string  `json:"discovery"`
			Sensitivity string  `json:"sensitivity"`
		} `json:"items"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if in.Correction || in.IsCorrection {
		i, err := s.cases.Correct(in.IncidentID, in.Reading, in.Lower, in.Upper, in.Discovery, in.CorrectionReason, actor(r), in.Revision, key)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, i)
		return
	}
	if len(in.Records) == 0 && len(in.Items) > 0 {
		in.Records = in.Items
	}
	if len(in.Records) > 0 {
		recs := make([]cases.IncidentInput, len(in.Records))
		for n, v := range in.Records {
			recs[n] = cases.IncidentInput{Showcase: v.Showcase, Artifact: v.Artifact, Metric: v.Metric, Reading: v.Reading, Lower: v.Lower, Upper: v.Upper, Discovery: v.Discovery, Sensitivity: v.Sensitivity}
		}
		result, err := s.cases.CreateBatch(recs, actor(r), key)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, result)
		return
	}
	i, err := s.cases.CreateWithKey(in.Showcase, in.Artifact, in.Metric, in.Reading, in.Lower, in.Upper, in.Discovery, in.Sensitivity, actor(r), key)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusCreated
	if i.RelatedIncidentID != "" || len(i.ReadingChanges) > 0 {
		status = http.StatusOK
	}
	writeJSON(w, status, i)
}

func (s *Server) assigneeLoad(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("assignee"))
	if name == "" {
		writeJSON(w, 400, map[string]string{"error": "assignee不能为空"})
		return
	}
	writeJSON(w, 200, s.dispatch.Load(name))
}
func (s *Server) assigneeCandidates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.dispatch.Candidates())
}
func (s *Server) batchDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/batches/")
	if len(id) > 80 || strings.ContainsAny(id, "/\r\n") {
		writeJSON(w, 400, map[string]string{"error": "batch_id格式非法"})
		return
	}
	result, err := s.cases.Batch(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) archiveDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/archives/")
	if !strings.HasPrefix(id, "ARC-") {
		writeJSON(w, 400, map[string]string{"error": "归档编号格式非法"})
		return
	}
	result, err := s.cases.Archive(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) batchDetailAlias(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.Replace(r.URL.Path, "/api/incidents/batch/", "/api/batches/", 1)
	s.batchDetail(w, r)
}
func (s *Server) archiveDetailAlias(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.Replace(r.URL.Path, "/api/incidents/archive/", "/api/archives/", 1)
	s.archiveDetail(w, r)
}
func (s *Server) incidentDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/incidents/")
	lookupID := id
	i, t, err := s.cases.Get(id)
	if err != nil {
		if archived, ok := s.cases.ByArchive(id); ok {
			i, t, err = s.cases.Get(archived.ID)
			lookupID = archived.ID
		} else {
			msg := err.Error()
			if strings.HasPrefix(id, "ARC-") {
				msg = "归档编号不存在"
			}
			writeJSON(w, 404, map[string]string{"error": msg})
			return
		}
	}
	task, ok := s.dispatch.TaskForIncident(lookupID)
	var tp any
	if ok {
		tp = task
	}
	deadlineStatus, remaining, overdue := s.cases.DeadlineInfo(i)
	deadlineText := deadlineStatus
	if remaining > 0 {
		deadlineText = deadlineStatus + "，剩余 " + remaining.Round(time.Minute).String()
	}
	if overdue > 0 {
		deadlineText = deadlineStatus + "，逾期 " + overdue.Round(time.Minute).String()
	}
	var summary any
	if ok {
		trend := "无"
		if len(task.Retests) >= 2 {
			prev, cur := task.Retests[len(task.Retests)-2], task.Retests[len(task.Retests)-1]
			switch {
			case cur.Difference < prev.Difference:
				trend = "改善"
			case cur.Difference > prev.Difference:
				trend = "恶化"
			default:
				trend = "持平"
			}
		}
		var latest any
		if task.Retest != nil {
			latest = *task.Retest
		}
		summary = map[string]any{"assignee": task.Assignee, "received_at": task.ReceivedAt, "task_revision": task.Revision, "measures_count": len(task.Measures), "attachments_count": len(task.Attachments), "updated_at": task.UpdatedAt, "latest_reading": latest, "retest_count": len(task.Retests), "deviation_trend": trend, "retest_trend": task.RetestTrend, "fluctuation_risk": task.FluctuationRisk, "receive_status": task.ReceiveStatus, "receive_reason": task.ReceiveReason, "receive_remaining_seconds": task.ReceiveRemainingSeconds, "receive_overdue_seconds": task.ReceiveOverdueSeconds, "remediation_status": task.RemediationStatus, "remediation_remaining_seconds": task.RemediationRemainingSeconds, "remediation_overdue_seconds": task.RemediationOverdueSeconds}
	}
	writeJSON(w, 200, map[string]any{"incident": i, "timeline": t, "task": tp, "task_summary": summary, "deadline_status": deadlineText, "remaining_seconds": int64(remaining.Seconds()), "overdue_seconds": int64(overdue.Seconds()), "archive_id": i.ArchiveID})
}
func (s *Server) dispatchIncident(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IncidentID string `json:"incident_id"`
		Assignee   string `json:"assignee"`
		Reason     string `json:"reason"`
		Revision   int    `json:"revision"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	var t storage.Task
	var err error
	if existing, exists := s.dispatch.TaskForIncident(in.IncidentID); exists {
		if in.Reason == "" {
			writeJSON(w, 400, map[string]string{"error": "已有任务，请提供转派原因"})
			return
		}
		t, err = s.dispatch.Transfer(existing.ID, in.Assignee, in.Reason, actor(r), in.Revision, key)
	} else {
		t, err = s.dispatch.AssignPending(in.IncidentID, in.Assignee, actor(r), in.Revision, key)
	}
	if err != nil {
		if strings.Contains(err.Error(), "负载已满") {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "load": s.dispatch.Load(in.Assignee)})
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) receiveTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskID   string `json:"task_id"`
		Revision int    `json:"revision"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	t, err := s.dispatch.Receive(in.TaskID, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) transferTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskID   string `json:"task_id"`
		Assignee string `json:"assignee"`
		Reason   string `json:"reason"`
		Revision int    `json:"revision"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	t, err := s.dispatch.Transfer(in.TaskID, in.Assignee, in.Reason, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, t)
}
func (s *Server) actionTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskID      string              `json:"task_id"`
		Action      string              `json:"action"`
		Revise      bool                `json:"revise"`
		Reason      string              `json:"reason"`
		Operator    string              `json:"operator"`
		Note        string              `json:"note"`
		Measures    []string            `json:"measures"`
		Attachments []string            `json:"attachments"`
		Categories  map[string][]string `json:"categories"`
		Revision    int                 `json:"revision"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if in.Revise || in.Action == "revise" || in.Action == "修订" {
		t, err := s.dispatch.ReviseAction(in.TaskID, in.Reason, in.Operator, in.Note, in.Categories, in.Attachments, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, 200, t)
		return
	}
	var t storage.Task
	var err error
	if current, ok := s.dispatch.Task(in.TaskID); ok && !current.StrictWorkflow && in.Categories == nil {
		t, err = s.dispatch.ActWithKey(in.TaskID, in.Operator, in.Note, in.Measures, in.Attachments, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
	} else {
		t, err = s.dispatch.ActWithCategories(in.TaskID, in.Operator, in.Note, in.Categories, in.Attachments, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, t)
}
func (s *Server) verifyTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskID     string  `json:"task_id"`
		Reading    float64 `json:"reading"`
		Revision   int     `json:"revision"`
		MeasuredAt string  `json:"measured_at"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	var measuredAt *time.Time
	if in.MeasuredAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, in.MeasuredAt)
		if parseErr != nil {
			writeJSON(w, 400, map[string]string{"error": "measured_at 必须是 RFC3339"})
			return
		}
		measuredAt = &parsed
	}
	t, err := s.dispatch.VerifyWithMeasuredAt(in.TaskID, in.Reading, measuredAt, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) reviseTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskID      string              `json:"task_id"`
		Revision    int                 `json:"revision"`
		Reason      string              `json:"reason"`
		Operator    string              `json:"operator"`
		Note        string              `json:"note"`
		Categories  map[string][]string `json:"categories"`
		Attachments []string            `json:"attachments"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	t, err := s.dispatch.ReviseAction(in.TaskID, in.Reason, in.Operator, in.Note, in.Categories, in.Attachments, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, t)
}
func (s *Server) reviewTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskID              string `json:"task_id"`
		Decision            string `json:"decision"`
		Note                string `json:"note"`
		Revision            int    `json:"revision"`
		RemediationDeadline string `json:"remediation_deadline"`
	}
	if err := decode(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if in.RemediationDeadline != "" {
		if task, ok := s.dispatch.Task(in.TaskID); ok && task.RemediationDeadline != nil {
			provided, err := time.Parse(time.RFC3339, in.RemediationDeadline)
			if err != nil || !provided.Equal(*task.RemediationDeadline) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "整改期限字段不一致，请刷新后重试"})
				return
			}
		}
	}
	t, err := s.dispatch.ReviewWithOptions(in.TaskID, in.Decision, in.Note, actor(r), in.Revision, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, t)
}

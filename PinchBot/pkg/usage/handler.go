package usage

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// DayStats is aggregate stats for one day.
type DayStats struct {
	Date             string `json:"date"`
	Requests         int    `json:"requests"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

// TaskSummary is a single LLM call record for the recent list.
type TaskSummary struct {
	Time             time.Time `json:"time"`
	SessionKey       string    `json:"session_key"`
	Channel          string    `json:"channel"`
	Source           string    `json:"source"`
	Model            string    `json:"model"`
	TotalTokens      int       `json:"total_tokens"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	Iteration        int       `json:"iteration"`
	Prompt           string    `json:"prompt,omitempty"`
	Completion       string    `json:"completion,omitempty"`
}

// StatsResponse is the JSON response for /usage.
type StatsResponse struct {
	ByDay   map[string]DayStats `json:"by_day"`
	Recent  []TaskSummary      `json:"recent"`
	Summary DayStats           `json:"summary"` // today or all-time
}

// NewHandler returns an http.Handler that serves /usage (JSON) and /dashboard (HTML).
func NewHandler(workspace string) http.Handler {
	mux := http.NewServeMux()
	usagePath := filepath.Join(workspace, "usage.jsonl")

	mux.HandleFunc("/usage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := readAndAggregate(usagePath)
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(dashboardHTML)
	})

	return mux
}

func readAndAggregate(usagePath string) StatsResponse {
	resp := StatsResponse{
		ByDay:  make(map[string]DayStats),
		Recent: []TaskSummary{},
	}
	f, err := os.Open(usagePath)
	if err != nil {
		return resp
	}
	defer f.Close()

	var allRecords []Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		allRecords = append(allRecords, r)
		day := r.Time.Format("2006-01-02")
		d := resp.ByDay[day]
		d.Date = day
		d.Requests++
		d.PromptTokens += r.PromptTokens
		d.CompletionTokens += r.CompletionTokens
		d.TotalTokens += r.TotalTokens
		resp.ByDay[day] = d
	}

	// Recent = last 100 LLM calls (newest first)
	sort.Slice(allRecords, func(i, j int) bool { return allRecords[i].Time.After(allRecords[j].Time) })
	n := 100
	if len(allRecords) < n {
		n = len(allRecords)
	}
	for i := 0; i < n; i++ {
		r := allRecords[i]
		resp.Recent = append(resp.Recent, TaskSummary{
			Time:             r.Time,
			SessionKey:       r.SessionKey,
			Channel:          r.Channel,
			Source:           r.Source,
			Model:            r.Model,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
			Iteration:        r.Iteration,
			Prompt:           r.Prompt,
			Completion:       r.Completion,
		})
	}

	// Summary = today or all-time
	today := time.Now().Format("2006-01-02")
	if d, ok := resp.ByDay[today]; ok {
		resp.Summary = d
		resp.Summary.Date = today
	} else {
		for _, d := range resp.ByDay {
			resp.Summary.Requests += d.Requests
			resp.Summary.PromptTokens += d.PromptTokens
			resp.Summary.CompletionTokens += d.CompletionTokens
			resp.Summary.TotalTokens += d.TotalTokens
		}
		resp.Summary.Date = "all"
	}
	return resp
}

var dashboardHTML = []byte(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>PinchBot 用量看板</title>
  <style>
    * { box-sizing: border-box; }
    body { font-family: system-ui, "Segoe UI", sans-serif; margin: 0; padding: 16px; background: #0f0f12; color: #e4e4e7; }
    h1 { font-size: 1.5rem; margin-bottom: 16px; }
    .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; margin-bottom: 24px; }
    .card { background: #18181b; border-radius: 8px; padding: 16px; border: 1px solid #27272a; }
    .card .label { font-size: 0.75rem; color: #71717a; text-transform: uppercase; margin-bottom: 4px; }
    .card .value { font-size: 1.25rem; font-weight: 600; color: #a5f3fc; }
    table { width: 100%; border-collapse: collapse; background: #18181b; border-radius: 8px; overflow: hidden; border: 1px solid #27272a; }
    th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #27272a; }
    th { background: #27272a; font-size: 0.75rem; color: #71717a; text-transform: uppercase; }
    tr:last-child td { border-bottom: 0; }
    .meta { font-size: 0.8rem; color: #71717a; margin-bottom: 16px; }
    a { color: #a5f3fc; }
    .toggler { cursor: pointer; user-select: none; display: inline-block; width: 1.2em; color: #71717a; }
    .toggler:hover { color: #a5f3fc; }
    .detail-row td { background: #1c1c1f; border-bottom: 1px solid #27272a; padding: 12px; vertical-align: top; }
    .detail-row .detail-inner { max-height: 400px; overflow: auto; }
    .detail-inner section { margin-bottom: 16px; }
    .detail-inner section:last-child { margin-bottom: 0; }
    .detail-inner .label { font-size: 0.7rem; color: #71717a; text-transform: uppercase; margin-bottom: 4px; }
    .detail-inner pre { margin: 0; font-size: 0.8rem; white-space: pre-wrap; word-break: break-word; background: #0f0f12; padding: 10px; border-radius: 6px; border: 1px solid #27272a; }
  </style>
</head>
<body>
  <h1>🦞 PinchBot 用量看板</h1>
  <p class="meta">每日任务与 Token 消耗 · 数据来自 <code>workspace/usage.jsonl</code></p>
  <div class="cards" id="cards"></div>
  <h2>近期任务</h2>
  <table>
    <thead><tr><th>时间</th><th>来源</th><th>通道</th><th>模型</th><th>轮数</th><th>Prompt</th><th>Completion</th><th>总 Token</th><th></th></tr></thead>
    <tbody id="table"></tbody>
  </table>
  <script>
    fetch('/usage')
      .then(r => r.json())
      .then(data => {
        const s = data.summary;
        document.getElementById('cards').innerHTML = [
          '<div class="card"><div class="label">今日请求</div><div class="value">' + (s.requests || 0) + '</div></div>',
          '<div class="card"><div class="label">今日 Prompt Tokens</div><div class="value">' + (s.prompt_tokens || 0).toLocaleString() + '</div></div>',
          '<div class="card"><div class="label">今日 Completion Tokens</div><div class="value">' + (s.completion_tokens || 0).toLocaleString() + '</div></div>',
          '<div class="card"><div class="label">今日总 Tokens</div><div class="value">' + (s.total_tokens || 0).toLocaleString() + '</div></div>'
        ].join('');
        const recent = (data.recent || []).slice(0, 50);
        const tbody = document.getElementById('table');
        if (recent.length === 0) {
          tbody.innerHTML = '<tr><td colspan="9">暂无记录</td></tr>';
        } else {
          tbody.innerHTML = '';
          recent.forEach(function(t, idx) {
            const time = new Date(t.time).toLocaleString('zh-CN');
            const tr = document.createElement('tr');
            tr.setAttribute('data-idx', idx);
            tr.innerHTML = '<td>' + time + '</td><td>' + (t.source || '-') + '</td><td>' + (t.channel || '-') + '</td><td>' + (t.model || '-') + '</td><td>' + (t.iteration || 0) + '</td><td>' + (t.prompt_tokens || 0).toLocaleString() + '</td><td>' + (t.completion_tokens || 0).toLocaleString() + '</td><td>' + (t.total_tokens || 0).toLocaleString() + '</td><td><span class="toggler" data-idx="' + idx + '" title="展开/收起">&#9654;</span></td>';
            tbody.appendChild(tr);
            const detailTr = document.createElement('tr');
            detailTr.id = 'detail-' + idx;
            detailTr.className = 'detail-row';
            detailTr.style.display = 'none';
            const td = document.createElement('td');
            td.colSpan = 9;
            const div = document.createElement('div');
            div.className = 'detail-inner';
            div.innerHTML = '<section><div class="label">输入 (发给 LLM)</div><pre></pre></section><section><div class="label">输出 (LLM 回复)</div><pre></pre></section>';
            const pres = div.querySelectorAll('pre');
            pres[0].textContent = (t.prompt && t.prompt.trim()) ? t.prompt : '(无)';
            pres[1].textContent = (t.completion && t.completion.trim()) ? t.completion : '(无)';
            td.appendChild(div);
            detailTr.appendChild(td);
            tbody.appendChild(detailTr);
          });
          tbody.addEventListener('click', function(e) {
            var el = e.target;
            if (!el.classList || !el.classList.contains('toggler')) return;
            var idx = el.getAttribute('data-idx');
            var detail = document.getElementById('detail-' + idx);
            if (!detail) return;
            var isHidden = detail.style.display === 'none';
            detail.style.display = isHidden ? 'table-row' : 'none';
            el.textContent = isHidden ? '\u25bc' : '\u9654';
          });
        }
      })
      .catch(function(e) { document.getElementById('table').innerHTML = '<tr><td colspan="9">加载失败: ' + e.message + '</td></tr>'; });
  </script>
</body>
</html>`)

package mewsync

import (
	"encoding/json"
	"net/http"
)

const webAddr = "127.0.0.1:8999"

type webStatusResponse struct {
	Source      string `json:"source"`
	SongName    string `json:"song_name"`
	SongAuthor  string `json:"song_author"`
	IsPlaying   bool   `json:"is_playing"`
	ProgressMs  uint64 `json:"progress_ms"`
	DurationMs  uint64 `json:"duration_ms"`
	CurrentLine string `json:"current_line"`
	LatencyMs   uint64 `json:"latency_ms"`
}

type webSettingsPayload struct {
	Source            string `json:"source"`
	LastfmAPIKey      string `json:"lastfm_api_key"`
	LastfmUsername    string `json:"lastfm_username"`
	ViewTimestamp     bool   `json:"view_timestamp"`
	ViewLabel         bool   `json:"view_label"`
	ViewEmoji         string `json:"view_emoji"`
	AutoClear         bool   `json:"auto_clear"`
	AdvancedEnabled   bool   `json:"advanced_enabled"`
	AdvancedEmoji     string `json:"advanced_emoji"`
	AdvancedTemplate  string `json:"advanced_template"`
	SendTimeOffsetMs  uint64 `json:"send_time_offset_ms"`
	EnableAutoOffset  bool   `json:"enable_autooffset"`
	AutoOffsetSamples int    `json:"autooffset_samples"`
	HasToken          bool   `json:"has_token"`
	Token             string `json:"token,omitempty"`
}

// NewWebServer builds the web panel's HTTP handler. configDir is where
// settings changes made through the panel are persisted.
func NewWebServer(shared *Shared, settings *SettingsBox, configDir string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(webIndexHTML))
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		snap := GetSnapshot(shared, settings)
		resp := webStatusResponse{
			Source:      snap.Source.String(),
			SongName:    snap.SongName,
			SongAuthor:  snap.SongAuthor,
			IsPlaying:   snap.IsPlaying,
			ProgressMs:  snap.ProgressMs,
			DurationMs:  snap.DurationMs,
			CurrentLine: snap.CurrentLine,
			LatencyMs:   snap.LatencyMs,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s := settings.Get()
			resp := webSettingsPayload{
				Source:            s.Source.String(),
				LastfmAPIKey:      s.LastfmAPIKey,
				LastfmUsername:    s.LastfmUsername,
				ViewTimestamp:     s.ViewTimestamp,
				ViewLabel:         s.ViewLabel,
				ViewEmoji:         s.ViewEmoji,
				AutoClear:         s.AutoClear,
				AdvancedEnabled:   s.AdvancedEnabled,
				AdvancedEmoji:     s.AdvancedEmoji,
				AdvancedTemplate:  s.AdvancedTemplate,
				SendTimeOffsetMs:  s.SendTimeOffsetMs,
				EnableAutoOffset:  s.EnableAutoOffset,
				AutoOffsetSamples: s.AutoOffsetSamples,
				HasToken:          s.Token != "",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case http.MethodPost:
			var in webSettingsPayload
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			cur := settings.Get()
			source, ok := ParseSource(in.Source)
			if !ok {
				source = cur.Source
			}
			cur.Source = source
			cur.LastfmAPIKey = in.LastfmAPIKey
			cur.LastfmUsername = in.LastfmUsername
			cur.ViewTimestamp = in.ViewTimestamp
			cur.ViewLabel = in.ViewLabel
			cur.ViewEmoji = in.ViewEmoji
			cur.AutoClear = in.AutoClear
			cur.AdvancedEnabled = in.AdvancedEnabled
			cur.AdvancedEmoji = in.AdvancedEmoji
			cur.AdvancedTemplate = in.AdvancedTemplate
			cur.SendTimeOffsetMs = in.SendTimeOffsetMs
			cur.EnableAutoOffset = in.EnableAutoOffset
			cur.AutoOffsetSamples = in.AutoOffsetSamples
			if in.Token != "" {
				if err := StoreToken(configDir, in.Token); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				cur.Token = in.Token
			}
			settings.Set(cur)
			if err := SaveSettings(configDir, cur); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		entries := GetHistory(configDir)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	})

	return mux
}

// RunWebServer blocks serving the web panel on webAddr.
func RunWebServer(shared *Shared, settings *SettingsBox, configDir string) error {
	return http.ListenAndServe(webAddr, NewWebServer(shared, settings, configDir))
}

const webIndexHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>mewsync</title>
<style>
  :root {
    --bg: #0b0e14;
    --panel: #12161f;
    --panel-2: #171c28;
    --border: #232a3a;
    --text: #e7ebf3;
    --muted: #7c869c;
    --accent: #7dd3c0;
    --accent-2: #b98ee8;
    font-family: -apple-system, "Segoe UI", Inter, sans-serif;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    min-height: 100vh;
    background:
      radial-gradient(1200px 600px at 15% -10%, rgba(125,211,192,0.10), transparent 60%),
      radial-gradient(1000px 500px at 110% 10%, rgba(185,142,232,0.10), transparent 60%),
      var(--bg);
    color: var(--text);
    display: flex;
    justify-content: center;
    padding: 3rem 1.25rem;
  }
  .shell { width: 100%; max-width: 560px; }
  .brand { display: flex; align-items: baseline; gap: 0.5rem; margin-bottom: 1.5rem; }
  .brand-mark {
    width: 10px; height: 10px; border-radius: 50%;
    background: var(--accent);
    box-shadow: 0 0 12px var(--accent);
  }
  .brand-name { font-weight: 700; letter-spacing: 0.02em; }
  .brand-tag { color: var(--muted); font-size: 0.85rem; }

  .card {
    background: linear-gradient(180deg, var(--panel-2), var(--panel));
    border: 1px solid var(--border);
    border-radius: 16px;
    padding: 1.75rem;
    box-shadow: 0 20px 40px -20px rgba(0,0,0,0.5);
  }
  .song-name { font-size: 1.35rem; font-weight: 700; }
  .song-artist { color: var(--muted); margin-top: 0.15rem; }

  .bar-track {
    margin-top: 1.25rem;
    height: 6px;
    border-radius: 999px;
    background: var(--border);
    overflow: hidden;
  }
  .bar-fill {
    height: 100%;
    border-radius: 999px;
    background: linear-gradient(90deg, var(--accent), var(--accent-2));
    width: 0%;
    transition: width 0.4s ease;
  }
  .times {
    display: flex; justify-content: space-between;
    color: var(--muted); font-size: 0.78rem; margin-top: 0.4rem;
  }

  .lyric {
    margin-top: 1.5rem;
    padding: 1rem 1.1rem;
    border-radius: 12px;
    background: rgba(125,211,192,0.06);
    border: 1px solid rgba(125,211,192,0.18);
    font-size: 1.05rem;
    min-height: 1.4em;
  }
  .lyric.empty { color: var(--muted); font-style: italic; font-size: 0.95rem; }

  .meta-row {
    display: flex; gap: 0.6rem; margin-top: 1.25rem; flex-wrap: wrap;
  }
  .pill {
    font-size: 0.78rem;
    padding: 0.3rem 0.65rem;
    border-radius: 999px;
    background: var(--panel-2);
    border: 1px solid var(--border);
    color: var(--muted);
  }
  .pill b { color: var(--text); font-weight: 600; }

  .history {
    margin-top: 1.25rem;
  }
  .history h2 {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--muted);
    margin: 0 0 0.6rem;
  }
  .history ul { list-style: none; margin: 0; padding: 0; }
  .history li {
    display: flex; justify-content: space-between; gap: 1rem;
    padding: 0.45rem 0;
    border-bottom: 1px solid var(--border);
    font-size: 0.85rem;
  }
  .history li:last-child { border-bottom: none; }
  .history .h-song { color: var(--text); }
  .history .h-artist { color: var(--muted); }

  .tabs { display: flex; gap: 0.4rem; margin-bottom: 1.1rem; }
  .tab {
    flex: 1; text-align: center; padding: 0.5rem 0;
    border-radius: 10px; border: 1px solid var(--border);
    background: var(--panel-2); color: var(--muted);
    font-size: 0.85rem; cursor: pointer; user-select: none;
  }
  .tab.active { color: var(--text); border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent) inset; }
  .view { display: none; }
  .view.active { display: block; }

  .field { margin-top: 1rem; }
  .field label {
    display: block; font-size: 0.78rem; text-transform: uppercase;
    letter-spacing: 0.06em; color: var(--muted); margin-bottom: 0.4rem;
  }
  .field input[type=text], .field input[type=password], .field input[type=number], .field select, .field textarea {
    width: 100%; padding: 0.55rem 0.7rem; border-radius: 8px;
    border: 1px solid var(--border); background: var(--panel-2);
    color: var(--text); font-size: 0.9rem; font-family: inherit;
  }
  .field textarea { min-height: 4rem; resize: vertical; }
  .field.row { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
  .field.row label { margin-bottom: 0; }
  .switch { position: relative; width: 40px; height: 22px; flex-shrink: 0; }
  .switch input { opacity: 0; width: 0; height: 0; }
  .switch .slider {
    position: absolute; inset: 0; background: var(--border);
    border-radius: 999px; transition: 0.2s; cursor: pointer;
  }
  .switch .slider::before {
    content: ""; position: absolute; width: 16px; height: 16px;
    left: 3px; top: 3px; background: var(--muted); border-radius: 50%;
    transition: 0.2s;
  }
  .switch input:checked + .slider { background: rgba(125,211,192,0.25); }
  .switch input:checked + .slider::before { transform: translateX(18px); background: var(--accent); }

  .fieldset-title {
    font-size: 0.78rem; text-transform: uppercase; letter-spacing: 0.08em;
    color: var(--muted); margin-top: 1.5rem; margin-bottom: -0.3rem;
  }
  .fieldset-title:first-child { margin-top: 0; }

  .save-row { display: flex; align-items: center; gap: 0.8rem; margin-top: 1.5rem; }
  .btn {
    padding: 0.6rem 1.2rem; border-radius: 10px; border: none;
    background: linear-gradient(90deg, var(--accent), var(--accent-2));
    color: #0b0e14; font-weight: 700; font-size: 0.9rem; cursor: pointer;
  }
  .btn:active { transform: translateY(1px); }
  .save-status { font-size: 0.82rem; color: var(--muted); }
</style>
</head>
<body>
<div class="shell">
  <div class="brand">
    <div class="brand-mark"></div>
    <div class="brand-name">mewsync</div>
    <div class="brand-tag">live status</div>
  </div>

  <div class="tabs">
    <div class="tab active" data-tab="status">Status</div>
    <div class="tab" data-tab="settings">Settings</div>
  </div>

  <div id="view-status" class="view active">
    <div class="card">
      <div id="song" class="song-name">Loading…</div>
      <div id="artist" class="song-artist"></div>

      <div class="bar-track"><div class="bar-fill" id="fill"></div></div>
      <div class="times"><span id="t-cur">0:00</span><span id="t-dur">0:00</span></div>

      <div class="lyric empty" id="line">waiting for a lyric line…</div>

      <div class="meta-row">
        <div class="pill">source: <b id="source">-</b></div>
        <div class="pill">latency: <b id="latency">-</b></div>
      </div>
    </div>

    <div class="card history">
      <h2>Recently played</h2>
      <ul id="history"></ul>
    </div>
  </div>

  <div id="view-settings" class="view">
    <div class="card">
      <div class="fieldset-title">Source</div>
      <div class="field">
        <label for="s-source">Playback source</label>
        <select id="s-source">
          <option value="spotify">Spotify</option>
          <option value="lastfm">Last.fm (also covers YouTube Music via WebScrobbler)</option>
          <option value="mpris">MPRIS (Linux desktop players)</option>
        </select>
      </div>
      <div class="field" id="lastfm-fields">
        <label for="s-lastfm-key">Last.fm API key</label>
        <input type="text" id="s-lastfm-key" placeholder="from last.fm/api/account/create">
      </div>
      <div class="field" id="lastfm-fields-2">
        <label for="s-lastfm-user">Last.fm username</label>
        <input type="text" id="s-lastfm-user">
      </div>
      <div class="field">
        <label for="s-token">Discord token <span id="token-hint"></span></label>
        <input type="password" id="s-token" placeholder="leave blank to keep the stored token">
      </div>

      <div class="fieldset-title">Status text</div>
      <div class="field row">
        <label for="s-timestamp">Show [m:ss] timestamp</label>
        <div class="switch"><input type="checkbox" id="s-timestamp"><span class="slider"></span></div>
      </div>
      <div class="field row">
        <label for="s-label">Show "Song lyrics -" label</label>
        <div class="switch"><input type="checkbox" id="s-label"><span class="slider"></span></div>
      </div>
      <div class="field row">
        <label for="s-autoclear">Clear status on song change</label>
        <div class="switch"><input type="checkbox" id="s-autoclear"><span class="slider"></span></div>
      </div>
      <div class="field">
        <label for="s-emoji">Status emoji</label>
        <input type="text" id="s-emoji" placeholder="🎵">
      </div>

      <div class="fieldset-title">Advanced template</div>
      <div class="field row">
        <label for="s-adv-enabled">Use custom template</label>
        <div class="switch"><input type="checkbox" id="s-adv-enabled"><span class="slider"></span></div>
      </div>
      <div class="field">
        <label for="s-adv-template">Template</label>
        <textarea id="s-adv-template" placeholder="[{timestamp}] [{lyrics}]"></textarea>
      </div>
      <div class="field">
        <label for="s-adv-emoji">Template emoji</label>
        <input type="text" id="s-adv-emoji">
      </div>

      <div class="fieldset-title">Timing</div>
      <div class="field row">
        <label for="s-autooffset">Learn offset from Discord latency</label>
        <div class="switch"><input type="checkbox" id="s-autooffset"><span class="slider"></span></div>
      </div>
      <div class="field">
        <label for="s-offset">Fixed send offset (ms)</label>
        <input type="number" id="s-offset" min="0" step="50">
      </div>
      <div class="field">
        <label for="s-samples">Auto-offset sample count</label>
        <input type="number" id="s-samples" min="1" step="1">
      </div>

      <div class="save-row">
        <button class="btn" id="save-btn">Save settings</button>
        <span class="save-status" id="save-status"></span>
      </div>
    </div>
  </div>
</div>

<script>
function fmtTime(ms) {
  const s = Math.floor(ms / 1000);
  return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
}

async function pollStatus() {
  try {
    const r = await fetch('/api/status');
    const d = await r.json();
    document.getElementById('song').textContent = d.song_name || 'Nothing playing';
    document.getElementById('artist').textContent = d.song_author || '';
    const pct = d.duration_ms ? Math.min(100, 100 * d.progress_ms / d.duration_ms) : 0;
    document.getElementById('fill').style.width = pct + '%';
    document.getElementById('t-cur').textContent = fmtTime(d.progress_ms || 0);
    document.getElementById('t-dur').textContent = fmtTime(d.duration_ms || 0);
    const lineEl = document.getElementById('line');
    if (d.current_line) {
      lineEl.textContent = d.current_line;
      lineEl.classList.remove('empty');
    } else {
      lineEl.textContent = 'waiting for a lyric line…';
      lineEl.classList.add('empty');
    }
    document.getElementById('source').textContent = d.source || '-';
    document.getElementById('latency').textContent = (d.latency_ms || 0) + 'ms';
  } catch (e) {
    document.getElementById('song').textContent = 'cannot reach mewsync';
  }
}

async function pollHistory() {
  try {
    const r = await fetch('/api/history');
    const entries = (await r.json()) || [];
    const list = document.getElementById('history');
    list.innerHTML = '';
    entries.slice().reverse().slice(0, 10).forEach(e => {
      const li = document.createElement('li');
      li.innerHTML =
        '<span class="h-song">' + escapeHtml(e.name) + '</span>' +
        '<span class="h-artist">' + escapeHtml(e.artist) + '</span>';
      list.appendChild(li);
    });
  } catch (e) {}
}

function escapeHtml(s) {
  const div = document.createElement('div');
  div.textContent = s || '';
  return div.innerHTML;
}

document.querySelectorAll('.tab').forEach(tab => {
  tab.addEventListener('click', () => {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
    tab.classList.add('active');
    document.getElementById('view-' + tab.dataset.tab).classList.add('active');
    if (tab.dataset.tab === 'settings') loadSettings();
  });
});

async function loadSettings() {
  try {
    const r = await fetch('/api/settings');
    const s = await r.json();
    document.getElementById('s-source').value = s.source || 'spotify';
    document.getElementById('s-lastfm-key').value = s.lastfm_api_key || '';
    document.getElementById('s-lastfm-user').value = s.lastfm_username || '';
    document.getElementById('s-timestamp').checked = !!s.view_timestamp;
    document.getElementById('s-label').checked = !!s.view_label;
    document.getElementById('s-autoclear').checked = !!s.auto_clear;
    document.getElementById('s-emoji').value = s.view_emoji || '';
    document.getElementById('s-adv-enabled').checked = !!s.advanced_enabled;
    document.getElementById('s-adv-template').value = s.advanced_template || '';
    document.getElementById('s-adv-emoji').value = s.advanced_emoji || '';
    document.getElementById('s-autooffset').checked = !!s.enable_autooffset;
    document.getElementById('s-offset').value = s.send_time_offset_ms || 0;
    document.getElementById('s-samples').value = s.autooffset_samples || 3;
    document.getElementById('token-hint').textContent = s.has_token ? '(a token is currently stored)' : '(no token stored yet)';
  } catch (e) {
    document.getElementById('save-status').textContent = 'could not load settings';
  }
}

document.getElementById('save-btn').addEventListener('click', async () => {
  const payload = {
    source: document.getElementById('s-source').value,
    lastfm_api_key: document.getElementById('s-lastfm-key').value,
    lastfm_username: document.getElementById('s-lastfm-user').value,
    view_timestamp: document.getElementById('s-timestamp').checked,
    view_label: document.getElementById('s-label').checked,
    view_emoji: document.getElementById('s-emoji').value,
    auto_clear: document.getElementById('s-autoclear').checked,
    advanced_enabled: document.getElementById('s-adv-enabled').checked,
    advanced_emoji: document.getElementById('s-adv-emoji').value,
    advanced_template: document.getElementById('s-adv-template').value,
    send_time_offset_ms: parseInt(document.getElementById('s-offset').value, 10) || 0,
    enable_autooffset: document.getElementById('s-autooffset').checked,
    autooffset_samples: parseInt(document.getElementById('s-samples').value, 10) || 1,
  };
  const token = document.getElementById('s-token').value;
  if (token) payload.token = token;
  const statusEl = document.getElementById('save-status');
  try {
    const r = await fetch('/api/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!r.ok) throw new Error(await r.text());
    document.getElementById('s-token').value = '';
    statusEl.textContent = 'saved';
    loadSettings();
    setTimeout(() => { statusEl.textContent = ''; }, 2000);
  } catch (e) {
    statusEl.textContent = 'save failed: ' + e.message;
  }
});

pollStatus();
pollHistory();
setInterval(pollStatus, 1000);
setInterval(pollHistory, 5000);
</script>
</body>
</html>
`

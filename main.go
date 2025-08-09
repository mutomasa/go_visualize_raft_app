package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strings"
    "sync"
    "time"
)

// Event represents a message/event in the simulation for sequence diagram
type Event struct {
    From string `json:"from"`
    To   string `json:"to"`
    Msg  string `json:"msg"`
    At   int64  `json:"at"`
}

// Node represents a simple Raft-like node state (mock)
type Node struct {
    ID     string
    Role   string // follower/candidate/leader
    Term   int
    Mu     sync.Mutex
}

// Cluster holds nodes and recorded events
type Cluster struct {
    Nodes  []*Node
    Events []Event
    Mu     sync.Mutex
}

func NewCluster(n int) *Cluster {
    c := &Cluster{Nodes: make([]*Node, n)}
    for i := 0; i < n; i++ {
        c.Nodes[i] = &Node{ID: fmt.Sprintf("N%d", i+1), Role: "follower", Term: 0}
    }
    return c
}

func (c *Cluster) record(from, to, msg string) {
    c.Mu.Lock()
    defer c.Mu.Unlock()
    c.Events = append(c.Events, Event{From: from, To: to, Msg: msg, At: time.Now().UnixMilli()})
}

// simulate runs a tiny mock of an election and a few heartbeats
func (c *Cluster) simulate() {
    if len(c.Nodes) == 0 {
        return
    }
    // election: pick N1 as candidate, becomes leader term 1
    cand := c.Nodes[0]
    cand.Mu.Lock()
    cand.Role = "candidate"
    cand.Term = 1
    cand.Mu.Unlock()
    // request votes from others
    for i := 1; i < len(c.Nodes); i++ {
        c.record(cand.ID, c.Nodes[i].ID, fmt.Sprintf("RequestVote(term=%d)", cand.Term))
        c.record(c.Nodes[i].ID, cand.ID, "VoteGranted")
    }
    // become leader
    cand.Mu.Lock()
    cand.Role = "leader"
    cand.Mu.Unlock()
    // heartbeats
    for r := 0; r < 3; r++ {
        for i := 1; i < len(c.Nodes); i++ {
            c.record(cand.ID, c.Nodes[i].ID, fmt.Sprintf("AppendEntries(term=%d, heartbeat)", cand.Term))
        }
        time.Sleep(50 * time.Millisecond)
    }
}

func (c *Cluster) reset() {
    c.Mu.Lock()
    defer c.Mu.Unlock()
    // reset nodes and clear events
    for i := range c.Nodes {
        c.Nodes[i].Mu.Lock()
        c.Nodes[i].Role = "follower"
        c.Nodes[i].Term = 0
        c.Nodes[i].Mu.Unlock()
    }
    c.Events = nil
}

func mermaidSequence(events []Event) string {
    var b strings.Builder
    b.WriteString("sequenceDiagram\n")
    // Declare participants
    parts := map[string]bool{}
    for _, e := range events {
        parts[e.From] = true
        parts[e.To] = true
    }
    for p := range parts {
        b.WriteString(fmt.Sprintf("participant %s\n", p))
    }
    // Messages
    for _, e := range events {
        b.WriteString(fmt.Sprintf("%s->>%s: %s\n", e.From, e.To, e.Msg))
    }
    return b.String()
}

func main() {
    cluster := NewCluster(5)
    cluster.simulate()

    mux := http.NewServeMux()
    mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
        cluster.Mu.Lock()
        defer cluster.Mu.Unlock()
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(cluster.Events)
    })
    mux.HandleFunc("/simulate", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        // run one simulation step (election + heartbeats)
        cluster.simulate()
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"ok":true}`))
    })
    mux.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        cluster.reset()
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"ok":true}`))
    })
    mux.HandleFunc("/sequence", func(w http.ResponseWriter, r *http.Request) {
        cluster.Mu.Lock()
        defer cluster.Mu.Unlock()
        m := mermaidSequence(cluster.Events)
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.Write([]byte(m))
    })
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Simple HTML page embedding Mermaid
        html := `<!doctype html><html><head>
<meta charset="utf-8"/>
<script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
<script>mermaid.initialize({ startOnLoad: false });</script>
<style>
  body { font-family: system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif; margin: 16px; }
  .toolbar { margin: 8px 0 16px; display: flex; gap: 8px; align-items: center; }
  .mermaid { border:1px solid #ccc; padding:12px; border-radius:6px; }
  button { padding: 8px 12px; cursor: pointer; }
</style>
</head><body>
<h2>Raft Simulation (Mock) - Sequence Diagram</h2>
<div class="toolbar">
  <button id="btn-sim">Run Simulation</button>
  <button id="btn-reset">Reset</button>
  <a href="/events" target="_blank">View JSON</a>
  <span id="count" style="margin-left:auto;opacity:.7;"></span>
</div>
<div id="seq" class="mermaid"></div>

<script>
async function loadSeq() {
  const res = await fetch('/sequence');
  const text = await res.text();
  try {
    const id = 'seq_'+Date.now();
    const out = await mermaid.render(id, text);
    const el = document.getElementById('seq');
    el.innerHTML = out.svg;
    if (out.bindFunctions) out.bindFunctions(el);
  } catch (e) {
    // Fallback: show raw text for troubleshooting
    const el = document.getElementById('seq');
    el.textContent = text;
  }
  try {
    const ev = await (await fetch('/events')).json();
    document.getElementById('count').textContent = 'Events: ' + ev.length;
  } catch {}
}
async function post(url){ await fetch(url, {method:'POST'}); }
document.getElementById('btn-sim').onclick = async ()=>{ await post('/simulate'); await loadSeq(); };
document.getElementById('btn-reset').onclick = async ()=>{ await post('/reset'); await loadSeq(); };
loadSeq();
</script>
</body></html>`
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write([]byte(html))
    })

    addr := ":8088"
    log.Printf("listening on %s", addr)
    log.Fatal(http.ListenAndServe(addr, mux))
}



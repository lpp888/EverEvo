package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGo)

	"everevo/internal/storage"
)

// Session is a persisted chat conversation.
type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	AgentID   string `json:"agentId"`
	Summary   string `json:"summary"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Message is a single turn inside a Session.
type Message struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Seq       int    `json:"seq"`
	Role      string `json:"role"` // user | assistant | tool | system
	Content   string `json:"content"`
	ToolJSON  string `json:"toolJson"` // tool_calls / tool results (opaque JSON)
	CreatedAt int64  `json:"createdAt"`
}

// Store wraps a SQLite database holding chat sessions and messages (and, in P2,
// the temporal knowledge graph). SQLite is accessed via the pure-Go modernc
// driver — no CGo — keeping the Wails build single-toolchain.
type Store struct {
	db     *sql.DB
	vector *VectorStore // may be nil → degraded (SQLite-only) mode
}

// NewStore opens (or creates) the memory SQLite DB under data/memory/memory.db
// and runs idempotent schema migrations.
func NewStore() (*Store, error) {
	base, err := storage.AppDataDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(base, "memory", "memory.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开记忆数据库失败: %w", err)
	}
	// WAL + busy_timeout is the SQLite sweet spot for a single-process desktop app.
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 WAL 失败: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("记忆库迁移失败: %w", err)
	}
	// Vector layer is best-effort: if chromem fails to init, memory degrades to
	// SQLite-only persistence (no semantic recall). The app layer binds the
	// embedding model via SetEmbeddingModel.
	if vs, vErr := NewVectorStore(); vErr != nil {
		log.Printf("[memory] 向量层初始化失败，降级为仅持久化: %v", vErr)
	} else {
		s.vector = vs
	}
	return s, nil
}

// migrate creates tables idempotently. P0 ships sessions + messages + memory_items;
// P2 adds the temporal knowledge graph (kg_nodes / kg_edges).
func (s *Store) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS sessions (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL,
	agent_id   TEXT NOT NULL DEFAULT '',
	summary    TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
	id         TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	seq        INTEGER NOT NULL,
	role       TEXT NOT NULL,
	content    TEXT NOT NULL DEFAULT '',
	tool_json  TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, seq);
CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_items (
	id         TEXT PRIMARY KEY,
	kind       TEXT NOT NULL,            -- turn | fact
	content    TEXT NOT NULL,            -- turn: userText; fact: fact text
	reply      TEXT NOT NULL DEFAULT '',  -- turn: assistant reply
	category   TEXT NOT NULL DEFAULT '',  -- fact: preference|fact|event|relationship
	session_id TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memory_kind ON memory_items(kind);
CREATE INDEX IF NOT EXISTS idx_memory_created ON memory_items(created_at DESC);
-- P2: temporal knowledge graph. kg_ prefix keeps these visually distinct.
CREATE TABLE IF NOT EXISTS kg_nodes (
	id           TEXT PRIMARY KEY,
	type         TEXT NOT NULL DEFAULT '',   -- entity type: person/place/project/...
	name         TEXT NOT NULL,              -- normalized (lowercase+trim) → disambiguation key
	name_raw     TEXT NOT NULL DEFAULT '',   -- original surface form
	props        TEXT NOT NULL DEFAULT '{}', -- JSON
	embedding_id TEXT NOT NULL DEFAULT '',   -- chromem doc id (kind=entity)
	created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_kg_nodes_name ON kg_nodes(name);
CREATE TABLE IF NOT EXISTS kg_edges (
	id          TEXT PRIMARY KEY,
	src_id      TEXT NOT NULL,
	dst_id      TEXT NOT NULL,
	type        TEXT NOT NULL,               -- relation: likes/works_at/owns/...
	props       TEXT NOT NULL DEFAULT '{}',
	valid_from  INTEGER NOT NULL,            -- fact lifetime start
	valid_to    INTEGER,                     -- NULL = currently valid (bi-temporal)
	recorded_at INTEGER NOT NULL,            -- when the system learned it
	session_id  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_kg_edges_src ON kg_edges(src_id);
CREATE INDEX IF NOT EXISTS idx_kg_edges_dst ON kg_edges(dst_id);
CREATE INDEX IF NOT EXISTS idx_kg_edges_valid ON kg_edges(valid_to);
-- P7: domain libraries (AI-managed knowledge domains, replaces workspaces).
CREATE TABLE IF NOT EXISTS domain_libraries (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	tags        TEXT NOT NULL DEFAULT '[]',
	auto_created INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL
);
-- P7 legacy: workspace table kept for migration compatibility.
CREATE TABLE IF NOT EXISTS everevo_workspaces (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
-- P5: core memory (identity/preferences/constraints) — permanent, never decayed/TTL'd.
CREATE TABLE IF NOT EXISTS user_facts (
	id           TEXT PRIMARY KEY,
	key          TEXT NOT NULL,
	value        TEXT NOT NULL,
	category     TEXT NOT NULL DEFAULT '',
	importance   TEXT NOT NULL DEFAULT 'high',
	locked       INTEGER NOT NULL DEFAULT 0,
	source       TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	last_access  INTEGER NOT NULL,
	access_count INTEGER NOT NULL DEFAULT 0
);
-- P8: cross-domain entity links (semantic anchors between domain KGs).
CREATE TABLE IF NOT EXISTS entity_links (
	id          TEXT PRIMARY KEY,
	src_node_id TEXT NOT NULL,
	dst_node_id TEXT NOT NULL,
	link_type   TEXT NOT NULL,
	confidence  REAL NOT NULL DEFAULT 0.5,
	source      TEXT NOT NULL DEFAULT 'auto',
	created_at  INTEGER NOT NULL
);
-- P8: evolution metrics (agent self-improvement tracking).
CREATE TABLE IF NOT EXISTS evolution_metrics (
	domain_id   TEXT NOT NULL,
	date        TEXT NOT NULL,
	total_turns INTEGER NOT NULL DEFAULT 0,
	reflected_turns INTEGER NOT NULL DEFAULT 0,
	experience_recalls INTEGER NOT NULL DEFAULT 0,
	cross_domain_links INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (domain_id, date)
);
-- P9: dream candidates (Light→REM→Deep pipeline staging).
CREATE TABLE IF NOT EXISTS dream_candidates (
	id          TEXT PRIMARY KEY,
	source_id   TEXT NOT NULL,
	source_type TEXT NOT NULL,
	stage       TEXT NOT NULL DEFAULT 'light',
	score       REAL NOT NULL DEFAULT 0,
	insight     TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL
);
-- P8: experience items (reflection loop — distilled insights from conversations).
CREATE TABLE IF NOT EXISTS experience_items (
	id           TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL DEFAULT 'default',
	kind         TEXT NOT NULL,           -- insight | lesson | strategy | error_pattern
	content      TEXT NOT NULL,           -- distilled experience text
	context      TEXT NOT NULL DEFAULT '',-- scenario that triggered this insight
	confidence   REAL NOT NULL DEFAULT 1.0,
	use_count    INTEGER NOT NULL DEFAULT 0,
	last_used    INTEGER NOT NULL DEFAULT 0,
	created_at   INTEGER NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return err
	}
	// P7: workspace/library isolation + cross-library tags.
	for _, c := range []struct{ table, col, def string }{
		{"memory_items", "workspace_id", "TEXT NOT NULL DEFAULT 'default'"},
		{"user_facts", "workspace_id", "TEXT NOT NULL DEFAULT 'default'"},
		{"kg_nodes", "workspace_id", "TEXT NOT NULL DEFAULT 'default'"},
		{"kg_edges", "workspace_id", "TEXT NOT NULL DEFAULT 'default'"},
		{"memory_items", "cross_tags", "TEXT NOT NULL DEFAULT '[]'"},
		{"kg_edges", "cross_tags", "TEXT NOT NULL DEFAULT '[]'"},
		{"kg_edges", "weight", "INTEGER NOT NULL DEFAULT 1"},
	} {
		if err := s.addColumnIfMissing(c.table, c.col, c.def); err != nil {
			return err
		}
	}
	// One-time dedup: consolidate duplicate valid edges into weighted singles.
	if s.GetMeta("kg_edge_dedup_done") == "" {
		_ = s.dedupEdges()
		_ = s.SetMeta("kg_edge_dedup_done", "1")
	}
	// P9: usage tracking on domain libraries.
	if err := s.addColumnIfMissing("domain_libraries", "use_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// P5: add decay columns to memory_items for existing DBs (SQLite has no
	// ADD COLUMN IF NOT EXISTS, so guard via pragma_table_info).
	for _, c := range []struct{ col, def string }{
		{"last_access", "INTEGER NOT NULL DEFAULT 0"},
		{"access_count", "INTEGER NOT NULL DEFAULT 0"},
		{"importance", "TEXT NOT NULL DEFAULT 'normal'"},
		{"recall_count", "INTEGER NOT NULL DEFAULT 0"},
		{"query_diversity", "INTEGER NOT NULL DEFAULT 0"},
		{"cross_domain_hits", "INTEGER NOT NULL DEFAULT 0"},
		{"concept_tags", "TEXT NOT NULL DEFAULT '[]'"},
	} {
		if err := s.addColumnIfMissing("memory_items", c.col, c.def); err != nil {
			return err
		}
	}
	return nil
}

// addColumnIfMissing adds a column to a table if it isn't already present.
func (s *Store) addColumnIfMissing(table, col, def string) error {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, col).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, def)); err != nil {
			return err
		}
	}
	return nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// ─── Sessions ─────────────────────────────────────────────────────

// CreateSession inserts a new session row.
func (s *Store) CreateSession(id, title, agentID string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO sessions(id, title, agent_id, summary, created_at, updated_at)
		VALUES(?, ?, ?, '', ?, ?)`, id, title, agentID, now, now)
	return err
}

// ListSessions returns all sessions, newest first.
func (s *Store) ListSessions() ([]Session, error) {
	rows, err := s.db.Query(`SELECT id, title, agent_id, summary, created_at, updated_at
		FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.AgentID, &sess.Summary, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// GetSession returns one session by id.
func (s *Store) GetSession(id string) (*Session, error) {
	var sess Session
	err := s.db.QueryRow(`SELECT id, title, agent_id, summary, created_at, updated_at
		FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.Title, &sess.AgentID, &sess.Summary, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// RenameSession updates a session's title.
func (s *Store) RenameSession(id, title string) error {
	_, err := s.db.Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UnixMilli(), id)
	return err
}

// UpdateSummary stores a rolled-up summary for a session (P0 unused; P1 hooks
// the summarizer here).
func (s *Store) UpdateSummary(id, summary string) error {
	_, err := s.db.Exec(`UPDATE sessions SET summary = ? WHERE id = ?`, summary, id)
	return err
}

// DeleteSession removes a session and all of its messages (cascade).
func (s *Store) DeleteSession(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// touchSession bumps updated_at; called inside the message-append transaction.
func touchSession(tx *sql.Tx, id string, now int64) error {
	_, err := tx.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now, id)
	return err
}

// ─── Messages ─────────────────────────────────────────────────────

// AppendMessage inserts a message with an auto-incremented per-session seq and
// touches the session's updated_at. Returns the stored message.
func (s *Store) AppendMessage(sessionID, id, role, content, toolJSON string) (*Message, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	var seq int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE session_id = ?`, sessionID).Scan(&seq); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(`INSERT INTO messages(id, session_id, seq, role, content, tool_json, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)`, id, sessionID, seq, role, content, toolJSON, now); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := touchSession(tx, sessionID, now); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Message{
		ID:        id,
		SessionID: sessionID,
		Seq:       seq,
		Role:      role,
		Content:   content,
		ToolJSON:  toolJSON,
		CreatedAt: now,
	}, nil
}

// ListMessages returns a session's messages ordered by seq.
func (s *Store) ListMessages(sessionID string) ([]Message, error) {
	rows, err := s.db.Query(`SELECT id, session_id, seq, role, content, tool_json, created_at
		FROM messages WHERE session_id = ? ORDER BY seq ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &m.Role, &m.Content, &m.ToolJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListMessagesRecent returns the last `limit` messages for a session in
// chronological order. Used by the frontend to load only the most recent
// messages on session switch rather than the entire history.
func (s *Store) ListMessagesRecent(sessionID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`SELECT id, session_id, seq, role, content, tool_json, created_at
		FROM messages WHERE session_id = ?
		ORDER BY seq DESC
		LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &m.Role, &m.Content, &m.ToolJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to chronological order (ASC).
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ClearMessages deletes all messages of a session but keeps the session row.
func (s *Store) ClearMessages(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID)
	return err
}

// UpdateMessageToolJSON updates the tool_json column of a message.
func (s *Store) UpdateMessageToolJSON(msgID, toolJSON string) error {
	_, err := s.db.Exec(`UPDATE messages SET tool_json = ? WHERE id = ?`, toolJSON, msgID)
	return err
}

// CountMessages returns the message count of a session (for summary cadence).
func (s *Store) CountMessages(sessionID string) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID).Scan(&n)
	return n
}

// CountAllUserMessages returns total user messages across all sessions.
func (s *Store) CountAllUserMessages() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE role = 'user'`).Scan(&n)
	return n
}

// ─── Meta (key-value) ─────────────────────────────────────────────

// SetMeta persists a key-value pair (used for the bound embedding model dir).
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetMeta reads a key; returns "" if absent.
func (s *Store) GetMeta(key string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

// EmbeddingModelDir returns the bound embedding model directory ("" if unset).
func (s *Store) EmbeddingModelDir() string { return s.GetMeta("embeddingModelDir") }

// SetEmbeddingModel binds the embedding model directory used for semantic memory.
func (s *Store) SetEmbeddingModel(dir string) error { return s.SetMeta("embeddingModelDir", dir) }

// MemoryPolicy holds the hardware-adaptive retention knobs (computed by the app
// from host RAM/disk, stored in meta). Episodic recall + TTL sweep read these.
type MemoryPolicy struct {
	Tier         string  `json:"tier"`         // low | standard | high
	HalfLifeDays int     `json:"halfLifeDays"` // recency half-life
	TTLDays      int     `json:"ttlDays"`      // episodic items older than this are sweep candidates
	RecallK      int     `json:"recallK"`      // top-k after decay re-rank
	ItemCap      int     `json:"itemCap"`      // soft cap on memory_items
	CoreCap      int     `json:"coreCap"`      // soft cap on user_facts
	Alpha        float64 `json:"alpha"`        // semantic vs recency weight (0..1)
}

// DefaultMemoryPolicy is the standard tier (used until the app computes one).
func DefaultMemoryPolicy() MemoryPolicy {
	return MemoryPolicy{Tier: "standard", HalfLifeDays: 14, TTLDays: 90, RecallK: 3, ItemCap: 2000, CoreCap: 200, Alpha: 0.7}
}

// Policy returns the stored memory policy (or the default if unset/unparsable).
func (s *Store) Policy() MemoryPolicy {
	raw := s.GetMeta("memoryPolicy")
	if raw == "" {
		return DefaultMemoryPolicy()
	}
	var p MemoryPolicy
	if json.Unmarshal([]byte(raw), &p) == nil && p.Tier != "" {
		return p
	}
	return DefaultMemoryPolicy()
}

// SetPolicyJSON stores a serialized memory policy (called by the app after
// computing it from host hardware).
func (s *Store) SetPolicyJSON(raw string) error { return s.SetMeta("memoryPolicy", raw) }

// ─── Semantic memory (vector + manifest) ──────────────────────────

// TurnHit is a recalled user question with its associated assistant reply.
type TurnHit struct {
	Content    string  `json:"content"`
	Reply      string  `json:"reply"`
	ItemID     string  `json:"-"` // join key to memory_items (not serialized)
	Similarity float32 `json:"similarity"`
	Score      float32 `json:"score"` // decay-adjusted score (set by re-rank)
}

// FactHit is a recalled extracted fact.
type FactHit struct {
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	ItemID     string  `json:"-"`
	Similarity float32 `json:"similarity"`
	Score      float32 `json:"score"`
}

// EntityHit is a recalled graph entity — the vector seed for graph expansion.
type EntityHit struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Similarity float32 `json:"similarity"`
}

// MemoryItem is a manifest row for UI listing / counts / clear.
type MemoryItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // turn | fact
	Content   string `json:"content"`
	Reply     string `json:"reply"`
	Category  string `json:"category"`
	SessionID string `json:"sessionId"`
	CreatedAt int64  `json:"createdAt"`
}

// HasVector reports whether the vector layer is available.
func (s *Store) HasVector() bool { return s.vector != nil }

// AddTurnMemory writes a user-question turn to both the SQLite manifest and the
// chromem collection (question vectorized; reply carried in metadata).
func (s *Store) AddTurnMemory(itemID, userText, reply, sessionID string, userEmb []float32) error {
	now := time.Now().UnixMilli()
	if _, err := s.db.Exec(`INSERT INTO memory_items(id, kind, content, reply, category, session_id, created_at)
		VALUES(?, 'turn', ?, ?, '', ?, ?)`, itemID, userText, reply, sessionID, now); err != nil {
		return err
	}
	if s.vector != nil {
		if err := s.vector.AddTurn(itemID, userText, reply, sessionID, itemID, userEmb); err != nil {
			return err
		}
	}
	return nil
}

// AddFactMemory writes an extracted fact to both stores. importance tags the
// row for decay-rate adjustment (low → forgets faster); high-importance facts
// should go to user_facts instead (handled by the caller).
func (s *Store) AddFactMemory(itemID, content, category, importance, libraryID, crossTags string, emb []float32) error {
	if importance == "" {
		importance = "normal"
	}
	if libraryID == "" {
		libraryID = "default"
	}
	now := time.Now().UnixMilli()
	if _, err := s.db.Exec(`INSERT INTO memory_items(id, kind, content, reply, category, session_id, created_at, importance, workspace_id, cross_tags)
		VALUES(?, 'fact', ?, '', ?, '', ?, ?, ?, ?)`, itemID, content, category, now, importance, libraryID, crossTags); err != nil {
		return err
	}
	if s.vector != nil {
		if err := s.vector.AddFact(itemID, content, category, itemID, emb); err != nil {
			return err
		}
	}
	return nil
}


// DeleteMemoryItem removes one item from memory_items by ID.
func (s *Store) DeleteMemoryItem(id string) error {
	_, err := s.db.Exec(`DELETE FROM memory_items WHERE id = ?`, id)
	return err
}
// UserFact is a permanent core-memory row (identity/preference/constraint) —
// never decayed or TTL'd.
type UserFact struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	Category   string `json:"category"`
	Importance string `json:"importance"`
	Locked     bool   `json:"locked"`
	Source     string `json:"source"`
	CreatedAt  int64  `json:"createdAt"`
}

// AddUserFact inserts a core-memory row.
func (s *Store) AddUserFact(id, key, value, category, importance, source string) error {
	if importance == "" {
		importance = "high"
	}
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO user_facts(id, key, value, category, importance, locked, source, created_at, last_access, access_count)
		VALUES(?, ?, ?, ?, ?, 0, ?, ?, ?, 0)`, id, key, value, category, importance, source, now, now)
	return err
}

// ListUserFacts returns all core-memory rows (newest first).
func (s *Store) ListUserFacts() ([]UserFact, error) {
	rows, err := s.db.Query(`SELECT id, key, value, category, importance, locked, source, created_at FROM user_facts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserFact
	for rows.Next() {
		var f UserFact
		var locked int
		if err := rows.Scan(&f.ID, &f.Key, &f.Value, &f.Category, &f.Importance, &locked, &f.Source, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.Locked = locked != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

// LockUserFact sets/clears the locked flag (locked rows are never modified by sweeps).
func (s *Store) LockUserFact(id string, locked bool) error {
	v := 0
	if locked {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE user_facts SET locked = ? WHERE id = ?`, v, id)
	return err
}

// ─── Workspaces (P7) ──────────────────────────────────────────────

// DefaultWorkspace returns the first workspace, creating a "核心领域" (core domain) one if none exist.
func (s *Store) DefaultWorkspace() (string, error) {
	var id string
	if err := s.db.QueryRow(`SELECT id FROM everevo_workspaces LIMIT 1`).Scan(&id); err == nil && id != "" {
		return id, nil
	}
	id = fmt.Sprintf("ws_%x", time.Now().UnixNano())
	name := "核心领域"
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO everevo_workspaces(id, name, created_at) VALUES(?, ?, ?)`, id, name, now)
	return id, err
}

// WorkspaceList returns all workspaces.
func (s *Store) WorkspaceList() ([]struct {
	ID        string
	Name      string
	CreatedAt int64
}, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM everevo_workspaces ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID        string
		Name      string
		CreatedAt int64
	}
	for rows.Next() {
		var ws struct {
			ID        string
			Name      string
			CreatedAt int64
		}
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}

// WorkspaceCreate adds a workspace.
func (s *Store) WorkspaceCreate(name string) (string, error) {
	id := fmt.Sprintf("ws_%x", time.Now().UnixNano())
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO everevo_workspaces(id, name, created_at) VALUES(?, ?, ?)`, id, name, now)
	return id, err
}

// WorkspaceDelete removes a workspace row. The caller is responsible for cascade-
// deleting or reassigning the workspace's data (app layer).
func (s *Store) WorkspaceDelete(id string) error {
	_, err := s.db.Exec(`DELETE FROM everevo_workspaces WHERE id = ?`, id)
	return err
}

// ─── Domain Libraries (P7) — AI-managed knowledge domains ──────

// DefaultLibrary returns the first library, creating a "核心领域" (core domain) one if none exist.
func (s *Store) DefaultLibrary() (string, error) {
	var id string
	if err := s.db.QueryRow(`SELECT id FROM domain_libraries LIMIT 1`).Scan(&id); err == nil && id != "" {
		return id, nil
	}
	id = fmt.Sprintf("lib_%x", time.Now().UnixNano())
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO domain_libraries(id, name, description, auto_created, created_at) VALUES(?, '核心领域', '', 0, ?)`, id, now)
	return id, err
}

// LibraryList returns all domain libraries.
func (s *Store) LibraryList() ([]struct {
	ID          string
	Name        string
	Description string
	Tags        string
	AutoCreated bool
	UseCount    int
	CreatedAt   int64
}, error) {
	rows, err := s.db.Query(`SELECT id, name, description, tags, auto_created, COALESCE(use_count,0), created_at FROM domain_libraries ORDER BY use_count DESC, auto_created ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID          string
		Name        string
		Description string
		Tags        string
		AutoCreated bool
		UseCount    int
		CreatedAt   int64
	}
	for rows.Next() {
		var lib struct {
			ID          string
			Name        string
			Description string
			Tags        string
			AutoCreated bool
			UseCount    int
			CreatedAt   int64
		}
		var ac int
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Description, &lib.Tags, &ac, &lib.UseCount, &lib.CreatedAt); err != nil {
			return nil, err
		}
		lib.AutoCreated = ac != 0
		out = append(out, lib)
	}
	return out, rows.Err()
}

// LibraryCreate adds a domain library and returns its id.
func (s *Store) LibraryCreate(name, description string, autoCreated bool) (string, error) {
	id := fmt.Sprintf("lib_%x", time.Now().UnixNano())
	ac := 0
	if autoCreated {
		ac = 1
	}
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO domain_libraries(id, name, description, auto_created, created_at) VALUES(?, ?, ?, ?, ?)`,
		id, name, description, ac, now)
	return id, err
}

// LibraryDelete removes a library row. The caller should cascade data first.
func (s *Store) LibraryDelete(id string) error {
	_, err := s.db.Exec(`DELETE FROM domain_libraries WHERE id = ?`, id)
	return err
}

// LibraryMerge re-points all knowledge from dropID to keepID and deletes dropID.

// BumpLibraryUse increments the usage counter for a domain library.
func (s *Store) BumpLibraryUse(id string) {
	s.db.Exec(`UPDATE domain_libraries SET use_count = COALESCE(use_count,0) + 1 WHERE id = ?`, id)
}
func (s *Store) LibraryMerge(keepID, dropID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, table := range []string{"memory_items", "user_facts", "kg_nodes", "kg_edges"} {
		if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET workspace_id = ? WHERE workspace_id = ?`, table), keepID, dropID); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM domain_libraries WHERE id = ?`, dropID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ─── Experience Items (P8) — reflection distilled insights ─────

// AddExperience stores a distilled insight from the reflection loop.
func (s *Store) AddExperience(id, workspaceID, kind, content, context string, confidence float64, now int64) error {
	_, err := s.db.Exec(
		`INSERT INTO experience_items(id, workspace_id, kind, content, context, confidence, use_count, last_used, created_at)
		 VALUES(?,?,?,?,?,?,0,0,?)`,
		id, workspaceID, kind, content, context, confidence, now)
	return err
}

// DeleteExperience removes a single experience item by ID.
func (s *Store) DeleteExperience(id string) error {
	_, err := s.db.Exec(`DELETE FROM experience_items WHERE id = ?`, id)
	return err
}

// ListExperience returns recent experience items, optionally filtered by workspace.
func (s *Store) ListExperience(workspaceID string, limit int) ([]ExperienceItem, error) {
	if limit <= 0 { limit = 20 }
	var rows *sql.Rows; var err error
	if workspaceID == "" {
		rows, err = s.db.Query(`SELECT id,workspace_id,kind,content,context,confidence,use_count,last_used,created_at FROM experience_items ORDER BY confidence DESC LIMIT ?`, limit)
	} else {
		rows, err = s.db.Query(`SELECT id,workspace_id,kind,content,context,confidence,use_count,last_used,created_at FROM experience_items WHERE workspace_id=? ORDER BY confidence DESC LIMIT ?`, workspaceID, limit)
	}
	if err != nil { return nil, err }
	defer rows.Close()
	var out []ExperienceItem
	for rows.Next() {
		var e ExperienceItem
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.Kind, &e.Content, &e.Context, &e.Confidence, &e.UseCount, &e.LastUsed, &e.CreatedAt); err != nil { return nil, err }
		out = append(out, e)
	}
	return out, rows.Err()
}

// BumpExperience increments the use count and last_used timestamp.
func (s *Store) BumpExperience(id string, now int64) error {
	_, err := s.db.Exec(`UPDATE experience_items SET use_count=use_count+1, last_used=? WHERE id=?`, now, id)
	return err
}

// ExperienceItem is one distilled insight from the reflection loop.
type ExperienceItem struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspaceId"`
	Kind        string  `json:"kind"`
	Content     string  `json:"content"`
	Context     string  `json:"context"`
	Confidence  float64 `json:"confidence"`
	UseCount    int     `json:"useCount"`
	LastUsed    int64   `json:"lastUsed"`
	CreatedAt   int64   `json:"createdAt"`
}

// ─── Entity Links (P8) — cross-domain semantic anchors ──────────

// LinkEntitiesAcrossLibraries finds entities with matching names across two
// libraries and creates entity_links for them. Returns the number of links created.
func (s *Store) LinkEntitiesAcrossLibraries(libA, libB string) (int, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.type, b.id, b.name, b.type
		FROM kg_nodes a
		JOIN kg_nodes b ON LOWER(a.name) = LOWER(b.name) AND a.id != b.id
		WHERE a.workspace_id = ? AND b.workspace_id = ?
		AND NOT EXISTS (
			SELECT 1 FROM entity_links el
			WHERE (el.src_node_id = a.id AND el.dst_node_id = b.id)
			   OR (el.src_node_id = b.id AND el.dst_node_id = a.id)
		)`, libA, libB)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	now := time.Now().UnixMilli()
	count := 0
	for rows.Next() {
		var srcID, srcName, srcType, dstID, dstName, dstType string
		if err := rows.Scan(&srcID, &srcName, &srcType, &dstID, &dstName, &dstType); err != nil {
			continue
		}
		id := fmt.Sprintf("el_%x", now+int64(count))
		linkType := "sameAs"
		if srcType != dstType {
			linkType = "relatedTo"
		}
		_, err := s.db.Exec(
			`INSERT INTO entity_links(id, src_node_id, dst_node_id, link_type, confidence, source, created_at)
			 VALUES(?,?,?,?,?,?,?)`,
			id, srcID, dstID, linkType, 0.85, "auto", now)
		if err == nil {
			count++
		}
	}
	return count, rows.Err()
}

// ListEntityLinks returns all cross-domain entity links for visualization.
func (s *Store) ListEntityLinks() ([]EntityLink, error) {
	rows, err := s.db.Query(`SELECT el.id, el.src_node_id, el.dst_node_id, el.link_type, el.confidence,
		sn.name, sn.workspace_id, dn.name, dn.workspace_id
		FROM entity_links el
		JOIN kg_nodes sn ON sn.id = el.src_node_id
		JOIN kg_nodes dn ON dn.id = el.dst_node_id
		ORDER BY el.confidence DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntityLink
	for rows.Next() {
		var el EntityLink
		if err := rows.Scan(&el.ID, &el.SrcNodeID, &el.DstNodeID, &el.LinkType, &el.Confidence,
			&el.SrcName, &el.SrcLibrary, &el.DstName, &el.DstLibrary); err != nil {
			continue
		}
		out = append(out, el)
	}
	return out, rows.Err()
}

type EntityLink struct {
	ID         string  `json:"id"`
	SrcNodeID  string  `json:"srcNodeId"`
	DstNodeID  string  `json:"dstNodeId"`
	LinkType   string  `json:"linkType"`
	Confidence float64 `json:"confidence"`
	SrcName    string  `json:"srcName"`
	SrcLibrary string  `json:"srcLibrary"`
	DstName    string  `json:"dstName"`
	DstLibrary string  `json:"dstLibrary"`
}

// ─── Conflict Detection (P8) ──────────────────────────────────────

// DetectConflicts finds entity_links where the linked entities have contradictory
// knowledge (edges with opposite semantics or conflicting facts).
func (s *Store) DetectConflicts() ([]Conflict, error) {
	rows, err := s.db.Query(`
		SELECT el.id, el.src_node_id, el.dst_node_id, el.link_type,
			sn.name, sn.workspace_id, dn.name, dn.workspace_id
		FROM entity_links el
		JOIN kg_nodes sn ON sn.id = el.src_node_id
		JOIN kg_nodes dn ON dn.id = el.dst_node_id
		WHERE el.confidence < 0.6`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Conflict
	for rows.Next() {
		var c Conflict
		if err := rows.Scan(&c.LinkID, &c.SrcNodeID, &c.DstNodeID, &c.LinkType,
			&c.SrcName, &c.SrcLib, &c.DstName, &c.DstLib); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type Conflict struct {
	LinkID    string `json:"linkId"`
	SrcNodeID string `json:"srcNodeId"`
	DstNodeID string `json:"dstNodeId"`
	LinkType  string `json:"linkType"`
	SrcName   string `json:"srcName"`
	SrcLib    string `json:"srcLib"`
	DstName   string `json:"dstName"`
	DstLib    string `json:"dstLib"`
}

// ─── Evolution Metrics (P8) ───────────────────────────────────────

// RecordMetrics upserts a daily metrics row for a domain.
func (s *Store) RecordMetrics(domainID, date string, turns, reflected, recalls, links int) error {
	_, err := s.db.Exec(`INSERT INTO evolution_metrics(domain_id, date, total_turns, reflected_turns, experience_recalls, cross_domain_links)
		VALUES(?,?,?,?,?,?) ON CONFLICT(domain_id, date) DO UPDATE SET
		total_turns=total_turns+?, reflected_turns=reflected_turns+?,
		experience_recalls=experience_recalls+?, cross_domain_links=cross_domain_links+?`,
		domainID, date, turns, reflected, recalls, links,
		turns, reflected, recalls, links)
	return err
}

// DeleteUserFact removes a core-memory row.
func (s *Store) DeleteUserFact(id string) error {
	_, err := s.db.Exec(`DELETE FROM user_facts WHERE id = ?`, id)
	return err
}

// QueryMemory runs a two-pass recall: kind=turn (question↔question) and
// kind=fact, each top-k. Embedding is computed once by the caller.
func (s *Store) QueryMemory(emb []float32, k int) (turns []TurnHit, facts []FactHit, err error) {
	if s.vector == nil {
		return nil, nil, nil
	}
	turns, err = s.vector.QueryTurns(emb, k)
	if err != nil {
		return nil, nil, err
	}
	facts, err = s.vector.QueryFacts(emb, k)
	if err != nil {
		return nil, nil, err
	}
	// P5: recency-decay re-rank + access warmth refresh.
	p := s.Policy()
	rk := p.RecallK
	if rk <= 0 {
		rk = k
	}
	now := time.Now().UnixMilli()
	turns = s.decayRankTurns(turns, p, now, rk)
	facts = s.decayRankFacts(facts, p, now, rk)
	return turns, facts, nil
}

// memMeta carries the recency/importance columns used for decay re-rank.
type memMeta struct {
	lastAccess int64
	createdAt  int64
	importance string
}

func toAny(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// memoryMeta fetches last_access/created_at/importance for the given item ids.
func (s *Store) memoryMeta(ids []string) map[string]memMeta {
	out := map[string]memMeta{}
	if len(ids) == 0 {
		return out
	}
	rows, err := s.db.Query(`SELECT id, last_access, created_at, importance FROM memory_items WHERE id IN (`+placeholders(len(ids))+`)`, toAny(ids)...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var m memMeta
		if err := rows.Scan(&id, &m.lastAccess, &m.createdAt, &m.importance); err == nil {
			out[id] = m
		}
	}
	return out
}

// decayScore = α·cos + (1-α)·0.5^(ageDays/halfLife); low-importance ages 2× faster.
func decayScore(sim float32, lastAccess, createdAt int64, importance string, p MemoryPolicy, now int64) float32 {
	ref := lastAccess
	if ref == 0 {
		ref = createdAt
	}
	ageDays := float64(now-ref) / 86400000
	if importance == "low" {
		ageDays *= 2
	}
	hl := p.HalfLifeDays
	if hl <= 0 {
		hl = 14
	}
	recency := math.Pow(0.5, ageDays/float64(hl))
	cos := sim
	if cos < 0 {
		cos = 0
	}
	alpha := p.Alpha
	if alpha <= 0 {
		alpha = 0.7
	}
	return float32(alpha)*cos + float32(1-alpha)*float32(recency)
}

// decayRankTurns scores + sorts + caps turn hits, then refreshes access warmth.
func (s *Store) decayRankTurns(hits []TurnHit, p MemoryPolicy, now int64, k int) []TurnHit {
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ItemID)
	}
	meta := s.memoryMeta(ids)
	// drop orphans (chromem docs whose SQLite row was TTL-deleted)
	kept := hits[:0]
	for _, h := range hits {
		if _, ok := meta[h.ItemID]; ok {
			kept = append(kept, h)
		}
	}
	hits = kept
	for i := range hits {
		m := meta[hits[i].ItemID]
		hits[i].Score = decayScore(hits[i].Similarity, m.lastAccess, m.createdAt, m.importance, p, now)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if k > 0 && len(hits) > k {
		hits = hits[:k]
	}
	keep := make([]string, 0, len(hits))
	for _, h := range hits {
		keep = append(keep, h.ItemID)
	}
	s.bumpAccess(keep, now)
	return hits
}

// decayRankFacts is the fact analog of decayRankTurns.
func (s *Store) decayRankFacts(hits []FactHit, p MemoryPolicy, now int64, k int) []FactHit {
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ItemID)
	}
	meta := s.memoryMeta(ids)
	// drop orphans (chromem docs whose SQLite row was TTL-deleted)
	kept := hits[:0]
	for _, h := range hits {
		if _, ok := meta[h.ItemID]; ok {
			kept = append(kept, h)
		}
	}
	hits = kept
	for i := range hits {
		m := meta[hits[i].ItemID]
		hits[i].Score = decayScore(hits[i].Similarity, m.lastAccess, m.createdAt, m.importance, p, now)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if k > 0 && len(hits) > k {
		hits = hits[:k]
	}
	keep := make([]string, 0, len(hits))
	for _, h := range hits {
		keep = append(keep, h.ItemID)
	}
	s.bumpAccess(keep, now)
	return hits
}

// bumpAccess refreshes last_access + bumps access_count (LRU warmth) for the
// recalled items.
func (s *Store) bumpAccess(ids []string, now int64) {
	if len(ids) == 0 {
		return
	}
	args := []any{now}
	args = append(args, toAny(ids)...)
	_, _ = s.db.Exec(`UPDATE memory_items SET last_access = ?, access_count = access_count + 1 WHERE id IN (`+placeholders(len(ids))+`)`, args...)
}

// SweepExpiredPolicy deletes episodic memory_items older than TTLDays whose decay
// score has fallen below 0.05 (using sim=1 as the worst case — only truly stale
// items get swept). Returns the deleted ids. user_facts is never touched.
// chromem docs become harmless orphans; the decay re-rank filters them out by
// the SQLite row being gone.

// BumpImportance adaptively adjusts importance based on recall frequency
// (Ebbinghaus effect: recalled items get stronger; unrecalled items decay).
// ScoreMemory computes a 6-dimension weighted score for a memory item.
// Dimensions: relevance(30%), frequency(24%), diversity(15%), recency(15%),
// consolidation(10%), richness(6%).
func ScoreMemory(recallCount, accessCount, queryDiversity, crossDomainHits int, lastAccess, createdAt int64, conceptTags string, now int64) float64 {
	totalRecalls := float64(max(1, recallCount))
	totalAccess := float64(max(1, accessCount))
	totalDiversity := float64(max(1, queryDiversity))

	relevance := float64(recallCount) / totalRecalls * 0.30
	frequency := float64(accessCount) / totalAccess * 0.24
	diversity := float64(queryDiversity) / totalDiversity * 0.15

	ref := lastAccess
	if ref == 0 {
		ref = createdAt
	}
	ageDays := float64(now-ref) / 86400000.0
	recency := (1.0 / (1.0 + ageDays*0.1)) * 0.15

	consolidation := 0.03
	if crossDomainHits > 0 {
		consolidation = 0.10
	}

	var tags []string
	json.Unmarshal([]byte(conceptTags), &tags)
	richness := float64(len(tags)) / max(1.0, float64(len(tags))) * 0.06
	if len(tags) == 0 {
		richness = 0
	}

	return relevance + frequency + diversity + recency + consolidation + richness
}

// BumpScore updates scoring fields on recall and applies Ebbinghaus decay.
func (s *Store) BumpScore(ids []string, recalled bool, now int64) {
	multiplier := 0.85
	if recalled { multiplier = 1.2 }
	for _, id := range ids {
		s.db.Exec(`UPDATE memory_items SET last_access=?, access_count=access_count+1,
			recall_count=CASE WHEN ? >= 1.2 THEN recall_count+1 ELSE recall_count END,
			importance=CASE
				WHEN importance='critical' THEN 'critical'
				WHEN importance='high' AND ? < 0.85 THEN 'normal'
				WHEN importance='normal' AND ? >= 1.2 THEN 'high'
				WHEN importance='normal' AND ? < 0.85 THEN 'low'
				WHEN importance='low' AND ? >= 1.2 THEN 'normal'
				ELSE importance
			END WHERE id=?`,
			now, multiplier, multiplier, multiplier, multiplier, id)
	}
}

// PromoteByScore re-ranks all memory_items using the 6-dimension score and
// adjusts importance accordingly. Returns counts for promoted/demoted/deleted.
func (s *Store) PromoteByScore(keepCap int) (promoted, demoted, deleted int) {
	now := time.Now().UnixMilli()
	rows, err := s.db.Query(`SELECT id, recall_count, access_count, query_diversity,
		cross_domain_hits, last_access, created_at, concept_tags, importance
		FROM memory_items ORDER BY importance DESC`)
	if err != nil {
		return
	}
	defer rows.Close()
	type scored struct {
		id    string
		score float64
		imp   string
	}
	var list []scored
	for rows.Next() {
		var id, imp, tags string
		var rc, ac, qd, cdh int
		var la, ca int64
		if rows.Scan(&id, &rc, &ac, &qd, &cdh, &la, &ca, &tags, &imp) != nil {
			continue
		}
		sc := ScoreMemory(rc, ac, qd, cdh, la, ca, tags, now)
		list = append(list, scored{id, sc, imp})
	}
	// Sort by score descending
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	// Promote top, demote bottom
	for i, v := range list {
		if v.score >= 0.6 && v.imp == "normal" {
			s.db.Exec(`UPDATE memory_items SET importance='high' WHERE id=?`, v.id)
			promoted++
		} else if v.score >= 0.6 && v.imp == "low" {
			s.db.Exec(`UPDATE memory_items SET importance='normal' WHERE id=?`, v.id)
			promoted++
		} else if v.score < 0.2 && v.imp == "normal" && v.score < 0.2 {
			s.db.Exec(`UPDATE memory_items SET importance='low' WHERE id=?`, v.id)
			demoted++
		}
		// Delete lowest scoring items beyond cap
		if i >= keepCap && v.imp != "critical" {
			s.db.Exec(`DELETE FROM memory_items WHERE id=?`, v.id)
			deleted++
		}
	}
	return
}

// TrimMemoryCapacity enforces a hard cap on total memory_items. Least-important,
// oldest-accessed items are deleted first. Called during the daily sweep.
func (s *Store) TrimMemoryCapacity(hardCap int) (int, error) {
	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM memory_items`).Scan(&total)
	if total <= hardCap {
		return 0, nil
	}
	excess := total - hardCap
	res, err := s.db.Exec(`DELETE FROM memory_items WHERE id IN (
		SELECT id FROM memory_items ORDER BY
		CASE importance
			WHEN 'critical' THEN 0 WHEN 'high' THEN 1
			WHEN 'normal' THEN 2 WHEN 'low' THEN 3 ELSE 4
		END ASC, last_access ASC LIMIT ?)`, excess)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
func (s *Store) SweepExpiredPolicy() ([]string, error) {
	p := s.Policy()
	now := time.Now().UnixMilli()
	cutoff := now - int64(p.TTLDays)*86400000
	rows, err := s.db.Query(`SELECT id, last_access, created_at, importance FROM memory_items WHERE created_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	var victims []string
	for rows.Next() {
		var id string
		var la, ca int64
		var imp string
		if err := rows.Scan(&id, &la, &ca, &imp); err != nil {
			continue
		}
		if decayScore(1.0, la, ca, imp, p, now) < 0.05 {
			victims = append(victims, id)
		}
	}
	rows.Close()
	for _, id := range victims {
		_, _ = s.db.Exec(`DELETE FROM memory_items WHERE id = ?`, id)
	}
	return victims, nil
}

// AddEntity writes a graph-entity vector (kind=entity). No-op when the vector
// layer is unavailable (graph degrades to SQLite-only — no seed-by-similarity).
func (s *Store) AddEntity(nodeID, name, entityType string, emb []float32) error {
	if s.vector == nil {
		return nil
	}
	return s.vector.AddEntity(nodeID, name, entityType, emb)
}

// EntitySearch returns up to k entity seeds matching the embedding (vector layer).
func (s *Store) EntitySearch(emb []float32, k int) ([]EntityHit, error) {
	if s.vector == nil {
		return nil, nil
	}
	return s.vector.QueryEntities(emb, k)
}

// ListMemoryItems returns the k most recent manifest rows (any kind).
func (s *Store) ListMemoryItems(k int) ([]MemoryItem, error) {
	if k <= 0 {
		k = 20
	}
	rows, err := s.db.Query(`SELECT id, kind, content, reply, category, session_id, created_at
		FROM memory_items ORDER BY created_at DESC LIMIT ?`, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryItem
	for rows.Next() {
		var m MemoryItem
		if err := rows.Scan(&m.ID, &m.Kind, &m.Content, &m.Reply, &m.Category, &m.SessionID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListMemoryItemsSince returns manifest rows created after ts (exclusive),
// oldest first — used for incremental extraction (only turns since the last run).
func (s *Store) ListMemoryItemsSince(ts int64) ([]MemoryItem, error) {
	rows, err := s.db.Query(`SELECT id, kind, content, reply, category, session_id, created_at
		FROM memory_items WHERE created_at > ? ORDER BY created_at ASC`, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryItem
	for rows.Next() {
		var m MemoryItem
		if err := rows.Scan(&m.ID, &m.Kind, &m.Content, &m.Reply, &m.Category, &m.SessionID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMemory returns the turn and fact counts.
func (s *Store) CountMemory() (turns int, facts int) {
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM memory_items WHERE kind='turn'`).Scan(&turns)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM memory_items WHERE kind='fact'`).Scan(&facts)
	return
}

// ListLowImportanceItems returns the k least important, oldest memory items.
func (s *Store) ListLowImportanceItems(k int) ([]MemoryItem, error) {
	rows, err := s.db.Query(`SELECT id, kind, content, reply, category FROM memory_items
		WHERE importance='low' OR importance='normal'
		ORDER BY CASE importance WHEN 'low' THEN 0 WHEN 'normal' THEN 1 END ASC,
		last_access ASC LIMIT ?`, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryItem
	for rows.Next() {
		var m MemoryItem
		if err := rows.Scan(&m.ID, &m.Kind, &m.Content, &m.Reply, &m.Category); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, rows.Err()
}


// MigrateDefaultWorkspace updates legacy 'default' workspace_id to the actual
// library ID on all kg_nodes, kg_edges, and memory_items tables.
func (s *Store) MigrateDefaultWorkspace(libID string) {
	s.db.Exec(`UPDATE kg_nodes SET workspace_id=? WHERE workspace_id='default'`, libID)
	s.db.Exec(`UPDATE kg_edges SET workspace_id=? WHERE workspace_id='default'`, libID)
	s.db.Exec(`UPDATE memory_items SET workspace_id=? WHERE workspace_id='default'`, libID)
}
// ─── Dream Candidates (P9) ──────────────────────────────────────

func (s *Store) AddDreamCandidate(id, sourceID, sourceType, stage string, now int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO dream_candidates(id,source_id,source_type,stage,score,insight,created_at) VALUES(?,?,?,?,0,'',?)`, id, sourceID, sourceType, stage, now)
	return err
}

func (s *Store) PromoteDreamStage(from, to string) (int, error) {
	res, err := s.db.Exec(`UPDATE dream_candidates SET stage=? WHERE stage=?`, to, from)
	if err != nil { return 0, err }
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) ClearDreamCandidates() { s.db.Exec(`DELETE FROM dream_candidates`) }

// ClearMemory wipes the manifest and the vector collection.
func (s *Store) ClearMemory() error {
	if _, err := s.db.Exec(`DELETE FROM memory_items`); err != nil {
		return err
	}
	if s.vector != nil {
		if err := s.vector.Clear(); err != nil {
			return err
		}
	}
	return nil
}

// MigrateModel re-embeds every memory item with a new model and rebinds.
// embedBatch must load the new model once and embed in bulk (e.g. rag.EmbedChunks).
// Safe order: read all → embed all → only then clear + rewrite → rebind. If any
// embed fails, existing vectors are left untouched.
func (s *Store) MigrateModel(newDir string, embedBatch func([]string) ([][]float32, error)) error {
	if s.vector == nil {
		return fmt.Errorf("向量层未就绪")
	}
	rows, err := s.db.Query(`SELECT id, kind, content, reply, category, session_id FROM memory_items ORDER BY created_at`)
	if err != nil {
		return err
	}
	var items []MemoryItem
	for rows.Next() {
		var it MemoryItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Content, &it.Reply, &it.Category, &it.SessionID); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(items) == 0 {
		return s.SetEmbeddingModel(newDir) // no data — just rebind
	}
	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.Content
	}
	vecs, err := embedBatch(texts)
	if err != nil {
		return fmt.Errorf("重新嵌入失败: %w", err)
	}
	if len(vecs) != len(items) {
		return fmt.Errorf("嵌入数量不匹配: %d vs %d", len(vecs), len(items))
	}
	if err := s.vector.Clear(); err != nil {
		return err
	}
	for i, it := range items {
		if it.Kind == "turn" {
			if err := s.vector.AddTurn(it.ID, it.Content, it.Reply, it.SessionID, it.ID, vecs[i]); err != nil {
				log.Printf("[memory] 迁移写入失败 %s: %v", it.ID, err)
			}
		} else {
			if err := s.vector.AddFact(it.ID, it.Content, it.Category, it.ID, vecs[i]); err != nil {
				log.Printf("[memory] 迁移写入失败 %s: %v", it.ID, err)
			}
		}
	}
	return s.SetEmbeddingModel(newDir)
}

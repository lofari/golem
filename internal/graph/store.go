package graph

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// Node represents a code entity in the graph.
type Node struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// Stats holds graph statistics.
type Stats struct {
	TotalNodes int            `json:"totalNodes"`
	TotalEdges int            `json:"totalEdges"`
	NodeTypes  map[string]int `json:"nodeTypes"`
	EdgeTypes  map[string]int `json:"edgeTypes"`
}

// Store is the SQLite-backed graph storage.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates a graph database at the given path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening graph db: %w", err)
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	schema := `
		CREATE TABLE IF NOT EXISTS nodes (
			id   TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			path TEXT,
			line INTEGER
		);
		CREATE TABLE IF NOT EXISTS edges (
			from_node TEXT NOT NULL,
			to_node   TEXT NOT NULL,
			type      TEXT NOT NULL,
			PRIMARY KEY (from_node, to_node, type)
		);
		CREATE TABLE IF NOT EXISTS graph_meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_node);
		CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_node);
		CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(type);
		CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
		CREATE INDEX IF NOT EXISTS idx_nodes_path ON nodes(path);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// InsertBatch inserts nodes and edges in a single transaction.
func (s *Store) InsertBatch(nodes []Node, edges []Edge) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nodeStmt, err := tx.Prepare("INSERT OR REPLACE INTO nodes (id, type, name, path, line) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer nodeStmt.Close()

	for _, n := range nodes {
		if _, err := nodeStmt.Exec(n.ID, n.Type, n.Name, n.Path, n.Line); err != nil {
			return fmt.Errorf("inserting node %s: %w", n.ID, err)
		}
	}

	edgeStmt, err := tx.Prepare("INSERT OR REPLACE INTO edges (from_node, to_node, type) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer edgeStmt.Close()

	for _, e := range edges {
		if _, err := edgeStmt.Exec(e.From, e.To, e.Type); err != nil {
			return fmt.Errorf("inserting edge %s->%s: %w", e.From, e.To, err)
		}
	}

	return tx.Commit()
}

// DeleteByPath removes all nodes with the given path and their edges.
func (s *Store) DeleteByPath(path string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete edges where either end is a node from this path
	_, err = tx.Exec(`
		DELETE FROM edges WHERE from_node IN (SELECT id FROM nodes WHERE path = ?)
		OR to_node IN (SELECT id FROM nodes WHERE path = ?)
	`, path, path)
	if err != nil {
		return err
	}

	// Delete nodes
	_, err = tx.Exec("DELETE FROM nodes WHERE path = ?", path)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Clear removes all nodes and edges.
func (s *Store) Clear() error {
	_, err := s.db.Exec("DELETE FROM nodes; DELETE FROM edges;")
	return err
}

// NodesByType returns all nodes of the given type.
func (s *Store) NodesByType(nodeType string) ([]Node, error) {
	return s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE type = ?", nodeType)
}

// NodesByPath returns all nodes with the given path.
func (s *Store) NodesByPath(path string) ([]Node, error) {
	return s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE path = ?", path)
}

// NodeByID returns a single node by ID.
func (s *Store) NodeByID(id string) (*Node, error) {
	nodes, err := s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	return &nodes[0], nil
}

// FindNodesByName returns nodes matching the given name (exact match).
func (s *Store) FindNodesByName(name string) ([]Node, error) {
	return s.queryNodes("SELECT id, type, name, path, line FROM nodes WHERE name = ?", name)
}

// EdgesFrom returns all edges originating from the given node.
func (s *Store) EdgesFrom(nodeID string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE from_node = ?", nodeID)
}

// EdgesTo returns all edges pointing to the given node.
func (s *Store) EdgesTo(nodeID string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE to_node = ?", nodeID)
}

// EdgesOfType returns all edges of the given type from a node.
func (s *Store) EdgesOfType(nodeID string, edgeType string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE from_node = ? AND type = ?", nodeID, edgeType)
}

// EdgesToOfType returns all edges of the given type pointing to a node.
func (s *Store) EdgesToOfType(nodeID string, edgeType string) ([]Edge, error) {
	return s.queryEdges("SELECT from_node, to_node, type FROM edges WHERE to_node = ? AND type = ?", nodeID, edgeType)
}

// SetMeta sets a key-value pair in graph_meta.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO graph_meta (key, value) VALUES (?, ?)", key, value)
	return err
}

// GetMeta retrieves a value from graph_meta.
func (s *Store) GetMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM graph_meta WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// Stats returns graph statistics.
func (s *Store) Stats() (Stats, error) {
	var stats Stats
	stats.NodeTypes = make(map[string]int)
	stats.EdgeTypes = make(map[string]int)

	s.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&stats.TotalNodes)
	s.db.QueryRow("SELECT COUNT(*) FROM edges").Scan(&stats.TotalEdges)

	rows, err := s.db.Query("SELECT type, COUNT(*) FROM nodes GROUP BY type")
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var c int
		rows.Scan(&t, &c)
		stats.NodeTypes[t] = c
	}

	rows, err = s.db.Query("SELECT type, COUNT(*) FROM edges GROUP BY type")
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var c int
		rows.Scan(&t, &c)
		stats.EdgeTypes[t] = c
	}

	return stats, nil
}

func (s *Store) queryNodes(query string, args ...any) ([]Node, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Type, &n.Name, &n.Path, &n.Line); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *Store) queryEdges(query string, args ...any) ([]Edge, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.From, &e.To, &e.Type); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, nil
}

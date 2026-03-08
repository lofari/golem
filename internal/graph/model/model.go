package model

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

// Commit represents a git commit.
type Commit struct {
	SHA         string `json:"sha"`
	Message     string `json:"message"`
	AuthorEmail string `json:"authorEmail"`
	Timestamp   int64  `json:"timestamp"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
}

// Author represents a git author.
type Author struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// CoChangedResult represents a file that co-changed with another file.
type CoChangedResult struct {
	File  string `json:"file"`
	Count int    `json:"count"`
}

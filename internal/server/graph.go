package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
	"github.com/lofari/golem/internal/graph/query"
)

func (s *Server) openProjectGraph(p *project) (*graph.Store, error) {
	dbPath := filepath.Join(p.path, ".ctx", "graph.db")
	return graph.OpenStore(dbPath)
}

func (s *Server) handleGraphRelated(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter is required")
		return
	}

	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "all"
	}
	if direction != "callers" && direction != "dependencies" && direction != "dependents" && direction != "all" {
		writeError(w, http.StatusBadRequest, "direction must be callers, dependencies, dependents, or all")
		return
	}

	depth := 1
	if d := r.URL.Query().Get("depth"); d != "" {
		if v, err := strconv.Atoi(d); err == nil {
			depth = v
		}
	}

	store, err := s.openProjectGraph(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "graph not found — run golem graph build")
		return
	}
	defer store.Close()

	result, err := query.Related(store, name, direction, depth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGraphSearch(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	var body struct {
		Query string   `json:"query"`
		Limit int      `json:"limit"`
		Types []string `json:"types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	store, err := s.openProjectGraph(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "graph not found — run golem graph build")
		return
	}
	defer store.Close()

	modelDir, err := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading embedding model: "+err.Error())
		return
	}
	embedder, err := embed.NewONNXEmbedder(modelDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating embedder: "+err.Error())
		return
	}
	defer embedder.Close()

	results, err := query.Search(store, embedder, body.Query, body.Limit, body.Types)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleGraphRuntimePath(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	sessionID := r.URL.Query().Get("session")
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "trace"
	}
	if mode != "trace" && mode != "failures" {
		writeError(w, http.StatusBadRequest, "mode must be trace or failures")
		return
	}
	cmdFilter := r.URL.Query().Get("command_filter")

	store, err := s.openProjectGraph(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "graph not found — run golem graph build")
		return
	}
	defer store.Close()

	result, err := query.RuntimePath(store, sessionID, mode, cmdFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

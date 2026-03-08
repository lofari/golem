package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lofari/golem/internal/config"
	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/git"
)

// ProjectInfo is the API representation of a registered project.
type ProjectInfo struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

// project holds internal state for a registered project.
type project struct {
	id   string
	path string
}

// RegisterProject adds a project directory to the registry.
func (s *Server) RegisterProject(dir string) error {
	ctxDir := filepath.Join(dir, ".ctx")
	if _, err := os.Stat(ctxDir); err != nil {
		return fmt.Errorf("no .ctx/ directory at %s", dir)
	}
	id := s.projectID(dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[id] = &project{id: id, path: dir}
	return nil
}

func (s *Server) projectID(dir string) string {
	h := sha256.Sum256([]byte(dir))
	return hex.EncodeToString(h[:8])
}

func (s *Server) getProject(id string) (*project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	return p, ok
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var infos []ProjectInfo
	for _, p := range s.projects {
		state, err := golemctx.ReadState(p.path)
		info := ProjectInfo{ID: p.id, Path: p.path}
		if err == nil {
			info.Name = state.Project.Name
			info.Phase = state.Status.Phase
		}
		infos = append(infos, info)
	}
	writeJSON(w, http.StatusOK, infos)
}

func (s *Server) handleRegisterProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.RegisterProject(body.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := s.projectID(body.Path)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	state, err := golemctx.ReadState(p.path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleGetLog(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	log, err := golemctx.ReadLog(p.path)
	if err != nil {
		// Return empty log if file doesn't exist yet (new project)
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			writeJSON(w, http.StatusOK, golemctx.Log{})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, log)
}

func (s *Server) handleGetProjectConfig(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	cfg := config.Load(config.GlobalPath(), config.ProjectPath(p.path))
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleUpdateProjectConfig(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := config.WriteFile(config.ProjectPath(p.path), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleGetGlobalConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Load(config.GlobalPath(), "")
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	baseRef := r.URL.Query().Get("ref")
	file := r.URL.Query().Get("file")

	// If a specific file is requested, return its patch
	if file != "" {
		patch, err := git.DiffPatch(p.path, baseRef, file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"patch": patch})
		return
	}

	summary, err := git.DiffSummary(p.path, baseRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleUpdateGlobalConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := config.WriteFile(config.GlobalPath(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

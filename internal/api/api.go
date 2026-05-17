package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Server struct {
	runDir string
	static http.Handler
}

func NewServer(runDir string, staticFS fs.FS) (*Server, error) {
	if err := requireRunArtifacts(runDir); err != nil {
		return nil, err
	}
	return &Server{runDir: runDir, static: http.FileServer(http.FS(staticFS))}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/runs/current", s.runSummary)
	mux.HandleFunc("/api/runs/current/manifest", s.jsonArtifact("manifest.json"))
	mux.HandleFunc("/api/runs/current/metrics", s.jsonArtifact("metrics.json"))
	mux.HandleFunc("/api/runs/current/events", s.jsonArtifact("events.json"))
	mux.HandleFunc("/api/runs/current/timeseries", s.timeseries)
	mux.Handle("/static/", http.StripPrefix("/static/", s.static))
	mux.HandleFunc("/", s.index)
	return mux
}

func StaticDir(dir string) http.FileSystem { return http.Dir(dir) }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) runSummary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"id": "current", "artifact_dir": s.runDir})
}

func (s *Server) jsonArtifact(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		data, err := os.ReadFile(filepath.Join(s.runDir, name))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}
}

func (s *Server) timeseries(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "controlled"
	}
	name := mode + "_timeseries.csv"
	if mode != "baseline" && mode != "controlled" {
		http.Error(w, "mode must be baseline or controlled", http.StatusBadRequest)
		return
	}
	rows, err := readTimeseries(filepath.Join(s.runDir, name))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.static.ServeHTTP(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join("web", "static", "index.html"))
}

func readTimeseries(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open timeseries %q: %w", path, err)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read timeseries %q: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("timeseries %q is empty", path)
	}
	header := records[0]
	out := make([]map[string]any, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]any{}
		for i, name := range header {
			value := ""
			if i < len(record) {
				value = record[i]
			}
			row[name] = parseCell(value)
		}
		out = append(out, row)
	}
	return out, nil
}

func parseCell(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "true" {
		return true
	}
	if trimmed == "false" {
		return false
	}
	if v, err := strconv.ParseFloat(trimmed, 64); err == nil && strings.ContainsAny(trimmed, ".0123456789") && !strings.Contains(trimmed, "T") {
		return v
	}
	return value
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func requireRunArtifacts(runDir string) error {
	for _, name := range []string{"manifest.json", "baseline_timeseries.csv", "controlled_timeseries.csv", "metrics.json"} {
		path := filepath.Join(runDir, name)
		if info, err := os.Stat(path); err != nil {
			return fmt.Errorf("run artifact %q is required: %w", path, err)
		} else if info.IsDir() {
			return fmt.Errorf("run artifact %q is a directory", path)
		}
	}
	return nil
}

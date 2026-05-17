package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"grid-dynamics-modelling/internal/scenario"
	"grid-dynamics-modelling/internal/sim"
)

func TestServerExposesRunArtifacts(t *testing.T) {
	cfg, err := scenario.LoadJSON("../../scenarios/demo.json")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Simulation.Duration = "5s"
	cfg.PMU.File = "../../" + cfg.PMU.File
	cfg.Workload.Source = "../../" + cfg.Workload.Source
	result, err := sim.Run(cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	dir := t.TempDir()
	if err := sim.WriteBaselineArtifacts(dir, result); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}
	server, err := NewServer(dir, fstest.MapFS{"index.html": {Data: []byte("ok")}, "app.js": {Data: []byte("console.log('ok')")}})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	h := server.Handler()
	for _, path := range []string{"/api/health", "/api/runs/current/manifest", "/api/runs/current/metrics", "/api/runs/current/timeseries?mode=controlled"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s code=%d body=%s", path, rec.Code, rec.Body.String())
		}
		var payload any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s invalid JSON: %v body=%s", path, err, rec.Body.String())
		}
	}
}

func TestServerRejectsBadTimeseriesMode(t *testing.T) {
	cfg, err := scenario.LoadJSON("../../scenarios/demo.json")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Simulation.Duration = "1s"
	cfg.PMU.File = "../../" + cfg.PMU.File
	cfg.Workload.Source = "../../" + cfg.Workload.Source
	result, err := sim.Run(cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	dir := t.TempDir()
	if err := sim.WriteBaselineArtifacts(dir, result); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}
	server, err := NewServer(dir, fstest.MapFS{"index.html": {Data: []byte("ok")}})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/runs/current/timeseries?mode=nope", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

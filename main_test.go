package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/powerscale"
)

func TestLivezReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyzReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthReturns200WhenAllClustersDown(t *testing.T) {
	store := powerscale.NewSnapshotStore()
	store.Store(powerscale.BuildSnapshot([]*powerscale.ClusterSnapshot{
		{Cluster: "pscale-01", Up: false, ScrapeError: "login POST: status 401", LastScrape: time.Now()},
	}))
	server := &Server{store: store}

	rec := httptest.NewRecorder()
	server.healthHandler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Clusters []struct {
			Cluster string `json:"cluster"`
			OK      bool   `json:"ok"`
			Err     string `json:"err"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Clusters) != 1 || body.Clusters[0].OK {
		t.Fatalf("clusters = %+v, want one cluster with ok=false", body.Clusters)
	}
	if body.Clusters[0].Err == "" {
		t.Fatalf("err field empty, want the scrape failure message")
	}
}

func TestHealthReturns200BeforeFirstCycle(t *testing.T) {
	server := &Server{store: powerscale.NewSnapshotStore()}

	rec := httptest.NewRecorder()
	server.healthHandler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

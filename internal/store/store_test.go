package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateUser("akena", "hash", RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	m := Monitor{
		OwnerID: u.ID, Name: "Test", Type: TypeHTTP, URL: "https://example.com",
		Method: "GET", ExpectedStatus: 200, TimeoutS: 5, IntervalS: 60,
		Active: true, Notify: true, MaxRetries: 1,
	}
	created, err := s.CreateMonitor(m)
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}

	got, err := s.GetMonitor(created.ID)
	if err != nil {
		t.Fatalf("GetMonitor: %v", err)
	}
	if got.Name != "Test" || !got.Active || !got.Notify {
		t.Fatalf("monitor inesperado: %+v", got)
	}

	if err := s.InsertHeartbeat(Heartbeat{
		MonitorID: created.ID, Status: StatusUp, LatencyMS: 42, CheckedAt: time.Now(),
	}); err != nil {
		t.Fatalf("InsertHeartbeat: %v", err)
	}
	up, total, err := s.Uptime(created.ID, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("Uptime: %v", err)
	}
	if up != 1 || total != 1 {
		t.Fatalf("uptime = %d/%d, esperado 1/1", up, total)
	}

	viewers, err := s.ListMonitorViewerIDs(created.ID)
	if err != nil {
		t.Fatalf("ListMonitorViewerIDs: %v", err)
	}
	if len(viewers) != 1 || viewers[0] != u.ID {
		t.Fatalf("viewers = %v, esperado [%d]", viewers, u.ID)
	}
}

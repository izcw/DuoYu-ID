package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func testPaginationSnapshot(t *testing.T, s *Server) {
	ctx := context.Background()
	_, err := s.db.Exec(ctx, `CREATE TABLE paging_fixture(id text PRIMARY KEY);
		INSERT INTO paging_fixture VALUES('one');
		CREATE FUNCTION paging_gate() RETURNS boolean LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(11706201); RETURN true; END$$`)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := s.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(ctx)
	var pid int
	if err := writer.QueryRow(ctx, "SELECT pg_backend_pid() FROM pg_advisory_xact_lock(11706201)").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.paginatedList(response, httptest.NewRequest("GET", "/?page=1", nil), "SELECT id FROM paging_fixture WHERE paging_gate() ORDER BY id LIMIT 200", nil)
	}()
	waitForDatabaseWaiters(t, s, pid, 1)
	if _, err := writer.Exec(ctx, "INSERT INTO paging_fixture VALUES('two')"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	<-done
	var out struct {
		Data struct {
			Total int
			Items []struct{ ID string }
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || out.Data.Total != 1 || len(out.Data.Items) != 1 || out.Data.Items[0].ID != "one" {
		t.Fatalf("count and rows disagree: HTTP %d, total %d, rows %v", response.Code, out.Data.Total, out.Data.Items)
	}
}

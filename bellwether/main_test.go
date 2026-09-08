package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests run against the in-memory fallback (no DATABASE_URL): they prove the app's
// behavior, not Postgres connectivity — that's the release harness's job (V2).

func testApp(t *testing.T) *app {
	t.Helper()
	a, err := newApp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.close)
	return a
}

func TestHealthzWithoutDB(t *testing.T) {
	a := testApp(t)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]string
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["status"] != "ok" || body["db"] != "not-configured" {
		t.Errorf("unexpected health body: %v", body)
	}
}

func TestNoteLifecycle(t *testing.T) {
	a := testApp(t)
	h := a.routes()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/notes", strings.NewReader(`{"body":"hello"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/notes", nil))
	var notes []note
	json.Unmarshal(rec.Body.Bytes(), &notes)
	if len(notes) != 1 || notes[0].Body != "hello" || notes[0].Processed {
		t.Fatalf("unexpected notes: %+v", notes)
	}

	n, err := a.processPending(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("processPending = %d, %v", n, err)
	}

	var out strings.Builder
	if err := a.report(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1 note(s), 1 processed") {
		t.Errorf("unexpected report: %q", out.String())
	}
}

func TestCreateNoteRejectsEmptyBody(t *testing.T) {
	a := testApp(t)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, httptest.NewRequest("POST", "/notes", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d", rec.Code)
	}
}

// bellwether is the representative reference application for neckbeard's release
// matrix (DESIGN §12.3): one binary, three modes matching the workload contract —
// an HTTP service, a background worker, and a scheduled job — using PostgreSQL when
// configured and degrading honestly when not.
//
//	bellwether serve    HTTP service on $PORT (default 8080)
//	bellwether work     worker loop: processes pending notes
//	bellwether report   scheduled job: prints a summary and exits
//
// Configuration (env): PORT, DATABASE_URL (optional), APP_ENV (dev|stg|prd).
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bellwether <serve|work|report>")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := newApp(ctx)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	defer app.close()

	switch os.Args[1] {
	case "serve":
		err = app.serve(ctx)
	case "work":
		err = app.work(ctx)
	case "report":
		err = app.report(ctx, os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, "usage: bellwether <serve|work|report>")
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

type note struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	Processed bool      `json:"processed"`
	CreatedAt time.Time `json:"created_at"`
}

// app holds either a real database or an in-memory fallback, so the binary is
// exercisable locally and in CI without infrastructure while remaining a genuine
// Postgres consumer when DATABASE_URL is set.
type app struct {
	env string
	db  *sql.DB

	mu    sync.Mutex
	notes []note
	next  int64
}

func newApp(ctx context.Context) (*app, error) {
	a := &app{env: envOr("APP_ENV", "dev"), next: 1}
	// TrimSpace: secret stores cannot represent empty values (Key Vault rejects
	// them), so a blank-ish secret means "no database" — V2 finding, 2026-09-08.
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return a, nil
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notes (
		id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		body TEXT NOT NULL,
		processed BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, fmt.Errorf("ensuring schema: %w", err)
	}
	a.db = db
	return a, nil
}

func (a *app) close() {
	if a.db != nil {
		a.db.Close()
	}
}

// --- serve ---

func (a *app) serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:              ":" + envOr("PORT", "8080"),
		Handler:           a.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Printf("bellwether %s serving on %s (env %s, db: %s)", version, srv.Addr, a.env, a.dbState())

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /", a.handleRoot)
	mux.HandleFunc("GET /notes", a.handleListNotes)
	mux.HandleFunc("POST /notes", a.handleCreateNote)
	return mux
}

func (a *app) handleHealthz(w http.ResponseWriter, r *http.Request) {
	status := http.StatusOK
	dbState := a.dbState()
	if a.db != nil {
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := a.db.PingContext(pingCtx); err != nil {
			status, dbState = http.StatusServiceUnavailable, "unreachable"
		}
	}
	writeJSON(w, status, map[string]string{
		"status":  map[bool]string{true: "ok", false: "degraded"}[status == http.StatusOK],
		"env":     a.env,
		"version": version,
		"db":      dbState,
	})
}

func (a *app) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"app": "bellwether", "version": version, "env": a.env})
}

func (a *app) handleListNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := a.listNotes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

func (a *app) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Body == "" {
		http.Error(w, "body required", http.StatusBadRequest)
		return
	}
	n, err := a.createNote(r.Context(), in.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

// --- work ---

// work polls for unprocessed notes and marks them processed: the queue-less,
// DB-backed worker shape the workload contract supports (DESIGN §3.1).
func (a *app) work(ctx context.Context) error {
	log.Printf("bellwether %s worker started (env %s, db: %s)", version, a.env, a.dbState())
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Print("worker: shutting down")
			return nil
		case <-ticker.C:
			n, err := a.processPending(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				log.Printf("worker: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("worker: processed %d note(s)", n)
			}
		}
	}
}

// --- report ---

func (a *app) report(ctx context.Context, out interface{ Write([]byte) (int, error) }) error {
	total, processed, err := a.counts(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "bellwether report (env %s): %d note(s), %d processed\n", a.env, total, processed)
	return err
}

// --- storage ---

func (a *app) dbState() string {
	if a.db == nil {
		return "not-configured"
	}
	return "ok"
}

func (a *app) listNotes(ctx context.Context) ([]note, error) {
	if a.db == nil {
		a.mu.Lock()
		defer a.mu.Unlock()
		out := make([]note, len(a.notes))
		copy(out, a.notes)
		return out, nil
	}
	rows, err := a.db.QueryContext(ctx, `SELECT id, body, processed, created_at FROM notes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []note{}
	for rows.Next() {
		var n note
		if err := rows.Scan(&n.ID, &n.Body, &n.Processed, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (a *app) createNote(ctx context.Context, body string) (note, error) {
	if a.db == nil {
		a.mu.Lock()
		defer a.mu.Unlock()
		n := note{ID: a.next, Body: body, CreatedAt: time.Now().UTC()}
		a.next++
		a.notes = append(a.notes, n)
		return n, nil
	}
	var n note
	err := a.db.QueryRowContext(ctx,
		`INSERT INTO notes (body) VALUES ($1) RETURNING id, body, processed, created_at`, body).
		Scan(&n.ID, &n.Body, &n.Processed, &n.CreatedAt)
	return n, err
}

func (a *app) processPending(ctx context.Context) (int64, error) {
	if a.db == nil {
		a.mu.Lock()
		defer a.mu.Unlock()
		var n int64
		for i := range a.notes {
			if !a.notes[i].Processed {
				a.notes[i].Processed = true
				n++
			}
		}
		return n, nil
	}
	res, err := a.db.ExecContext(ctx, `UPDATE notes SET processed = TRUE WHERE NOT processed`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (a *app) counts(ctx context.Context) (total, processed int64, err error) {
	if a.db == nil {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, n := range a.notes {
			total++
			if n.Processed {
				processed++
			}
		}
		return total, processed, nil
	}
	err = a.db.QueryRowContext(ctx,
		`SELECT count(*), count(*) FILTER (WHERE processed) FROM notes`).Scan(&total, &processed)
	return total, processed, err
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

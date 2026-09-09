package analyze

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bealesh/neckbeard/schemas"
)

// The repo's own bellwether app is the fixture: a Dockerfile with EXPOSE, a
// DATABASE_URL read, and a /healthz literal.
func TestAnalyzeBellwether(t *testing.T) {
	res, err := Dir("../../bellwether")
	if err != nil {
		t.Fatal(err)
	}
	p := res.Profile

	if len(p.Services) != 1 {
		t.Fatalf("want 1 service (one Dockerfile), got %d", len(p.Services))
	}
	svc := p.Services[0]
	if svc.Port != 8080 {
		t.Errorf("EXPOSE 8080 should be detected, got %d", svc.Port)
	}
	if svc.HealthPath != "/healthz" {
		t.Errorf("health path literal should be detected, got %q", svc.HealthPath)
	}

	var hasPostgres bool
	for _, n := range p.Needs {
		if n.Capability == "postgres" {
			hasPostgres = true
			if len(n.Evidence) == 0 {
				t.Error("postgres need must carry evidence")
			}
		}
	}
	if !hasPostgres {
		t.Error("DATABASE_URL read should yield a postgres need")
	}

	var hasDBSecret bool
	for _, s := range p.Secrets {
		if s.Name == "DATABASE_URL" {
			hasDBSecret = true
		}
		if s.Name == "PORT" || s.Name == "APP_ENV" {
			t.Errorf("%s must not be classified as a secret", s.Name)
		}
	}
	if !hasDBSecret {
		t.Error("DATABASE_URL should surface as a secret NAME")
	}

	// The provision-vs-reference question is never auto-answered (DESIGN §8).
	var modeAssumed bool
	for _, a := range p.Assumptions {
		if a.ID == "postgres-mode" {
			modeAssumed = true
		}
	}
	if !modeAssumed {
		t.Error("postgres provision mode must be flagged as an assumption")
	}
	if len(res.Questions) == 0 {
		t.Error("analysis must surface open questions (traffic, workers, RPO/RTO)")
	}
}

// The draft must be a valid app-profile so `neckbeard plan` runs immediately.
func TestDraftValidatesAgainstSchema(t *testing.T) {
	res, err := Dir("../../bellwether")
	if err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(res.Profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.ValidateYAML("app-profile", out); err != nil {
		t.Fatalf("draft profile violates the schema: %v\n%s", err, out)
	}
}

func TestClassifyEnv(t *testing.T) {
	cases := map[string]envClass{
		"DATABASE_URL":    classPostgres,
		"REDIS_URL":       classUnsupported,
		"KAFKA_BROKERS":   classUnsupported,
		"MYSQL_HOST":      classUnsupported,
		"S3_BUCKET":       classObjectStorage,
		"SECRET_KEY_BASE": classSecret,
		"STRIPE_API_KEY":  classSecret,
		"SENTRY_DSN":      classSecret,
		"PORT":            classPort,
		"APP_ENV":         classIgnore,
		"LOG_LEVEL":       classIgnore,
	}
	for name, want := range cases {
		if got, _ := classifyEnv(name); got != want {
			t.Errorf("classifyEnv(%s) = %v, want %v", name, got, want)
		}
	}
}

func TestUnsupportedIsNamedNotForced(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 3000\n")
	writeFile(t, dir, "app.js", `const r = process.env.REDIS_URL; const s = process.env.SESSION_SECRET;`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profile.Unsupported) != 1 || res.Profile.Unsupported[0].Capability != "redis" {
		t.Fatalf("redis must land in unsupported, got %+v", res.Profile.Unsupported)
	}
	for _, n := range res.Profile.Needs {
		if n.Capability == "redis" {
			t.Error("unsupported capabilities must never become needs")
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(dir+"/"+name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

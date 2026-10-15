package analyze

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bealesh/neckbeard/core/profile"
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

// Drafts are schema-valid but planning still requires resolving their assumptions.
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

// Real-repo shapes from the 2026-09-11 dogfood sweep (mastodon, saleor,
// outline): devcontainers and base images must not become draft services, and
// a root Dockerfile wins with the rest surfaced as a plan-gating assumption.
func TestRootDockerfileWinsAndOthersGateAsAssumption(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 3000\n")
	writeFile(t, dir, "Dockerfile.base", "FROM debian\n")
	if err := os.MkdirAll(dir+"/streaming", 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "streaming/Dockerfile", "FROM node\nEXPOSE 4000\n")
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profile.Services) != 1 || res.Profile.Services[0].Dockerfile != "Dockerfile" {
		t.Fatalf("root Dockerfile must be the single draft service, got %+v", res.Profile.Services)
	}
	found := false
	for _, a := range res.Profile.Assumptions {
		if a.ID == "container-images" {
			found = true
			if !strings.Contains(a.Statement, "Dockerfile.base") || !strings.Contains(a.Statement, "streaming/Dockerfile") {
				t.Errorf("assumption must name the demoted builds, got %q", a.Statement)
			}
		}
	}
	if !found {
		t.Fatal("expected a plan-gating container-images assumption")
	}
}

func TestDotDirectoriesAreNeverServices(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/.devcontainer", 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, ".devcontainer/Dockerfile", "FROM x\nEXPOSE 8080\n")
	writeFile(t, dir, "app.js", "process.env.SESSION_SECRET")
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profile.Services) != 0 {
		t.Fatalf("devcontainer Dockerfile must not become a service, got %+v", res.Profile.Services)
	}
}

// Without a root Dockerfile there is no convention to lean on: every candidate
// stays a visible service and the multi-image refusal happens in the workload
// gate, not silently here.
func TestNoRootDockerfileKeepsAllCandidates(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"packaging/debian", "packaging/rpm"} {
		if err := os.MkdirAll(dir+"/"+d, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, d+"/Dockerfile", "FROM x\n")
	}
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profile.Services) != 2 {
		t.Fatalf("expected both packaging candidates visible, got %+v", res.Profile.Services)
	}
}

func TestTypedEnvWrapperAndTemplateDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 3000\n")
	writeFile(t, dir, "redis.ts", `const tls = (env.REDIS_URL || "").startsWith("rediss://");`)
	writeFile(t, dir, ".env.sample", "AWS_S3_UPLOAD_BUCKET_NAME=bucket_here\nSECRET_KEY=change_me\n")
	writeFile(t, dir, ".env", "REAL_LIVE_TOKEN=supersecret\n")
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	foundRedis, foundStorage := false, false
	for _, u := range res.Profile.Unsupported {
		if u.Capability == "redis" {
			foundRedis = true
		}
	}
	for _, n := range res.Profile.Needs {
		if n.Capability == "object-storage" {
			foundStorage = true
		}
	}
	if !foundRedis {
		t.Error("env.REDIS_URL through a typed wrapper must surface as unsupported redis")
	}
	if !foundStorage {
		t.Error("a bucket variable declared in .env.sample must surface as object storage")
	}
	for _, f := range res.Profile.Facts {
		if strings.Contains(f.Statement, "REAL_LIVE_TOKEN") {
			t.Error("a real .env file must never be scanned")
		}
	}
	for _, s := range res.Profile.Secrets {
		if s.Name == "REAL_LIVE_TOKEN" {
			t.Error("a real .env file must never feed secret names")
		}
	}
}

func TestDatabaseURLLiteralBecomesInferenceNotFact(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 4000\n")
	writeFile(t, dir, "runtime.exs", `db = get_var_from_path_or_env(dir, "DATABASE_URL")`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	needFound := false
	for _, n := range res.Profile.Needs {
		if n.Capability == "postgres" && n.Mode == "undecided" {
			needFound = true
		}
	}
	if !needFound {
		t.Fatal("quoted DATABASE_URL literal must produce an undecided postgres need")
	}
	inferred := false
	for _, inf := range res.Profile.Inferences {
		if inf.ID == "postgres-indirect" && inf.Confidence == "medium" {
			inferred = true
		}
	}
	if !inferred {
		t.Error("indirect detection must land as a medium-confidence inference, not a fact")
	}
	for _, f := range res.Profile.Facts {
		if strings.Contains(f.Statement, "DATABASE_URL") {
			t.Error("no fact may claim a direct DATABASE_URL read that was not observed")
		}
	}
}

func TestManifestDatastoreHeuristics(t *testing.T) {
	const manifestAssumption = "Database dependency found in a package manifest: verify the engine and choose provision or reference; a dependency does not establish a need for a new database."
	cases := []struct {
		name         string
		files        map[string]string
		wantPostgres bool
		wantORM      bool
	}{
		{
			name: "python-dj-database-url",
			files: map[string]string{
				"Dockerfile":       "FROM x\nEXPOSE 8000\n",
				"requirements.txt": "Django==4.2\ndj-database-url==2.1.0\n",
			},
			wantPostgres: true,
		},
		{
			name: "python-django-environ-pyproject",
			files: map[string]string{
				"Dockerfile":     "FROM x\nEXPOSE 8000\n",
				"pyproject.toml": "[project]\ndependencies = [\"django-environ>=0.11\"]\n",
			},
			wantPostgres: true,
		},
		{
			name: "node-prisma",
			files: map[string]string{
				"Dockerfile":   "FROM x\nEXPOSE 3000\n",
				"package.json": "{\n  \"dependencies\": {\n    \"prisma\": \"^5.0.0\"\n  }\n}\n",
			},
			wantORM: true,
		},
		{
			name: "node-pg",
			files: map[string]string{
				"Dockerfile":   "FROM x\nEXPOSE 3000\n",
				"package.json": "{\n  \"dependencies\": {\n    \"pg\": \"^8.11.0\"\n  }\n}\n",
			},
			wantPostgres: true,
		},
		{
			name: "node-pg-devDependencies",
			files: map[string]string{
				"Dockerfile":   "FROM x\nEXPOSE 3000\n",
				"package.json": "{\n  \"devDependencies\": {\n    \"pg\": \"^8.11.0\"\n  }\n}\n",
			},
			wantPostgres: true,
		},
		{
			name: "node-sequelize",
			files: map[string]string{
				"Dockerfile":   "FROM x\nEXPOSE 3000\n",
				"package.json": "{\n  \"dependencies\": {\n    \"sequelize\": \"^6.0.0\"\n  }\n}\n",
			},
			wantORM: true,
		},
		{
			name: "node-pg-and-prisma-prefers-postgres",
			files: map[string]string{
				"Dockerfile":   "FROM x\nEXPOSE 3000\n",
				"package.json": "{\n  \"dependencies\": {\n    \"pg\": \"^8.11.0\",\n    \"prisma\": \"^5.0.0\"\n  }\n}\n",
			},
			wantPostgres: true,
		},
		{
			name: "elixir-ecto-and-postgrex",
			files: map[string]string{
				"Dockerfile": "FROM x\nEXPOSE 4000\n",
				"mix.exs":    "defp deps do\n  [{:ecto_sql, \"~> 3.10\"}, {:postgrex, \">= 0.0.0\"}]\nend\n",
			},
			wantPostgres: true,
		},
		{
			name: "elixir-ecto-alone-not-enough",
			files: map[string]string{
				"Dockerfile": "FROM x\nEXPOSE 4000\n",
				"mix.exs":    "defp deps do\n  [{:ecto_sql, \"~> 3.10\"}]\nend\n",
			},
		},
		{
			name: "ruby-activerecord-and-pg",
			files: map[string]string{
				"Dockerfile": "FROM x\nEXPOSE 3000\n",
				"Gemfile":    "gem \"activerecord\", \"~> 7.0\"\ngem \"pg\", \"~> 1.5\"\n",
			},
			wantPostgres: true,
		},
		{
			name: "ruby-pg-alone-not-enough",
			files: map[string]string{
				"Dockerfile": "FROM x\nEXPOSE 3000\n",
				"Gemfile":    "gem \"pg\", \"~> 1.5\"\n",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				writeFile(t, dir, name, content)
			}
			res, err := Dir(dir)
			if err != nil {
				t.Fatal(err)
			}
			needFound, inferred, factClaim, ormInferred := false, false, false, false
			var needEv []string
			var reasoning, assumption, ormStatement, ormReasoning string
			for _, n := range res.Profile.Needs {
				if n.Capability == "postgres" && n.Mode == "undecided" {
					needFound = true
					for _, e := range n.Evidence {
						needEv = append(needEv, e.File)
					}
				}
			}
			for _, inf := range res.Profile.Inferences {
				if inf.ID == "postgres-manifest" && inf.Confidence == "medium" {
					inferred = true
					reasoning = inf.Reasoning
				}
				if inf.ID == "database-orm-manifest" {
					ormInferred = true
					ormStatement = inf.Statement
					ormReasoning = inf.Reasoning
				}
			}
			for _, a := range res.Profile.Assumptions {
				if a.ID == "postgres-mode" {
					assumption = a.Statement
				}
			}
			for _, f := range res.Profile.Facts {
				if strings.Contains(strings.ToLower(f.Statement), "postgres") || strings.Contains(f.Statement, "dj-database-url") {
					factClaim = true
				}
			}
			if tc.wantPostgres {
				if !needFound {
					t.Fatalf("expected undecided postgres need, evidence=%v", needEv)
				}
				if !inferred {
					t.Fatal("expected medium-confidence postgres-manifest inference")
				}
				if factClaim {
					t.Fatal("manifest markers must not become facts")
				}
				if len(needEv) == 0 {
					t.Fatal("postgres need must cite the manifest file as evidence")
				}
				if assumption != manifestAssumption {
					t.Fatalf("manifest postgres-mode must not claim an env var was detected, got %q", assumption)
				}
				if _, ok := tc.files["package.json"]; ok && !strings.Contains(reasoning, "dev-only") {
					t.Fatal("node manifest reasoning must note the dependency may be dev-only")
				}
			} else if needFound || inferred {
				t.Fatalf("must not infer postgres (need=%v inf=%v)", needFound, inferred)
			}
			if tc.wantORM {
				if !ormInferred {
					t.Fatal("expected database-orm-manifest inference")
				}
				if !strings.Contains(ormStatement, "engine is unverified") {
					t.Fatalf("ORM statement too strong: %q", ormStatement)
				}
				if !strings.Contains(ormReasoning, "dev-only") {
					t.Fatal("ORM reasoning must note the dependency may be dev-only")
				}
			} else if ormInferred {
				t.Fatal("unexpected database-orm-manifest inference")
			}
		})
	}
}

func TestManifestHeuristicYieldsToDirectEnvRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 8000\n")
	writeFile(t, dir, "requirements.txt", "dj-database-url==2.1.0\n")
	writeFile(t, dir, "app.py", `url = os.environ["DATABASE_URL"]`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, inf := range res.Profile.Inferences {
		if inf.ID == "postgres-manifest" {
			t.Fatal("direct DATABASE_URL env read must win; no manifest inference")
		}
	}
	found := false
	for _, n := range res.Profile.Needs {
		if n.Capability == "postgres" {
			found = true
		}
	}
	if !found {
		t.Fatal("direct env read should still yield a postgres need")
	}
}

func TestORMHeuristicYieldsToDirectEnvRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 3000\n")
	writeFile(t, dir, "package.json", "{\n  \"dependencies\": {\n    \"prisma\": \"^5.0.0\"\n  }\n}\n")
	writeFile(t, dir, "app.js", `const url = process.env.DATABASE_URL`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, inf := range res.Profile.Inferences {
		if inf.ID == "database-orm-manifest" || inf.ID == "postgres-manifest" {
			t.Fatalf("direct DATABASE_URL env read must win; got %s", inf.ID)
		}
	}
	found := false
	for _, n := range res.Profile.Needs {
		if n.Capability == "postgres" {
			found = true
		}
	}
	if !found {
		t.Fatal("direct env read should still yield a postgres need")
	}
}

func TestComposeImagesBecomeInferencesAndUnsupported(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 8080\n")
	writeFile(t, dir, "docker-compose.yml", `services:
  web:
    build: .
    ports:
      - "8080:8080"
  db:
    image: postgres:16
  cache:
    image: redis:7
  broker:
    image: rabbitmq:3
  blobs:
    image: minio/minio:latest
  legacy:
    image: mysql:8
`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}

	needCaps := map[string]bool{}
	for _, n := range res.Profile.Needs {
		needCaps[n.Capability] = true
		if n.Capability == "postgres" || n.Capability == "object-storage" {
			if n.Mode != "undecided" {
				t.Fatalf("%s need must be undecided, got %q", n.Capability, n.Mode)
			}
			if len(n.Evidence) == 0 || n.Evidence[0].File != "docker-compose.yml" {
				t.Fatalf("%s evidence should point at compose file, got %+v", n.Capability, n.Evidence)
			}
		}
	}
	if !needCaps["postgres"] {
		t.Error("postgres compose image must create an undecided postgres need")
	}
	if !needCaps["object-storage"] {
		t.Error("minio compose image must create an undecided object-storage need")
	}

	infIDs := map[string]bool{}
	for _, inf := range res.Profile.Inferences {
		infIDs[inf.ID] = true
		if strings.HasPrefix(inf.ID, "compose-") && inf.Confidence != "medium" {
			t.Errorf("%s confidence want medium, got %s", inf.ID, inf.Confidence)
		}
	}
	if !infIDs["compose-postgres"] || !infIDs["compose-object-storage"] {
		t.Errorf("missing compose inferences: %+v", infIDs)
	}

	unsup := map[string]bool{}
	for _, u := range res.Profile.Unsupported {
		unsup[u.Capability] = true
		if len(u.Evidence) == 0 || u.Evidence[0].File != "docker-compose.yml" {
			t.Errorf("%s unsupported evidence want compose file, got %+v", u.Capability, u.Evidence)
		}
	}
	for _, cap := range []string{"redis", "queues", "non-postgres-database"} {
		if !unsup[cap] {
			t.Errorf("compose image should surface unsupported %s", cap)
		}
	}

	// app build: . service must not invent a datastore from itself
	for _, inf := range res.Profile.Inferences {
		if strings.Contains(inf.Statement, `service "web"`) {
			t.Errorf("build:. web service must not map as a datastore inference: %s", inf.Statement)
		}
	}
}

func TestComposeDedupesWithEnvPostgres(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 8080\n")
	writeFile(t, dir, "main.go", `package main; import "os"; var _ = os.Getenv("DATABASE_URL")`)
	writeFile(t, dir, "compose.yaml", `services:
  db:
    image: postgres:16
`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	postgresNeeds := 0
	for _, n := range res.Profile.Needs {
		if n.Capability == "postgres" {
			postgresNeeds++
		}
	}
	if postgresNeeds != 1 {
		t.Fatalf("env+compose must dedupe to one postgres need, got %d", postgresNeeds)
	}
	for _, inf := range res.Profile.Inferences {
		if inf.ID == "compose-postgres" {
			t.Error("compose postgres inference must not fire when env already established the need")
		}
	}
}

func TestComposeImageNameStripsRegistryAndTag(t *testing.T) {
	cases := map[string]string{
		"postgres:16":                 "postgres",
		"postgis/postgis:15-3.4":      "postgis",
		"redis:7-alpine":              "redis",
		"valkey/valkey:8":             "valkey",
		"docker.io/library/mongo:6":   "mongo",
		"minio/minio@sha256:deadbeef": "minio",
	}
	for in, want := range cases {
		if got := composeImageName(in); got != want {
			t.Errorf("composeImageName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestComposeParsesFourSpaceIndent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 8080\n")
	writeFile(t, dir, "docker-compose.yml", "services:\n    db:\n        image: postgres:16\n")
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range res.Profile.Needs {
		if n.Capability != "postgres" {
			continue
		}
		found = true
		if len(n.Evidence) == 0 || n.Evidence[0].File != "docker-compose.yml" || n.Evidence[0].Line != 3 {
			t.Fatalf("want compose image line 3, got %+v", n.Evidence)
		}
	}
	if !found {
		t.Fatal("4-space compose must still yield a postgres need")
	}
}

func TestComposeDedupesWithQuotedDatabaseURLLiteral(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 4000\n")
	writeFile(t, dir, "runtime.exs", `db = get_var_from_path_or_env(dir, "DATABASE_URL")`)
	writeFile(t, dir, "docker-compose.yml", `services:
  db:
    image: postgres:16
`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	postgresNeeds, postgresModes := 0, 0
	for _, n := range res.Profile.Needs {
		if n.Capability == "postgres" {
			postgresNeeds++
		}
	}
	for _, a := range res.Profile.Assumptions {
		if a.ID == "postgres-mode" {
			postgresModes++
		}
	}
	if postgresNeeds != 1 {
		t.Fatalf("quoted DATABASE_URL + compose must dedupe to one postgres need, got %d", postgresNeeds)
	}
	if postgresModes != 1 {
		t.Fatalf("postgres-mode assumption must not duplicate, got %d", postgresModes)
	}
	for _, inf := range res.Profile.Inferences {
		if inf.ID == "compose-postgres" {
			t.Error("compose postgres inference must not fire when quoted DATABASE_URL already established the need")
		}
	}
}

func TestUnsupportedMixedEnvAndComposeDetected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 8080\n")
	writeFile(t, dir, "app.js", `const r = process.env.REDIS_URL;`)
	writeFile(t, dir, "docker-compose.yml", `services:
  cache:
    image: redis:7
`)
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profile.Unsupported) != 1 || res.Profile.Unsupported[0].Capability != "redis" {
		t.Fatalf("want one redis unsupported, got %+v", res.Profile.Unsupported)
	}
	got := res.Profile.Unsupported[0].Detected
	want := "environment variables REDIS_URL; compose service cache(redis:7)"
	if got != want {
		t.Fatalf("Detected=%q want %q", got, want)
	}
}

func TestProcfileExpandsProcessRoles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 3000\n")
	writeFile(t, dir, "Procfile", "web: bundle exec puma\nworker: bundle exec sidekiq\n")
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profile.Services) != 2 {
		t.Fatalf("expected web+worker services from Procfile, got %+v", res.Profile.Services)
	}
	byName := map[string]profile.Service{}
	for _, s := range res.Profile.Services {
		byName[s.Name] = s
	}
	web, ok := byName["web"]
	if !ok || web.Kind != "http" || web.Port != 3000 || web.Dockerfile != "Dockerfile" {
		t.Fatalf("web service: %+v", web)
	}
	worker, ok := byName["worker"]
	if !ok || worker.Kind != "worker" || worker.Dockerfile != "Dockerfile" {
		t.Fatalf("worker service: %+v", worker)
	}
	if len(web.Command) != 0 || len(worker.Command) != 0 {
		t.Fatalf("Procfile commands must not become container args, web=%v worker=%v", web.Command, worker.Command)
	}
	inferred := false
	for _, inf := range res.Profile.Inferences {
		if inf.ID == "kind-worker" && inf.Confidence == "medium" {
			inferred = true
		}
	}
	if !inferred {
		t.Fatal("expected medium-confidence worker-role inference")
	}
	named := false
	for _, a := range res.Profile.Assumptions {
		if a.ID == "workload-roles" && strings.Contains(a.Statement, "web") && strings.Contains(a.Statement, "worker") {
			named = true
		}
	}
	if !named {
		t.Fatal("workload-roles assumption must name Procfile entries")
	}
	for _, q := range res.Questions {
		if strings.Contains(q, "Procfile") && strings.Contains(q, "worker") {
			return
		}
	}
	t.Fatal("open questions must name discovered Procfile processes")
}

func TestProcfileDevUsedWhenProcfileMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 8080\n")
	writeFile(t, dir, "Procfile.dev", "web: npm start\nclock: bundle exec clockwork\n")
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profile.Services) != 2 {
		t.Fatalf("Procfile.dev should expand roles, got %+v", res.Profile.Services)
	}
	foundClock := false
	for _, s := range res.Profile.Services {
		if s.Name == "clock" && s.Kind == "worker" {
			foundClock = true
		}
		if len(s.Command) != 0 {
			t.Fatalf("Procfile.dev command must not become container args: %+v", s.Command)
		}
	}
	if !foundClock {
		t.Fatalf("clock process missing: %+v", res.Profile.Services)
	}
	devNoted := false
	for _, f := range res.Profile.Facts {
		if strings.Contains(f.Statement, "dev process file") && strings.Contains(f.Statement, "confirm these roles exist in production") {
			devNoted = true
		}
	}
	if !devNoted {
		t.Fatal("Procfile.dev facts must say it is a dev process file")
	}
	assumed := false
	for _, a := range res.Profile.Assumptions {
		if a.ID == "workload-roles" && strings.Contains(a.Statement, "dev process file") {
			assumed = true
		}
	}
	if !assumed {
		t.Fatal("Procfile.dev workload-roles assumption must say it is a dev process file")
	}
}

func TestProcfileIgnoresReleaseAndMigrate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM x\nEXPOSE 8080\n")
	writeFile(t, dir, "Procfile", "web: bundle exec puma\nrelease: rake db:migrate\nmigrate: python manage.py migrate\nworker: bundle exec sidekiq\n")
	res, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]profile.Service{}
	for _, s := range res.Profile.Services {
		byName[s.Name] = s
	}
	if _, ok := byName["release"]; ok {
		t.Fatal("release process must not become a service")
	}
	if _, ok := byName["migrate"]; ok {
		t.Fatal("migrate process must not become a service")
	}
	if byName["web"].Kind != "http" || byName["worker"].Kind != "worker" {
		t.Fatalf("expected web+worker, got %+v", res.Profile.Services)
	}
	if len(res.Profile.Services) != 2 {
		t.Fatalf("expected only web+worker, got %+v", res.Profile.Services)
	}
}

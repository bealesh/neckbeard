// Package analyze is the deterministic detection layer under the analyzer
// (DESIGN §7, §8): it walks a repository and reports what is mechanically
// observable — Dockerfiles, exposed ports, environment-variable reads, language
// markers — as FACTS with file:line evidence, plus the inferences and assumptions
// those facts support. The agent (plugin skill) refines the draft with judgment
// and turns open questions into confirmed answers; this package never guesses
// silently and never reads secret VALUES.
package analyze

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/bealesh/neckbeard/core/profile"
)

// Result is a draft app profile plus the questions inspection cannot settle.
type Result struct {
	Profile   profile.AppProfile
	Questions []string
}

var skipDirs = map[string]bool{
	".neckbeard": true, ".agents": true, ".codex": true, ".cursor": true, ".claude": true,
	".git": true, "node_modules": true, "vendor": true, ".terraform": true,
	"dist": true, "build": true, "_build": true, "deps": true, ".next": true,
	"target": true, ".venv": true, "__pycache__": true,
}

const (
	maxFiles    = 5000
	maxFileSize = 1 << 20 // env-scan cap per file
)

// envAccessors match environment reads across the launch languages. The captured
// group is the variable name.
var envAccessors = []*regexp.Regexp{
	regexp.MustCompile(`os\.Getenv\(\s*"([A-Z][A-Z0-9_]+)"\s*\)`),        // Go
	regexp.MustCompile(`os\.LookupEnv\(\s*"([A-Z][A-Z0-9_]+)"\s*\)`),     // Go
	regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]+)`),                // Node
	regexp.MustCompile(`process\.env\[["']([A-Z][A-Z0-9_]+)["']\]`),      // Node
	regexp.MustCompile(`(?:^|[^.\w])env\.([A-Z][A-Z0-9_]+)\b`),           // typed env wrappers (outline-style `env.REDIS_URL`)
	regexp.MustCompile(`System\.get_env\(\s*"([A-Z][A-Z0-9_]+)"`),        // Elixir
	regexp.MustCompile(`ENV\[["']([A-Z][A-Z0-9_]+)["']\]`),               // Ruby
	regexp.MustCompile(`os\.environ(?:\.get\(|\[)["']([A-Z][A-Z0-9_]+)`), // Python
	regexp.MustCompile(`env::var\(\s*"([A-Z][A-Z0-9_]+)"`),               // Rust
	regexp.MustCompile(`System\.getenv\(\s*"([A-Z][A-Z0-9_]+)"`),         // JVM
	regexp.MustCompile(`\$\{?([A-Z][A-Z0-9_]+)\}?`),                      // shell-ish (weak; only counted in .sh/.env.example)
}

var languageMarkers = map[string]string{
	"go.mod":           "Go",
	"package.json":     "Node.js",
	"mix.exs":          "Elixir",
	"pyproject.toml":   "Python",
	"requirements.txt": "Python",
	"Gemfile":          "Ruby",
	"Cargo.toml":       "Rust",
	"pom.xml":          "JVM (Maven)",
	"build.gradle":     "JVM (Gradle)",
}

// envClass buckets a variable name into workload-contract terms.
type envClass int

const (
	classIgnore envClass = iota
	classPostgres
	classObjectStorage
	classSecret
	classUnsupported
	classPort
)

type unsupportedHit struct{ capability, hint string }

var ignoreEnv = map[string]bool{
	"PORT": true, "HOME": true, "PATH": true, "HOSTNAME": true, "TERM": true,
	"APP_ENV": true, "ENV": true, "NODE_ENV": true, "MIX_ENV": true,
	"RACK_ENV": true, "GO_ENV": true, "ENVIRONMENT": true, "LOG_LEVEL": true,
	"TZ": true, "LANG": true, "USER": true, "PWD": true, "DEBUG": true,
}

func classifyEnv(name string) (envClass, unsupportedHit) {
	n := strings.ToUpper(name)
	switch {
	case n == "PORT":
		return classPort, unsupportedHit{}
	case ignoreEnv[n]:
		return classIgnore, unsupportedHit{}
	case n == "DATABASE_URL" || strings.HasPrefix(n, "POSTGRES") || strings.HasPrefix(n, "PG") && strings.Contains(n, "HOST"):
		return classPostgres, unsupportedHit{}
	case strings.Contains(n, "REDIS") || strings.Contains(n, "MEMCACHE"):
		return classUnsupported, unsupportedHit{"redis", name}
	case strings.Contains(n, "KAFKA") || strings.Contains(n, "AMQP") || strings.Contains(n, "RABBIT") || strings.Contains(n, "SQS_") || strings.Contains(n, "NATS"):
		return classUnsupported, unsupportedHit{"queues", name}
	case strings.Contains(n, "ELASTIC") || strings.HasPrefix(n, "ES_") || strings.Contains(n, "OPENSEARCH"):
		return classUnsupported, unsupportedHit{"elasticsearch", name}
	case strings.Contains(n, "MONGO") || strings.Contains(n, "MYSQL") || strings.Contains(n, "MARIADB") || strings.Contains(n, "CLICKHOUSE"):
		return classUnsupported, unsupportedHit{"non-postgres-database", name}
	case strings.Contains(n, "BUCKET") || strings.HasPrefix(n, "S3_") || strings.HasPrefix(n, "GCS_") || strings.Contains(n, "BLOB_"):
		return classObjectStorage, unsupportedHit{}
	case strings.Contains(n, "SECRET") || strings.Contains(n, "TOKEN") || strings.Contains(n, "PASSWORD") || strings.Contains(n, "API_KEY") || strings.HasSuffix(n, "_KEY") || strings.HasSuffix(n, "_DSN") || strings.HasSuffix(n, "_URL"):
		// _URL last: service URLs are usually credentials-bearing; they surface as
		// secret NAMES for the operator to fill (values never touch neckbeard).
		return classSecret, unsupportedHit{}
	default:
		return classIgnore, unsupportedHit{}
	}
}

var (
	exposeRe = regexp.MustCompile(`(?i)^\s*EXPOSE\s+(\d+)`)
	healthRe = regexp.MustCompile(`"(/(?:_?healthz?|readyz|livez))"`)
	dbURLRe  = regexp.MustCompile(`["']DATABASE_URL["']`)
	// Template names only — a real .env may hold values and is never scanned.
	envTemplateRe = regexp.MustCompile(`^\.?env\.(example|sample|template|dist)$`)
	envAssignRe   = regexp.MustCompile(`^([A-Z][A-Z0-9_]+)=`)
)

type envHit struct {
	name string
	ev   profile.Evidence
}

// Dir analyzes a repository directory into a draft profile + open questions.
func Dir(root string) (*Result, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a repository directory", root)
	}
	var (
		dockerfiles   []string           // rel paths
		exposePorts   = map[string]int{} // dockerfile rel path → first EXPOSE
		langs         []profile.Fact
		envHits       = map[string][]profile.Evidence{}
		healthPaths   = map[string]profile.Evidence{}
		dbURLLiterals []profile.Evidence
		fileCount     int
	)

	manifestPostgres := readManifestPostgresEvidence(root)
	manifestORM := readManifestORMEvidence(root)

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if d.IsDir() {
			// Dot-directories are tooling by convention (.devcontainer, .github,
			// .circleci …), never deployed application code; a devcontainer
			// Dockerfile must not become a draft service.
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		fileCount++
		if fileCount > maxFiles {
			return fs.SkipAll
		}
		rel, _ := filepath.Rel(root, path)
		base := d.Name()

		if lang, ok := languageMarkers[base]; ok && filepath.Dir(rel) == "." {
			langs = append(langs, profile.Fact{
				ID:        "lang-" + strings.ToLower(strings.ReplaceAll(lang, " ", "-")),
				Statement: lang + " project (" + base + ")",
				Evidence:  []profile.Evidence{{File: rel}},
			})
		}

		if strings.HasPrefix(base, "Dockerfile") {
			dockerfiles = append(dockerfiles, rel)
			scanLines(path, func(line string, n int) {
				if m := exposeRe.FindStringSubmatch(line); m != nil {
					if _, seen := exposePorts[rel]; !seen {
						var p int
						fmt.Sscanf(m[1], "%d", &p)
						exposePorts[rel] = p
					}
				}
			})
			return nil
		}

		// Env-template files (.env.example/.env.sample/.env.template/.env.dist)
		// declare the app's environment surface by convention — names only; real
		// .env files are never read.
		if envTemplateRe.MatchString(base) {
			scanLines(path, func(line string, n int) {
				if m := envAssignRe.FindStringSubmatch(line); m != nil && len(envHits[m[1]]) < 5 {
					envHits[m[1]] = append(envHits[m[1]], profile.Evidence{File: rel, Line: n})
				}
			})
			return nil
		}

		if !isSourceFile(base) {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > maxFileSize {
			return nil
		}
		weakOK := strings.HasSuffix(base, ".sh") || strings.HasSuffix(base, ".env.example")
		scanLines(path, func(line string, n int) {
			for i, re := range envAccessors {
				if i == len(envAccessors)-1 && !weakOK {
					continue // the shell pattern only counts in shell-ish files
				}
				for _, m := range re.FindAllStringSubmatch(line, -1) {
					name := m[1]
					if len(envHits[name]) < 5 {
						envHits[name] = append(envHits[name], profile.Evidence{File: rel, Line: n})
					}
				}
			}
			if m := healthRe.FindStringSubmatch(line); m != nil {
				if _, seen := healthPaths[m[1]]; !seen {
					healthPaths[m[1]] = profile.Evidence{File: rel, Line: n}
				}
			}
			if dbURLRe.MatchString(line) && len(dbURLLiterals) < 5 {
				dbURLLiterals = append(dbURLLiterals, profile.Evidence{File: rel, Line: n})
			}
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	composeHits := readComposeCapabilities(root)
	return synthesize(root, dockerfiles, exposePorts, langs, envHits, healthPaths, dbURLLiterals, composeHits, manifestPostgres, manifestORM)
}

func synthesize(root string, dockerfiles []string, exposePorts map[string]int, langs []profile.Fact, envHits map[string][]profile.Evidence, healthPaths map[string]profile.Evidence, dbURLLiterals []profile.Evidence, composeHits []composeCapability, manifestPostgres, manifestORM []profile.Evidence) (*Result, error) {
	p := profile.AppProfile{Version: 1}
	var questions []string

	p.Facts = append(p.Facts, langs...)
	sort.Strings(dockerfiles)

	// Services: one candidate per Dockerfile. Kind and schedule are judgment calls
	// the agent/user must settle; the draft assumes http so `plan` runs.
	if len(dockerfiles) == 0 {
		questions = append(questions, "No Dockerfile found: the workload contract covers containerized services only — is there a container build, or does one need to be added?")
	}

	// A repository-root Dockerfile is the application image by convention; the
	// rest (Dockerfile.base, packaging/, examples) become a loud, plan-gating
	// assumption instead of synthesized services. Real second images (a
	// separately deployed service) get added back by the agent and are then
	// refused honestly as multi-image. Without a root Dockerfile there is no
	// convention to lean on, so every candidate stays visible.
	if len(dockerfiles) > 1 && slices.Contains(dockerfiles, "Dockerfile") {
		others := make([]string, 0, len(dockerfiles)-1)
		for _, df := range dockerfiles {
			if df != "Dockerfile" {
				others = append(others, df)
			}
		}
		dockerfiles = []string{"Dockerfile"}
		p.Assumptions = append(p.Assumptions, profile.Assumption{
			ID:        "container-images",
			Statement: fmt.Sprintf("using the repository-root Dockerfile as the single application image; other container builds (%s) assumed to be dev, base, or packaging images rather than separately deployed services", strings.Join(others, ", ")),
		})
		questions = append(questions, fmt.Sprintf("Are any of these other container builds separately deployed services: %s? Multi-image applications are not supported and are refused, not squeezed into one image.", strings.Join(others, ", ")))
	}
	healthPath := ""
	for _, hp := range []string{"/healthz", "/health", "/_health", "/readyz", "/livez"} {
		if _, found := healthPaths[hp]; found {
			healthPath = hp
			break
		}
	}
	for i, df := range dockerfiles {
		name := serviceNameFor(df, i)
		port := exposePorts[df]
		svc := profile.Service{Name: name, Kind: "http", Dockerfile: df}
		if port != 0 {
			svc.Port = port
			p.Facts = append(p.Facts, profile.Fact{
				ID:        "expose-" + name,
				Statement: fmt.Sprintf("%s EXPOSEs port %d", df, port),
				Evidence:  []profile.Evidence{{File: df}},
			})
		} else {
			svc.Port = 8080
			p.Assumptions = append(p.Assumptions, profile.Assumption{
				ID:        "port-" + name,
				Statement: fmt.Sprintf("service %q assumed to listen on 8080 (no EXPOSE found)", name),
			})
			questions = append(questions, fmt.Sprintf("What port does %q listen on?", name))
		}
		if healthPath != "" {
			svc.HealthPath = healthPath
			ev := healthPaths[healthPath]
			p.Facts = append(p.Facts, profile.Fact{
				ID:        "health-" + name,
				Statement: fmt.Sprintf("health-check path %q referenced in source", healthPath),
				Evidence:  []profile.Evidence{ev},
			})
		} else {
			svc.HealthPath = "/healthz"
			p.Assumptions = append(p.Assumptions, profile.Assumption{
				ID:        "health-" + name,
				Statement: fmt.Sprintf("service %q assumed to serve /healthz (no health path found in source)", name),
			})
			questions = append(questions, fmt.Sprintf("What is %q's health-check path?", name))
		}
		p.Assumptions = append(p.Assumptions, profile.Assumption{
			ID:        "kind-" + name,
			Statement: fmt.Sprintf("service %q assumed kind=http; workers and cron jobs cannot be detected mechanically", name),
		})
		p.Services = append(p.Services, svc)
	}
	questions = append(questions,
		"Are there background workers or scheduled jobs (and their schedules)? Detection cannot see process roles.",
		"Expected sustained traffic, availability objective, and RPO/RTO? These pick the tier.",
		"For each detected datastore: provision new, or reference an existing managed service?",
	)
	p.Assumptions = append(p.Assumptions,
		profile.Assumption{ID: "workload-roles", Statement: "Confirm HTTP services, workers, and scheduled jobs; one Dockerfile does not establish process roles"},
		profile.Assumption{ID: "capacity", Statement: "Confirm expected traffic, availability and recovery needs, and select the tier in neckbeard.yaml"},
	)

	names := make([]string, 0, len(envHits))
	for n := range envHits {
		names = append(names, n)
	}
	sort.Strings(names)
	seenNeed := map[string]bool{}
	unsupportedByCap := map[string]*pendingUnsupported{}
	var unsupportedOrder []string
	for _, name := range names {
		class, hit := classifyEnv(name)
		ev := envHits[name]
		switch class {
		case classPostgres:
			if !seenNeed["postgres"] {
				p.Needs = append(p.Needs, profile.Need{Capability: "postgres", Mode: "undecided", Evidence: ev})
				p.Assumptions = append(p.Assumptions, profile.Assumption{
					ID:        "postgres-mode",
					Statement: "Database environment variable detected: verify the engine and choose provision or reference; a client does not establish a need for a new database",
				})
				seenNeed["postgres"] = true
			}
			p.Secrets = append(p.Secrets, profile.SecretRef{Name: name})
		case classObjectStorage:
			if !seenNeed["object-storage"] {
				p.Needs = append(p.Needs, profile.Need{Capability: "object-storage", Mode: "undecided", Evidence: ev})
				p.Assumptions = append(p.Assumptions, profile.Assumption{ID: "object-storage-mode", Statement: "Choose provision or reference for the detected object storage"})
				seenNeed["object-storage"] = true
			}
		case classSecret:
			p.Secrets = append(p.Secrets, profile.SecretRef{Name: name})
		case classUnsupported:
			// One finding per capability: five REDIS_* variables are one redis
			// dependency, with every variable named and evidence merged.
			u := unsupportedByCap[hit.capability]
			if u == nil {
				u = &pendingUnsupported{}
				unsupportedByCap[hit.capability] = u
				unsupportedOrder = append(unsupportedOrder, hit.capability)
			}
			u.vars = append(u.vars, hit.hint)
			if len(u.ev) < 5 {
				u.ev = append(u.ev, ev[0])
			}
		}
		if class != classIgnore && class != classPort && class != classUnsupported {
			p.Facts = append(p.Facts, profile.Fact{
				ID:        "env-" + strings.ToLower(name),
				Statement: fmt.Sprintf("reads environment variable %s", name),
				Evidence:  ev[:1],
			})
		}
	}
	// PostgreSQL is often read through a configuration helper the accessor
	// patterns cannot see; the exact quoted literal "DATABASE_URL" is still a
	// deterministic signal. It lands as an INFERENCE (medium confidence, verify
	// in code) plus an undecided need, never as a fact.
	if !seenNeed["postgres"] && len(dbURLLiterals) > 0 {
		p.Needs = append(p.Needs, profile.Need{Capability: "postgres", Mode: "undecided", Evidence: dbURLLiterals})
		p.Inferences = append(p.Inferences, profile.Inference{
			ID:         "postgres-indirect",
			Statement:  "the app appears to use PostgreSQL configured via DATABASE_URL",
			Confidence: "medium",
			Reasoning:  `"DATABASE_URL" appears as a quoted literal but no direct environment read was detected; it is likely consumed through a configuration helper — verify in the code`,
		})
		p.Assumptions = append(p.Assumptions, profile.Assumption{
			ID:        "postgres-mode",
			Statement: "Database environment variable detected: verify the engine and choose provision or reference; a client does not establish a need for a new database",
		})
		p.Secrets = append(p.Secrets, profile.SecretRef{Name: "DATABASE_URL"})
		seenNeed["postgres"] = true
	}
	// Dependency manifests can hide DATABASE_URL behind helpers (Saleor-style
	// dj-database-url). An allowlisted marker is mechanical evidence: medium-
	// confidence inference + undecided need, never a fact.
	if !seenNeed["postgres"] && len(manifestPostgres) > 0 {
		p.Needs = append(p.Needs, profile.Need{Capability: "postgres", Mode: "undecided", Evidence: manifestPostgres})
		reasoning := "an allowlisted datastore dependency marker was found in a package manifest but no direct DATABASE_URL environment read was detected — verify in the code"
		if hasPackageJSONEvidence(manifestPostgres) {
			reasoning += ". The dependency may be dev-only"
		}
		p.Inferences = append(p.Inferences, profile.Inference{
			ID:         "postgres-manifest",
			Statement:  "the app appears to use PostgreSQL via a dependency that typically reads DATABASE_URL",
			Confidence: "medium",
			Reasoning:  reasoning,
		})
		p.Assumptions = append(p.Assumptions, profile.Assumption{
			ID:        "postgres-mode",
			Statement: "Database dependency found in a package manifest: verify the engine and choose provision or reference; a dependency does not establish a need for a new database.",
		})
		p.Secrets = append(p.Secrets, profile.SecretRef{Name: "DATABASE_URL"})
		seenNeed["postgres"] = true
	}
	// prisma/sequelize are multi-engine ORMs: they are not evidence of postgres.
	if !seenNeed["postgres"] && len(manifestORM) > 0 {
		p.Inferences = append(p.Inferences, profile.Inference{
			ID:         "database-orm-manifest",
			Statement:  "a database ORM is present; the engine is unverified — confirm postgres vs another engine, which would be an unsupported finding",
			Confidence: "medium",
			Reasoning:  "prisma or sequelize was found in a package manifest; these ORMs support multiple engines so postgres is not inferred — confirm the engine in the code. The dependency may be dev-only",
		})
	}
	// Compose image services (root docker-compose.yml / compose.yaml) are mechanical
	// evidence of datastores the app runs beside in development. Merge by capability
	// with env/literal/manifest hits — never duplicate a need/unsupported already recorded.
	applyComposeCapabilities(&p, composeHits, seenNeed, unsupportedByCap, &unsupportedOrder)
	for _, capability := range unsupportedOrder {
		u := unsupportedByCap[capability]
		p.Unsupported = append(p.Unsupported, profile.Unsupported{
			Capability:  capability,
			Detected:    formatUnsupportedDetected(u.vars),
			Explanation: fmt.Sprintf("%s is outside the launch workload contract (DESIGN §3.2); neckbeard will not force-fit it onto the catalog", capability),
			Evidence:    u.ev,
		})
	}
	if len(p.Secrets) > 0 && !seenNeed["secrets-cap"] {
		p.Needs = append(p.Needs, profile.Need{Capability: "secrets", Mode: "provision"})
	}
	if len(p.Unsupported) > 0 {
		questions = append(questions, "Unsupported capabilities were detected (see the unsupported section) — how are they handled today, and is a reference to an existing service acceptable?")
	}

	return &Result{Profile: p, Questions: questions}, nil
}

// manifestRule maps dependency-manifest markers onto a postgres capability signal.
// anyOf: one marker is enough; allOf: every marker must appear in the same file set.
type manifestRule struct {
	files []string
	anyOf []string
	allOf []string
	match func(line, marker string) bool
}

var postgresManifestRules = []manifestRule{
	{
		files: []string{"requirements.txt", "pyproject.toml"},
		anyOf: []string{"dj-database-url", "django-environ"},
		match: matchPythonManifestDep,
	},
	{
		files: []string{"package.json"},
		anyOf: []string{"pg"},
		match: matchNodeManifestDep,
	},
	{
		files: []string{"mix.exs"},
		allOf: []string{"ecto_sql", "postgrex"},
		match: matchElixirManifestDep,
	},
	{
		files: []string{"Gemfile"},
		allOf: []string{"activerecord", "pg"},
		match: matchRubyManifestDep,
	},
}

var nodeORMManifestRules = []manifestRule{
	{
		files: []string{"package.json"},
		anyOf: []string{"prisma", "sequelize"},
		match: matchNodeManifestDep,
	},
}

func matchPythonManifestDep(line, marker string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	// requirements.txt: package at line start; pyproject: quoted package name.
	re := regexp.MustCompile(`(?i)(^|["'\s])` + regexp.QuoteMeta(marker) + `([="'\[\]\s,~!=<>]|$)`)
	return re.MatchString(line)
}

func matchNodeManifestDep(line, marker string) bool {
	return regexp.MustCompile(`"` + regexp.QuoteMeta(marker) + `"\s*:`).MatchString(line)
}

func matchElixirManifestDep(line, marker string) bool {
	return regexp.MustCompile(`\{:` + regexp.QuoteMeta(marker) + `\b`).MatchString(line)
}

func matchRubyManifestDep(line, marker string) bool {
	return regexp.MustCompile(`gem\s+['"]` + regexp.QuoteMeta(marker) + `['"]`).MatchString(line)
}

// readManifestPostgresEvidence scans root dependency manifests for allowlisted
// datastore markers. A hit is evidence only — synthesize turns it into an
// inference, never a fact.
func readManifestPostgresEvidence(root string) []profile.Evidence {
	return readManifestEvidence(root, postgresManifestRules)
}

func readManifestORMEvidence(root string) []profile.Evidence {
	return readManifestEvidence(root, nodeORMManifestRules)
}

func readManifestEvidence(root string, rules []manifestRule) []profile.Evidence {
	var out []profile.Evidence
	for _, rule := range rules {
		hits := matchManifestRule(root, rule)
		if len(hits) == 0 {
			continue
		}
		out = append(out, hits...)
		if len(out) >= 5 {
			return out[:5]
		}
	}
	return out
}

func hasPackageJSONEvidence(ev []profile.Evidence) bool {
	for _, e := range ev {
		if e.File == "package.json" {
			return true
		}
	}
	return false
}

func matchManifestRule(root string, rule manifestRule) []profile.Evidence {
	want := rule.anyOf
	requireAll := false
	if len(rule.allOf) > 0 {
		want = rule.allOf
		requireAll = true
	}
	found := map[string]profile.Evidence{}
	for _, name := range rule.files {
		path := filepath.Join(root, name)
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			continue
		}
		scanLines(path, func(line string, n int) {
			for _, marker := range want {
				if _, ok := found[marker]; ok {
					continue
				}
				if rule.match(line, marker) {
					found[marker] = profile.Evidence{File: name, Line: n}
				}
			}
		})
	}
	if requireAll {
		if len(found) < len(want) {
			return nil
		}
	} else if len(found) == 0 {
		return nil
	}
	var out []profile.Evidence
	for _, marker := range want {
		if ev, ok := found[marker]; ok {
			out = append(out, ev)
		}
	}
	return out
}

type pendingUnsupported struct {
	vars []string
	ev   []profile.Evidence
}

type composeCapability struct {
	Capability string // postgres | object-storage | redis | queues | non-postgres-database
	AsNeed     bool   // true → undecided need; false → unsupported finding
	Image      string
	Service    string
	File       string
	Line       int
}

// readComposeCapabilities maps well-known images in a root compose file to
// capabilities. Services with a build: key (the app image) are skipped; only
// image-based sidecar services count.
func readComposeCapabilities(root string) []composeCapability {
	var out []composeCapability
	seenCap := map[string]bool{}
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml"} {
		path := filepath.Join(root, name)
		hits := parseComposeFile(name, path)
		for _, h := range hits {
			if seenCap[h.Capability] {
				continue
			}
			seenCap[h.Capability] = true
			out = append(out, h)
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func parseComposeFile(rel, path string) []composeCapability {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		Services map[string]struct {
			Image yaml.Node `yaml:"image"`
			Build any       `yaml:"build"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var out []composeCapability
	for name, svc := range doc.Services {
		if svc.Build != nil {
			continue
		}
		if svc.Image.Kind != yaml.ScalarNode {
			continue
		}
		img := strings.TrimSpace(svc.Image.Value)
		if img == "" {
			continue
		}
		cap, asNeed, ok := classifyComposeImage(img)
		if !ok {
			continue
		}
		out = append(out, composeCapability{
			Capability: cap,
			AsNeed:     asNeed,
			Image:      img,
			Service:    name,
			File:       rel,
			Line:       svc.Image.Line,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Service < out[j].Service
	})
	return out
}

func classifyComposeImage(image string) (capability string, asNeed bool, ok bool) {
	base := composeImageName(image)
	switch base {
	case "postgres", "postgis":
		return "postgres", true, true
	case "redis", "valkey":
		return "redis", false, true
	case "rabbitmq":
		return "queues", false, true
	case "minio":
		return "object-storage", true, true
	case "mysql", "mariadb", "mongo", "mongodb", "clickhouse":
		return "non-postgres-database", false, true
	default:
		return "", false, false
	}
}

func formatUnsupportedDetected(vars []string) string {
	var envVars, composeVars []string
	for _, v := range vars {
		if strings.HasPrefix(v, "compose:") {
			composeVars = append(composeVars, strings.TrimPrefix(v, "compose:"))
		} else {
			envVars = append(envVars, v)
		}
	}
	switch {
	case len(envVars) > 0 && len(composeVars) > 0:
		return "environment variables " + strings.Join(envVars, ", ") + "; compose service " + strings.Join(composeVars, ", ")
	case len(composeVars) > 0:
		return "compose services " + strings.Join(composeVars, ", ")
	default:
		return "environment variables " + strings.Join(envVars, ", ")
	}
}

func composeImageName(image string) string {
	image = strings.TrimSpace(image)
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	if i := strings.LastIndex(image, ":"); i >= 0 {
		image = image[:i]
	}
	return strings.ToLower(image)
}

func applyComposeCapabilities(p *profile.AppProfile, hits []composeCapability, seenNeed map[string]bool, unsupportedByCap map[string]*pendingUnsupported, unsupportedOrder *[]string) {
	for _, h := range hits {
		ev := []profile.Evidence{{File: h.File, Line: h.Line}}
		if h.AsNeed {
			if seenNeed[h.Capability] {
				continue
			}
			p.Needs = append(p.Needs, profile.Need{Capability: h.Capability, Mode: "undecided", Evidence: ev})
			p.Inferences = append(p.Inferences, profile.Inference{
				ID:         "compose-" + h.Capability,
				Statement:  fmt.Sprintf("compose service %q runs image %q (%s)", h.Service, h.Image, h.Capability),
				Confidence: "medium",
				Reasoning:  fmt.Sprintf("%s:%d declares image %q; treat as an undecided %s need and confirm provision vs reference", h.File, h.Line, h.Image, h.Capability),
			})
			switch h.Capability {
			case "postgres":
				p.Assumptions = append(p.Assumptions, profile.Assumption{
					ID:        "postgres-mode",
					Statement: "Database environment variable detected: verify the engine and choose provision or reference; a client does not establish a need for a new database",
				})
			case "object-storage":
				p.Assumptions = append(p.Assumptions, profile.Assumption{ID: "object-storage-mode", Statement: "Choose provision or reference for the detected object storage"})
			}
			seenNeed[h.Capability] = true
			continue
		}
		u := unsupportedByCap[h.Capability]
		if u == nil {
			u = &pendingUnsupported{}
			unsupportedByCap[h.Capability] = u
			*unsupportedOrder = append(*unsupportedOrder, h.Capability)
		}
		hint := fmt.Sprintf("compose:%s(%s)", h.Service, h.Image)
		u.vars = append(u.vars, hint)
		if len(u.ev) < 5 {
			u.ev = append(u.ev, ev[0])
		}
	}
}

func serviceNameFor(dockerfile string, i int) string {
	dir := filepath.Dir(dockerfile)
	if dir != "." {
		return sanitizeName(filepath.Base(dir))
	}
	if suffix := strings.TrimPrefix(filepath.Base(dockerfile), "Dockerfile"); suffix != "" {
		return sanitizeName(strings.TrimLeft(suffix, ".-_"))
	}
	if i == 0 {
		return "web"
	}
	return fmt.Sprintf("web-%d", i)
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "web"
	}
	return out
}

func isSourceFile(name string) bool {
	for _, ext := range []string{".go", ".js", ".ts", ".jsx", ".tsx", ".mjs", ".ex", ".exs", ".rb", ".py", ".rs", ".java", ".kt", ".sh", ".yml", ".yaml", ".toml"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return strings.HasSuffix(name, ".env.example")
}

func scanLines(path string, fn func(line string, n int)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	n := 0
	for sc.Scan() {
		n++
		fn(sc.Text(), n)
	}
}

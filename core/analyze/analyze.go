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
	"sort"
	"strings"

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
	case strings.Contains(n, "MONGO") || strings.Contains(n, "MYSQL") || strings.Contains(n, "MARIADB"):
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
	healthRe = regexp.MustCompile(`"(/(?:healthz|health|readyz|livez))"`)
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
		dockerfiles []string           // rel paths
		exposePorts = map[string]int{} // dockerfile rel path → first EXPOSE
		langs       []profile.Fact
		envHits     = map[string][]profile.Evidence{}
		healthPaths = map[string]profile.Evidence{}
		fileCount   int
	)

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
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
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return synthesize(root, dockerfiles, exposePorts, langs, envHits, healthPaths)
}

func synthesize(root string, dockerfiles []string, exposePorts map[string]int, langs []profile.Fact, envHits map[string][]profile.Evidence, healthPaths map[string]profile.Evidence) (*Result, error) {
	p := profile.AppProfile{Version: 1}
	var questions []string

	p.Facts = append(p.Facts, langs...)
	sort.Strings(dockerfiles)

	// Services: one candidate per Dockerfile. Kind and schedule are judgment calls
	// the agent/user must settle; the draft assumes http so `plan` runs.
	if len(dockerfiles) == 0 {
		questions = append(questions, "No Dockerfile found: the workload contract covers containerized services only — is there a container build, or does one need to be added?")
	}
	healthPath := ""
	for _, hp := range []string{"/healthz", "/health", "/readyz", "/livez"} {
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
			p.Unsupported = append(p.Unsupported, profile.Unsupported{
				Capability:  hit.capability,
				Detected:    fmt.Sprintf("environment variable %s", hit.hint),
				Explanation: fmt.Sprintf("%s is outside the launch workload contract (DESIGN §3.2); neckbeard will not force-fit it onto the catalog", hit.capability),
				Evidence:    ev,
			})
		}
		if class != classIgnore && class != classPort && class != classUnsupported {
			p.Facts = append(p.Facts, profile.Fact{
				ID:        "env-" + strings.ToLower(name),
				Statement: fmt.Sprintf("reads environment variable %s", name),
				Evidence:  ev[:1],
			})
		}
	}
	if len(p.Secrets) > 0 && !seenNeed["secrets-cap"] {
		p.Needs = append(p.Needs, profile.Need{Capability: "secrets", Mode: "provision"})
	}
	if len(p.Unsupported) > 0 {
		questions = append(questions, "Unsupported capabilities were detected (see the unsupported section) — how are they handled today, and is a reference to an existing service acceptable?")
	}

	return &Result{Profile: p, Questions: questions}, nil
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

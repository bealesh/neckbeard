package disco

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bealesh/neckbeard/core/profile"
)

func bellwether(t *testing.T) *profile.AppProfile {
	t.Helper()
	p, _, err := profile.Load("../../bellwether/app-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderBellwether(t *testing.T) {
	out, err := Render(bellwether(t), "bellwether")
	if err != nil {
		t.Fatal(err)
	}
	var f File
	if err := json.Unmarshal(out.DiscoJSON, &f); err != nil {
		t.Fatal(err)
	}
	want := map[string]Service{
		"web":            {Image: "default", Port: 8080, Command: "serve"},
		"worker":         {Image: "default", Command: "work"},
		"nightly-report": {Type: "cron", Image: "default", Command: "report", Schedule: "0 3 * * *"},
	}
	if len(f.Services) != len(want) {
		t.Fatalf("services = %v, want %v", f.Services, want)
	}
	for name, w := range want {
		if f.Services[name] != w {
			t.Errorf("service %s = %+v, want %+v", name, f.Services[name], w)
		}
	}
	if img := f.Images["default"]; img.Dockerfile != "bellwether/Dockerfile" || img.Context != "." {
		t.Errorf("image = %+v", img)
	}
	rb := string(out.Runbook)
	for _, s := range []string{`disco init "root@$SERVER_IP"`, "disco projects:add --name bellwether", "disco postgres:create --project bellwether\n", "disco deploy --project bellwether --dir ."} {
		if !strings.Contains(rb, s) {
			t.Errorf("runbook missing %q", s)
		}
	}
	// projects:add --github deploys at once: the repository is linked only after postgres + env exist
	if strings.Contains(rb, "projects:add --name bellwether --github") {
		t.Error("runbook adds the project with --github, which deploys before postgres exists")
	}
	pg, gh, link := strings.Index(rb, "disco postgres:create"), strings.Index(rb, "disco github:apps:add"), strings.Index(rb, `disco projects:update bellwether --github "$GITHUB_REPO"`)
	if pg < 0 || gh < pg || link < gh {
		t.Errorf("runbook order: postgres:create (%d), github:apps:add (%d), projects:update --github (%d)", pg, gh, link)
	}
	if !strings.Contains(rb, "It does not call /healthz itself") {
		t.Error("runbook missing the health path note")
	}
	if strings.Contains(rb, "DATABASE_URL=") {
		t.Error("runbook sets DATABASE_URL by hand; postgres:create sets it")
	}
}

func TestRenderExternalUnsupported(t *testing.T) {
	p := bellwether(t)
	p.Unsupported = append(p.Unsupported, profile.Unsupported{
		Capability: "redis", Detected: "redis client", Explanation: "not in the catalog",
		Disposition: "external", SecretName: "REDIS_URL", Resolution: "the team's managed redis",
	}, profile.Unsupported{
		Capability: "queue", Detected: "x", Explanation: "y",
		Disposition: "not-required", Resolution: "dead code",
	})
	out, err := Render(p, "bellwether")
	if err != nil {
		t.Fatal(err)
	}
	rb := string(out.Runbook)
	for _, s := range []string{`disco env:set --project bellwether REDIS_URL="$REDIS_URL"`, "- `REDIS_URL`: connection for your redis", "- redis at `REDIS_URL`: the team's managed redis"} {
		if !strings.Contains(rb, s) {
			t.Errorf("runbook missing %q", s)
		}
	}
	if strings.Contains(rb, "queue") {
		t.Error("a not-required capability should not appear in the runbook")
	}
}

func TestRenderRefuses(t *testing.T) {
	cases := map[string]func(p *profile.AppProfile){
		"open assumption": func(p *profile.AppProfile) {
			p.Assumptions = append(p.Assumptions, profile.Assumption{ID: "unanswered", Statement: "x"})
		},
		"two http services": func(p *profile.AppProfile) {
			s := p.Services[0]
			s.Name = "admin"
			p.Services = append(p.Services, s)
		},
		"worker named web": func(p *profile.AppProfile) {
			for i := range p.Services {
				switch p.Services[i].Kind {
				case "http":
					p.Services[i].Name = "site"
				case "worker":
					p.Services[i].Name = "web"
				}
			}
		},
		"no http service": func(p *profile.AppProfile) {
			var keep []profile.Service
			for _, s := range p.Services {
				if s.Kind != "http" {
					keep = append(keep, s)
				}
			}
			p.Services = keep
		},
		"unmapped datastore": func(p *profile.AppProfile) {
			p.Needs = append(p.Needs, profile.Need{Capability: "object-storage", Mode: "provision"})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := bellwether(t)
			mutate(p)
			if _, err := Render(p, "bellwether"); err == nil {
				t.Fatal("rendered, want an error")
			}
		})
	}
	for _, bad := range []string{"", "Bellwether", "1app", "a_b", strings.Repeat("a", 43)} {
		if _, err := Render(bellwether(t), bad); err == nil {
			t.Errorf("project name %q accepted", bad)
		}
	}
}

func TestRenderEnvVars(t *testing.T) {
	p := bellwether(t)
	p.Secrets = append(p.Secrets, profile.SecretRef{Name: "SESSION_SECRET"})
	p.Needs = append(p.Needs, profile.Need{Capability: "redis", Mode: "reference", SecretName: "CACHE_URL"})
	out, err := Render(p, "bellwether")
	if err != nil {
		t.Fatal(err)
	}
	rb := string(out.Runbook)
	for _, s := range []string{
		`disco env:set --project bellwether CACHE_URL="$CACHE_URL" SESSION_SECRET="$SESSION_SECRET"` + "\n",
		"- `SESSION_SECRET`: secret",
		"- `CACHE_URL`: connection for the existing redis",
	} {
		if !strings.Contains(rb, s) {
			t.Errorf("runbook missing %q", s)
		}
	}
	for _, line := range strings.Split(rb, "\n") {
		if strings.HasPrefix(line, "disco ") && strings.ContainsAny(line, "<>") {
			t.Errorf("command has a <placeholder>, a shell redirection if pasted: %s", line)
		}
	}

	for _, bad := range []string{"", "MY KEY", "A&B", "1X", "X=Y", "$(id)", "A;B"} {
		p := bellwether(t)
		p.Secrets = append(p.Secrets, profile.SecretRef{Name: bad})
		if _, err := Render(p, "bellwether"); err == nil {
			t.Errorf("secret name %q accepted", bad)
		}
	}
	p = bellwether(t)
	for i := range p.Needs {
		if p.Needs[i].Capability == "postgres" {
			p.Needs[i].SecretName = "DB URL"
		}
	}
	if _, err := Render(p, "bellwether"); err == nil {
		t.Error("postgres secret_name with a space accepted")
	}
}

func TestShellJoin(t *testing.T) {
	cases := map[string][]string{
		"serve":                       {"serve"},
		"python manage.py migrate":    {"python", "manage.py", "migrate"},
		`sh -c 'echo "hi" && exit 1'`: {"sh", "-c", `echo "hi" && exit 1`},
		`echo 'it'"'"'s'`:             {"echo", "it's"},
		"x ''":                        {"x", ""},
	}
	for want, args := range cases {
		if got := shellJoin(args); got != want {
			t.Errorf("shellJoin(%q) = %s, want %s", args, got, want)
		}
	}
}

func TestProvisionRedis(t *testing.T) {
	p := bellwether(t)
	p.Needs = append(p.Needs, profile.Need{Capability: "redis", Mode: "provision"})
	p.Secrets = append(p.Secrets, profile.SecretRef{Name: "REDIS_URL"})
	out, err := Render(p, "bellwether")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out.DiscoJSON), "redis") {
		t.Fatal("Redis must not share the app's blue/green lifecycle")
	}
	rb := string(out.Runbook)
	for _, want := range []string{redisImage, "--network disco-main --network-alias bellwether-redis", "appendonly yes", "appendfsync everysec", "maxmemory-policy noeviction", "--memory 512m", "requirepass %s", `REDIS_URL="redis://:$REDIS_PASSWORD@bellwether-redis:6379/0"`, "root-only", "docker stop neckbeard-bellwether-redis"} {
		if !strings.Contains(rb, want) {
			t.Errorf("runbook missing %q", want)
		}
	}
	for _, bad := range []string{"--publish", "-p 6379", `REDIS_URL="$REDIS_URL"`} {
		if strings.Contains(rb, bad) {
			t.Errorf("runbook contains %q", bad)
		}
	}
	p.Needs[len(p.Needs)-1].SecretName = "CACHE_URL"
	out, err = Render(p, "bellwether")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.Runbook), `CACHE_URL="redis://:`) {
		t.Fatal("custom connection binding not rendered")
	}
	for _, bad := range []string{"bad name", "$(id)", "DATABASE_URL"} {
		p.Needs[len(p.Needs)-1].SecretName = bad
		if _, err := Render(p, "bellwether"); err == nil {
			t.Errorf("accepted Redis binding %q", bad)
		}
	}
	p.Needs[len(p.Needs)-1].SecretName = "REDIS_URL"
	p.Needs = append(p.Needs, profile.Need{Capability: "redis", Mode: "provision"})
	if _, err := Render(p, "bellwether"); err == nil {
		t.Fatal("accepted two Redis servers")
	}
}

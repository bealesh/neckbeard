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
	for _, s := range []string{"disco projects:add --name bellwether", "disco postgres:create --project bellwether\n", "disco deploy --project bellwether --dir ."} {
		if !strings.Contains(rb, s) {
			t.Errorf("runbook missing %q", s)
		}
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
	for _, s := range []string{"REDIS_URL=<connection for your redis>", "- redis at `REDIS_URL`: the team's managed redis"} {
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

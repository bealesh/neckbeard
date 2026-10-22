package disco

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedisSetupScript(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "setup.sh")
	if err := os.WriteFile(script, []byte(redisSetup("trial")), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	// Exercise the actual generated shell with Docker mocked; no daemon or root writes.
	source := strings.ReplaceAll(redisSetup("trial"), "/var/lib/neckbeard/disco/trial/redis", filepath.Join(root, "config"))
	if err := os.WriteFile(script, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"id": "echo 0",
		"docker": `printf '%s\n' "$*" >> "$CAPTURE"
case "$*" in
 'container inspect '*) exit 1;;
 'volume inspect '*) exit 1;;
 'inspect '*) echo healthy;;
esac`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/bash\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	capture := filepath.Join(root, "args")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "CAPTURE="+capture)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	conf, err := os.ReadFile(filepath.Join(root, "config/redis.conf"))
	if err != nil {
		t.Fatal(err)
	}
	password := strings.TrimPrefix(strings.Split(string(conf), "\n")[0], "requirepass ")
	if len(password) != 64 {
		t.Fatal("password was not generated")
	}
	argv, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(argv), password) || strings.Contains(string(out), password) {
		t.Fatal("password leaked to process arguments or output")
	}
	if !strings.Contains(string(argv), "--network disco-main") || strings.Contains(string(argv), "--publish") {
		t.Fatalf("unsafe networking: %s", argv)
	}
}

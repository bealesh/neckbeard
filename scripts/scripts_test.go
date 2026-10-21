package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestPublishRelease(t *testing.T) {
	for _, tc := range []struct {
		tag        string
		prerelease bool
	}{
		{"v0.4.0", false}, {"v0.4.0-beta.1", true}, {"v0.4.0-rc.1", true},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			root := t.TempDir()
			dist := filepath.Join(root, "release assets")
			if err := os.Mkdir(dist, 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"checksums.txt", "neckbeard.tar.gz"} {
				writeFile(t, filepath.Join(dist, name), "")
			}
			capture := filepath.Join(root, "args")
			writeFile(t, filepath.Join(root, "gh"), "#!/bin/bash\nprintf '%s\\n' \"$@\" > \"$CAPTURE\"\n")
			cmd := exec.Command("bash", "publish-release.sh")
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "CAPTURE="+capture, "DIST="+dist, "GITHUB_REF_NAME="+tc.tag)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			raw, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
			want := []string{"release", "create", tc.tag, filepath.Join(dist, "checksums.txt"), filepath.Join(dist, "neckbeard.tar.gz"), "--verify-tag", "--generate-notes", "--title", tc.tag}
			if tc.prerelease {
				want = append(want, "--prerelease")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q; want %q", got, want)
			}
		})
	}
}

func TestInstallerVersionWithoutCurl(t *testing.T) {
	source, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, manifest, explicit, want string
		fail                           bool
	}{
		{name: "beta manifest", manifest: "0.4.0-beta.1", want: "v0.4.0-beta.1"},
		{name: "stable manifest", manifest: "0.4.0", want: "v0.4.0"},
		{name: "explicit beta", explicit: "v0.4.0-beta.1", want: "v0.4.0-beta.1"},
		{name: "override manifest", manifest: "0.3.0", explicit: "v0.4.0-beta.1", want: "v0.4.0-beta.1"},
		{name: "explicit latest", manifest: "0.4.0-beta.1", explicit: "latest", want: "latest"},
		{name: "no manifest", want: "latest"},
		{name: "unpublished beta", manifest: "0.4.0-beta.1", want: "v0.4.0-beta.1", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			scripts := filepath.Join(root, "scripts")
			bin := filepath.Join(root, "tools")
			for _, dir := range []string{scripts, bin, filepath.Join(root, ".claude-plugin")} {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(scripts, "install.sh"), string(source))
			if tc.manifest != "" {
				writeFile(t, filepath.Join(root, ".claude-plugin/plugin.json"), "{\n  \"version\": \""+tc.manifest+"\"\n}\n")
			}
			// An isolated PATH makes curl genuinely unavailable, regardless of the host.
			for _, tool := range []string{"dirname", "sed", "head", "uname", "tr", "mkdir"} {
				path, err := exec.LookPath(tool)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
					t.Fatal(err)
				}
			}
			capture := filepath.Join(root, "args")
			mock := "#!/bin/bash\nprintf '%s\\n' \"$@\" > \"$CAPTURE\"\n"
			if tc.fail {
				mock += "exit 1\n"
			} else {
				mock += "printf '#!/bin/bash\\necho neckbeard-test\\n' > \"$GOBIN/neckbeard\"\n/bin/chmod +x \"$GOBIN/neckbeard\"\n"
			}
			writeFile(t, filepath.Join(bin, "go"), mock)
			args := []string{filepath.Join(scripts, "install.sh"), "-f", "-d", filepath.Join(root, "installed")}
			if tc.explicit != "" {
				args = append(args, "-v", tc.explicit)
			}
			cmd := exec.Command(bash, args...)
			cmd.Env = append(os.Environ(), "PATH="+bin, "CAPTURE="+capture)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v, output=%s", err, out)
			}
			raw, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			want := "install\ngithub.com/bealesh/neckbeard/cmd/neckbeard@" + tc.want + "\n"
			if string(raw) != want {
				t.Fatalf("got %q; want %q", raw, want)
			}
			if tc.fail && strings.Contains(string(out), "installed ") {
				t.Fatalf("claimed success: %s", out)
			}
		})
	}
}

func TestInstallerExistingVersion(t *testing.T) {
	source, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, installed string
		mismatch        bool
	}{
		{"matching beta", "0.4.0-beta.1", false},
		{"stable needs beta", "0.3.0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			scripts := filepath.Join(root, "scripts")
			manifest := filepath.Join(root, ".claude-plugin")
			for _, dir := range []string{scripts, manifest} {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(scripts, "install.sh"), string(source))
			writeFile(t, filepath.Join(manifest, "plugin.json"), "{\n  \"version\": \"0.4.0-beta.1\"\n}\n")
			writeFile(t, filepath.Join(root, "neckbeard"), "#!/bin/bash\necho neckbeard "+tc.installed+"\n")
			cmd := exec.Command("bash", filepath.Join(scripts, "install.sh"))
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.mismatch {
				t.Fatalf("err=%v, output=%s", err, out)
			}
			if tc.mismatch {
				for _, expected := range []string{"0.3.0", "v0.4.0-beta.1", "-f"} {
					if !strings.Contains(string(out), expected) {
						t.Fatalf("missing %q in %s", expected, out)
					}
				}
			} else if !strings.Contains(string(out), "already installed:") {
				t.Fatalf("unexpected output: %s", out)
			}
		})
	}
}

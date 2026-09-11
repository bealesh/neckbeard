package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The one permitted exclusion is surgical: the exact upstream-missing `queue`
// diagnostic, and only when the flagged line literally is `queue: max`. A real
// error on the same line number, a different queue value, or unparseable
// output must all stay failures.
func TestFilterActionlint(t *testing.T) {
	root := t.TempDir()
	wf := filepath.Join(".github", "workflows", "neckbeard-release.yml")
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(wf)), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  a:\n    concurrency:\n      queue: max\n      group: g\n"
	if err := os.WriteFile(filepath.Join(root, wf), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	queueDiag := `{"message":"unexpected key \"queue\" for \"concurrency\" section. expected one of \"cancel-in-progress\", \"group\"","filepath":".github/workflows/neckbeard-release.yml","line":4,"column":7,"kind":"syntax-check"}`
	realDiag := `{"message":"undefined variable \"oops\"","filepath":".github/workflows/neckbeard-release.yml","line":2,"column":3,"kind":"expression"}`

	kept, excluded, err := filterActionlint(root, []byte("["+queueDiag+"]"))
	if err != nil || len(kept) != 0 || excluded != 1 {
		t.Fatalf("queue-only diagnostics must be excluded: kept=%v excluded=%d err=%v", kept, excluded, err)
	}

	kept, excluded, err = filterActionlint(root, []byte("["+queueDiag+","+realDiag+"]"))
	if err != nil || len(kept) != 1 || excluded != 1 || !strings.Contains(kept[0].Message, "oops") {
		t.Fatalf("real diagnostics must survive filtering: kept=%v excluded=%d err=%v", kept, excluded, err)
	}

	// Same diagnostic shape, but the flagged line is not `queue: max`.
	wrongLine := strings.Replace(queueDiag, `"line":4`, `"line":5`, 1)
	kept, excluded, err = filterActionlint(root, []byte("["+wrongLine+"]"))
	if err != nil || len(kept) != 1 || excluded != 0 {
		t.Fatalf("a queue diagnostic on a non-queue-max line must be kept: kept=%v excluded=%d err=%v", kept, excluded, err)
	}

	if _, _, err = filterActionlint(root, []byte("not json")); err == nil {
		t.Fatal("unparseable output must be an error, never silently filtered")
	}
}

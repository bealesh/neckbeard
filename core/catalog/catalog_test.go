package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGCPHTTPIngressWorksWithDomainRestrictedSharing(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "catalog", "gcp", "runtime-serverless", "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	// Check the HTTP resource itself: disabling IAM without restricted ingress
	// would also expose its direct run.app endpoint.
	http := regexp.MustCompile(`(?s)resource "google_cloud_run_v2_service" "http" \{(.*?)\n  template \{`).FindStringSubmatch(source)
	if len(http) != 2 {
		t.Fatal("HTTP service resource not found")
	}
	for _, setting := range []string{
		`invoker_iam_disabled\s*=\s*true`,
		`ingress\s*=\s*"INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"`,
	} {
		if !regexp.MustCompile(setting).MatchString(http[1]) {
			t.Errorf("HTTP service missing required setting %s", setting)
		}
	}
	if strings.Contains(source, `"allUsers"`) {
		t.Error("public IAM principal is incompatible with domain restricted sharing")
	}
	// Removing public HTTP IAM must not remove authenticated scheduler access.
	cron := regexp.MustCompile(`(?s)resource "google_cloud_run_v2_job_iam_member" "scheduler_invokes" \{(.*?)\n\}`).FindStringSubmatch(source)
	if len(cron) != 2 || !regexp.MustCompile(`role\s*=\s*"roles/run.invoker"`).MatchString(cron[1]) || !regexp.MustCompile(`member\s*=\s*"serviceAccount:\$\{google_service_account\.run\.email\}"`).MatchString(cron[1]) {
		t.Error("scheduler must retain authenticated job invocation")
	}
}

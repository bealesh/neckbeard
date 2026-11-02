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

func TestGCPStartupProbeAllowsColdStarts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "catalog", "gcp", "runtime-serverless", "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	probe := regexp.MustCompile(`(?s)startup_probe \{(.*?)http_get \{`).FindStringSubmatch(string(data))
	if len(probe) != 2 {
		t.Fatal("HTTP startup probe not found")
	}
	for _, setting := range []string{`failure_threshold\s*=\s*24`, `period_seconds\s*=\s*10`, `timeout_seconds\s*=\s*5`} {
		if !regexp.MustCompile(setting).MatchString(probe[1]) {
			t.Errorf("startup probe needs a four-minute allowance with five-second attempts: missing %s", setting)
		}
	}
}

// A healthy app that takes a few minutes to answer its first request must not
// be killed mid-boot. Each serverless lane gets the same four-minute startup
// allowance; the regression here is per cloud because each expresses it
// differently (#44).
func TestAWSLoadBalancedServicesGetAStartupGracePeriod(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "catalog", "aws", "runtime-serverless", "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	service := regexp.MustCompile(`(?s)resource "aws_ecs_service" "this" \{(.*?)\n\}`).FindStringSubmatch(string(data))
	if len(service) != 2 {
		t.Fatal("ECS service resource not found")
	}
	grace := regexp.MustCompile(`health_check_grace_period_seconds\s*=\s*each\.value\.kind == "http" && contains\(keys\(var\.target_group_arns\), each\.key\) \? 240 : null`)
	if !grace.MatchString(service[1]) {
		t.Error("load-balanced HTTP services need a 240s grace period; workers (no load balancer) must keep the default, or ECS rejects the service")
	}
	if !strings.Contains(service[1], `rollback = true`) {
		t.Error("the deployment circuit breaker rollback must survive the grace-period change")
	}
}

func TestAzureHTTPContainersSeparateStartupReadinessAndLiveness(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "catalog", "azure", "runtime-serverless", "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, probe := range []string{"startup_probe", "readiness_probe", "liveness_probe"} {
		block := regexp.MustCompile(`(?s)dynamic "` + probe + `" \{(.*?)\n      \}`).FindStringSubmatch(source)
		if len(block) != 2 {
			t.Fatalf("%s block not found", probe)
		}
		if !strings.Contains(block[1], `for_each = each.value.kind == "http"`) {
			t.Errorf("%s must apply to HTTP services only; workers have no ingress to probe", probe)
		}
		if !regexp.MustCompile(`transport\s*=\s*"HTTP"`).MatchString(block[1]) || !strings.Contains(block[1], ".health_path") {
			t.Errorf("%s must probe the app's health path over HTTP, not just the TCP port", probe)
		}
	}
	startup := regexp.MustCompile(`(?s)dynamic "startup_probe" \{(.*?)\n      \}`).FindStringSubmatch(source)[1]
	for _, setting := range []string{`interval_seconds\s*=\s*10`, `timeout\s*=\s*5`, `failure_count_threshold\s*=\s*24`} {
		if !regexp.MustCompile(setting).MatchString(startup) {
			t.Errorf("startup probe needs a four-minute allowance (24 x 10s, 5s timeout): missing %s", setting)
		}
	}
	liveness := regexp.MustCompile(`(?s)dynamic "liveness_probe" \{(.*?)\n      \}`).FindStringSubmatch(source)[1]
	if !regexp.MustCompile(`failure_count_threshold\s*=\s*3`).MatchString(liveness) {
		t.Error("liveness must stay tight (3 failures); the startup allowance belongs to the startup probe, not liveness")
	}
}

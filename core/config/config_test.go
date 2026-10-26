package config

import "testing"

func TestObjectOperationUsageAcceptsScientificNotation(t *testing.T) {
	cfg, _, err := Parse([]byte(`version: 1
app: example
org: example
cloud: aws
region: us-east-1
vcs: github
runtime: serverless-containers
tier: solo
entry_path: adopt
repo: example/example
finops:
  usage:
    object_read_requests: 1e4
    object_write_requests: 1e3
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FinOps.Usage["object_read_requests"] != 10000 || cfg.FinOps.Usage["object_write_requests"] != 1000 {
		t.Fatalf("unexpected operation quantities: %v", cfg.FinOps.Usage)
	}
}

package main

import (
	"flag"
	"fmt"

	"github.com/bealesh/neckbeard/core/catalog"
	"github.com/bealesh/neckbeard/core/doctor"
)

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	stage := fs.String("for", "all", "plan | estimate | validate | all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cat, err := catalog.Load("")
	if err != nil {
		return err
	}
	checks, err := doctor.CheckTools(*stage, cat.OpenTofu)
	if err != nil {
		return err
	}
	fmt.Printf("READY  bundled catalog %s (%s)\n", cat.Version, cat.Digest)
	fmt.Println("Supported: AWS/GCP/Azure × GitHub/GitLab × serverless-containers/kubernetes; adopt existing accounts; one image, up to 10 services.")
	fmt.Println("Plan uses the bundled catalog and schemas; no external tools or cloud credentials are required.")
	failed := false
	for _, c := range checks {
		fmt.Printf("%-12s %-12s %-10s %s\n", c.Status, c.Tool, c.Version, c.Detail)
		failed = failed || c.Status != "READY"
	}
	fmt.Println("Tool availability only: pricing login, cloud permissions, regional quotas and deployment behavior are NOT EXERCISED.")
	if failed {
		return fmt.Errorf("some prerequisites are missing or incompatible; follow the instructions above, or choose -for plan/estimate/validate for a specific step")
	}
	return nil
}

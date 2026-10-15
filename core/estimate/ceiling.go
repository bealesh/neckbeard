package estimate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Summary is the machine-readable sidecar estimate writes next to the report
// (costs/estimate.json). scaffold reads it to enforce finops.monthly_ceiling,
// the one hard limit neckbeard offers: before anything exists. Budgets in the
// cloud notify; this refuses.
type Summary struct {
	BlueprintHash    string             `json:"blueprint_hash"`
	Currency         string             `json:"currency"`
	ExpectedTotal    float64            `json:"expected_total_monthly"`
	ExpectedByEnv    map[string]float64 `json:"expected_by_env"`
	GeneratedAt      string             `json:"generated_at"`
	InfracostVersion string             `json:"infracost_version"`
}

// SummaryPath is the sidecar's path for a given report path.
func SummaryPath(reportPath string) string {
	return filepath.Join(filepath.Dir(reportPath), "estimate.json")
}

func Summarize(res *Result, blueprintHash string, now time.Time) Summary {
	s := Summary{
		BlueprintHash: blueprintHash, Currency: res.Currency,
		ExpectedByEnv: map[string]float64{}, GeneratedAt: now.UTC().Format(time.RFC3339),
		InfracostVersion: res.InfracostVersion,
	}
	for _, e := range res.Envs {
		s.ExpectedByEnv[e.Env] = e.Costs["expected"]
		s.ExpectedTotal += e.Costs["expected"]
	}
	return s
}

func WriteSummary(path string, s Summary) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func ReadSummary(path string) (Summary, error) {
	var s Summary
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// CeilingError is returned when the expected total exceeds the ceiling.
type CeilingError struct {
	Expected, Ceiling float64
	Currency          string
}

func (e *CeilingError) Error() string {
	return fmt.Sprintf("expected total %.2f %s/month exceeds finops.monthly_ceiling %.2f by %.2f: lower the tier, change the runtime, drop a provisioned need, or raise the ceiling in neckbeard.yaml, then re-plan and re-estimate",
		e.Expected, e.Currency, e.Ceiling, e.Expected-e.Ceiling)
}

// CheckCeiling enforces the ceiling against the sidecar written for exactly
// this blueprint. A ceiling with no matching estimate is unenforceable, so
// that is an error too: fail closed, never "probably fine".
func CheckCeiling(summaryPath, blueprintHash string, ceiling float64) error {
	if ceiling <= 0 {
		return nil
	}
	s, err := ReadSummary(summaryPath)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("finops.monthly_ceiling is set but %s does not exist: run `neckbeard estimate` for this blueprint first (a ceiling with no estimate cannot be enforced, so scaffold refuses)", summaryPath)
	}
	if err != nil {
		return err
	}
	if s.BlueprintHash != blueprintHash {
		return fmt.Errorf("%s was produced for blueprint %s, current blueprint is %s: re-run `neckbeard estimate` before scaffolding under a ceiling", summaryPath, s.BlueprintHash, blueprintHash)
	}
	if s.ExpectedTotal > ceiling {
		return &CeilingError{Expected: s.ExpectedTotal, Ceiling: ceiling, Currency: s.Currency}
	}
	return nil
}

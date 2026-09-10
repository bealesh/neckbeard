package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func verifyHealth(ctx context.Context, a Artifact) error {
	if a.HealthURL == "" {
		return nil
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("health probe redirected away from HTTPS")
		}
		if len(via) > 5 {
			return fmt.Errorf("too many health redirects")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.HealthURL, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTPS health check: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTPS health check returned %d", res.StatusCode)
	}
	if a.ExpectedVersion != "" || a.ExpectedEnvironment != "" || a.RequireDatabase {
		var body struct {
			Version     string `json:"version"`
			Environment string `json:"env"`
			DB          string `json:"db"`
		}
		if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
			return err
		}
		if a.ExpectedVersion != "" && body.Version != a.ExpectedVersion {
			return fmt.Errorf("application identity mismatch: expected %s, got %s", a.ExpectedVersion, body.Version)
		}
		if a.ExpectedEnvironment != "" && body.Environment != a.ExpectedEnvironment {
			return fmt.Errorf("health endpoint serves a different environment")
		}
		if a.RequireDatabase && body.DB != "ok" {
			return fmt.Errorf("application database is not healthy: %s", body.DB)
		}
	}
	return nil
}

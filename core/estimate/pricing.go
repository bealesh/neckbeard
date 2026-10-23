package estimate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type rateLookup func(region, service, description string) (float64, error)

// Reuse the configured pricing service and credentials. Keys never enter command
// arguments, reports, fixtures or diagnostics. This is price lookup, not cloud auth.
func pricingLookup(bin, currency string) (rateLookup, error) {
	if !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(currency) {
		return nil, fmt.Errorf("invalid pricing currency")
	}
	key := os.Getenv("INFRACOST_API_KEY")
	if key == "" {
		b, err := exec.Command(bin, "configure", "get", "api_key").Output()
		if err != nil {
			return nil, fmt.Errorf("read Infracost API key: run infracost auth login")
		}
		key = strings.TrimSpace(string(b))
	}
	if key == "" {
		return nil, fmt.Errorf("missing Infracost API key: run infracost auth login")
	}
	endpoint := os.Getenv("INFRACOST_PRICING_API_ENDPOINT")
	if endpoint == "" {
		b, err := exec.Command(bin, "configure", "get", "pricing_api_endpoint").Output()
		if err != nil {
			return nil, fmt.Errorf("read Infracost pricing endpoint")
		}
		endpoint = strings.TrimSpace(string(b))
	}
	if endpoint == "" {
		endpoint = "https://pricing.api.infracost.io/graphql"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(endpoint, "/graphql") {
		endpoint += "/graphql"
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return nil, fmt.Errorf("pricing endpoint must use HTTPS (or localhost HTTP)")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return newRateLookup(client, endpoint, key, currency), nil
}

func newRateLookup(client *http.Client, endpoint, key, currency string) rateLookup {
	cache := map[string]float64{}
	return func(region, service, description string) (float64, error) {
		id := region + "\x00" + service + "\x00" + description
		if p, ok := cache[id]; ok {
			return p, nil
		}
		query := fmt.Sprintf(`{products(filter:{vendorName:"gcp",region:%q,service:%q,attributeFilters:[{key:"description",value_regex:%q}]}){prices{%s}}}`, region, service, description, currency)
		body, _ := json.Marshal(map[string]string{"query": query})
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return 0, fmt.Errorf("create pricing request")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", key)
		response, err := client.Do(req)
		if err != nil {
			return 0, fmt.Errorf("pricing API unavailable for %s", description)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("pricing API returned HTTP %d", response.StatusCode)
		}
		var result struct {
			Data struct {
				Products []struct {
					Prices []map[string]string `json:"prices"`
				} `json:"products"`
			} `json:"data"`
			Errors []json.RawMessage `json:"errors"`
		}
		if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil || len(result.Errors) > 0 {
			return 0, fmt.Errorf("invalid pricing API response for %s", description)
		}
		found := false
		var price float64
		for _, product := range result.Data.Products {
			for _, rates := range product.Prices {
				v, e := number(rates[currency], "unit price")
				if e != nil {
					return 0, e
				}
				if found && price != v && !strings.Contains(description, "Network Internet Data Transfer Out") && description != "/^Log Volume$/i" {
					return 0, fmt.Errorf("ambiguous unit price for %s", description)
				}
				if !found || v > price {
					price = v
				}
				found = true
			}
		}
		if !found {
			return 0, fmt.Errorf("no unit price for %s in %s", description, currency)
		}
		cache[id] = price
		return price, nil
	}
}

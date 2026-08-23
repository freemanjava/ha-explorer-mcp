package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// maxRESTResponseBytes bounds a single /api/config GET. Full response-size
// budgeting per doc §10 is Phase 02 policy work; this only stops the spike
// client from buffering an unbounded body.
const maxRESTResponseBytes = 1 << 20 // 1 MiB

// GetConfig performs GET {baseURL}/api/config against Core through the
// Supervisor REST proxy, authenticated with token as a bearer token. token
// is read once by the caller from SUPERVISOR_TOKEN and is never logged or
// embedded in any error this function returns.
func GetConfig(ctx context.Context, httpClient *http.Client, baseURL string, token string) (map[string]any, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/config", nil)
	if err != nil {
		return nil, fmt.Errorf("ha: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s/api/config", ErrUpstreamUnavailable, baseURL)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrAuthFailed
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: GET %s/api/config: status %d", ErrUpstreamUnavailable, baseURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRESTResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: reading response body", ErrUpstreamUnavailable)
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("%w: decoding response", ErrUnexpectedMessage)
	}
	return out, nil
}

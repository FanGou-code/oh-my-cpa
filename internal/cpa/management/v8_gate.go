package management

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

// APIGeneration names a CPA Management API base path.
type APIGeneration string

const (
	// APIGenerationV8 is /v8/management, the API Oh My CPA is built on.
	APIGenerationV8 APIGeneration = "v8"
	// APIGenerationV0 is /v0/management. It is addressed only for the reads the v8
	// tree does not offer (see client_legacy.go); nothing else may use it.
	APIGenerationV0 APIGeneration = "v0"
)

// ErrManagementV8Required is returned, before any request is sent, by every
// operation against a gateway that has answered that it does not serve the v8
// Management API. A CPA older than v8 would otherwise answer 404 to every path,
// which reads as a scattering of "unsupported operation" errors rather than the
// one fact the operator has to act on.
var ErrManagementV8Required = errors.New("CPA does not serve the v8 Management API; Oh My CPA requires CPA v8.0.0 or later")

// ManagementAPIStatus is the gate's answer as the console shows it.
type ManagementAPIStatus string

const (
	ManagementAPIV8          ManagementAPIStatus = "v8"
	ManagementAPIUnsupported ManagementAPIStatus = "unsupported"
	// ManagementAPIUnknown means the probe got no answer (unreachable, 401, 5xx).
	ManagementAPIUnknown ManagementAPIStatus = "unknown"
)

// MANAGEMENT_V8_PROBE_ENDPOINT is the read the gate is decided by.
//
// Support is observed, never inferred from a version string: builds, forks and
// release tags do not map reliably onto routes, while a route either answers or
// it does not. This field is always present in a v8 view of the configuration
// (GET never migrates the file, it renders it), it is a single small integer,
// and a gateway older than v8 answers 404 because the whole /v8 tree is
// unregistered.
const MANAGEMENT_V8_PROBE_ENDPOINT = "/config/config-version"

// API_SUPPORT_TTL bounds how long a v8 answer is trusted. Gateways are replaced
// in place behind the same base URL, and a v8 route that answers "missing"
// invalidates the answer immediately, so the TTL only has to bound how late a
// rollback is noticed.
const API_SUPPORT_TTL = 5 * time.Minute

// API_UNSUPPORTED_TTL is shorter: a "not v8" answer blocks the whole console, and
// an operator who has just upgraded CPA should not wait minutes to see it lift.
// The console's health poll re-asks on this cadence.
const API_UNSUPPORTED_TTL = 15 * time.Second

type apiSupportEntry struct {
	hasV8     bool
	expiresAt time.Time
}

// Clients are built per request, so the answer is kept per gateway: without it
// every usage-queue poll would pay a probe round trip.
var apiSupportCache = struct {
	sync.Mutex
	entries map[string]apiSupportEntry
}{entries: map[string]apiSupportEntry{}}

// SupportsManagementV8 reports whether the gateway serves /v8/management.
//
// Only a definite answer is cached: the value 8 means supported, a
// missing-capability status or any other body means not. Anything else
// (unreachable, 401, 5xx) is returned as an error and not remembered, so one bad
// moment cannot block the console.
func (c *Client) SupportsManagementV8(ctx context.Context) (bool, error) {
	if c == nil {
		return false, errors.New("CPA client is not initialized")
	}
	apiSupportCache.Lock()
	entry, found := apiSupportCache.entries[c.baseURL]
	apiSupportCache.Unlock()
	if found && time.Now().Before(entry.expiresAt) {
		return entry.hasV8, nil
	}
	request, err := c.newRequestAt(ctx, http.MethodGet, APIGenerationV8, MANAGEMENT_V8_PROBE_ENDPOINT, nil, "")
	if err != nil {
		return false, err
	}
	// The probe is the one request that bypasses the gate it decides.
	data, _, err := c.sendBytes(request, 4*1024)
	if err != nil && !IsMissingCapability(err) {
		return false, err
	}
	// The value is checked, not just the status: a proxy or catch-all route that
	// answers 2xx to any path must not be mistaken for the v8 tree.
	var version int
	hasV8 := err == nil && json.Unmarshal(bytes.TrimSpace(data), &version) == nil && version == 8
	c.rememberManagementV8(hasV8)
	return hasV8, nil
}

// ManagementAPI folds SupportsManagementV8 into the status the console renders.
func (c *Client) ManagementAPI(ctx context.Context) ManagementAPIStatus {
	hasV8, err := c.SupportsManagementV8(ctx)
	switch {
	case err != nil:
		return ManagementAPIUnknown
	case hasV8:
		return ManagementAPIV8
	default:
		return ManagementAPIUnsupported
	}
}

// requireManagementV8 is the gate every management request passes. An undecided
// probe lets the request through: it will fail on its own with the error that
// actually describes the problem (unreachable, wrong key), which is more useful
// than a guess about the version.
func (c *Client) requireManagementV8(ctx context.Context) error {
	if hasV8, err := c.SupportsManagementV8(ctx); err == nil && !hasV8 {
		return ErrManagementV8Required
	}
	return nil
}

func (c *Client) rememberManagementV8(hasV8 bool) {
	ttl := API_SUPPORT_TTL
	if !hasV8 {
		ttl = API_UNSUPPORTED_TTL
	}
	apiSupportCache.Lock()
	defer apiSupportCache.Unlock()
	apiSupportCache.entries[c.baseURL] = apiSupportEntry{hasV8: hasV8, expiresAt: time.Now().Add(ttl)}
}

// forgetManagementV8 drops the cached answer after a v8 route answered
// "missing", so a gateway rolled back behind the same URL is re-probed on the
// next call instead of producing unsupported-operation errors until the TTL.
func (c *Client) forgetManagementV8() {
	apiSupportCache.Lock()
	defer apiSupportCache.Unlock()
	delete(apiSupportCache.entries, c.baseURL)
}

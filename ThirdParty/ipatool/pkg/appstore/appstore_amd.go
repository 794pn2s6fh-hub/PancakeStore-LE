package appstore

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/majd/ipatool/v2/internal/storeauth"
	"github.com/majd/ipatool/v2/pkg/http"
)

// ErrStoreAMDSynchronizationRequired indicates Apple rejected the current
// Store machine state and asked the client to synchronize it before retrying.
// The native bridge can recover from this response by refreshing its Store
// authentication once, matching the app's existing manual sign-in recovery.
var ErrStoreAMDSynchronizationRequired = errors.New("Apple requires Store AMD machine synchronization")

func (t *appstore) storeAMDProof(ctx context.Context, endpoint string, hardwareID []byte) (map[string]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if len(hardwareID) != 6 {
		return nil, errors.New("store AMD proof requires a six-byte device GUID")
	}

	if !isStoreAMDCommerceURL(endpoint) {
		return nil, errors.New("store AMD proof endpoint is not allowlisted")
	}

	t.authStateMu.Lock()
	defer t.authStateMu.Unlock()

	if len(t.anisetteState) == 0 && t.keychain != nil {
		t.anisetteState, _ = t.keychain.Get(anisetteKeychainKey)
	}

	proof, state, err := storeauth.StoreAMDHeaders(ctx, t.anisetteState, endpoint, hardwareID)
	if err != nil {
		return nil, fmt.Errorf("generate Store AMD proof: %w", err)
	}

	if t.keychain == nil {
		return nil, errors.New("keychain is not configured")
	}

	if err = t.keychain.Set(anisetteKeychainKey, state); err != nil {
		return nil, fmt.Errorf("save Store AMD state: %w", err)
	}

	t.anisetteState = state

	return proof, nil
}

func isStoreAMDCommerceURL(endpoint string) bool {
	if strings.Contains(endpoint, "#") {
		return false
	}

	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" ||
		parsed.RawPath != "" || parsed.Opaque != "" || (parsed.Port() != "" && parsed.Port() != "443") {
		return false
	}

	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || parsed.ForceQuery || len(query) != 1 {
		return false
	}

	values, ok := query["guid"]
	if !ok || len(values) != 1 || values[0] == "" {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	switch host {
	case downloadDispatchDomain:
		switch parsed.Path {
		case entDownloadPath, redownloadProductPath, updateProductPath:
			return true
		}
	default:
		if !isStoreBuyHost(host) {
			return false
		}

		switch parsed.Path {
		case PrivateAppStoreAPIPathDownload:
			return true
		}
	}

	return false
}

func isStoreBuyHost(host string) bool {
	if host == PrivateAppStoreAPIDomain {
		return true
	}

	const suffix = "-" + PrivateAppStoreAPIDomain

	pod := strings.TrimSuffix(host, suffix)
	if pod == host || len(pod) < 2 || pod[0] != 'p' {
		return false
	}

	for _, digit := range pod[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}

	return true
}

func storeAMDSynchronizationAction(res http.Result[downloadResult]) bool {
	for key, value := range res.Headers {
		if strings.EqualFold(key, "X-Apple-AMD-Action") && isStoreAMDSynchronizationAction(value) {
			return true
		}
	}

	return isStoreAMDSynchronizationAction(res.Data.CustomerMessage)
}

func isStoreAMDSynchronizationAction(value string) bool {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "SM") || strings.EqualFold(value, "volumeStoreDownloadProduct:SM") {
		return true
	}

	// Some Store responses expose the action in customerMessage instead of
	// the dedicated response header. Accept only Apple's exact action token;
	// unrelated messages containing the letters "SM" must not trigger login.
	parts := strings.Split(value, ":")
	return len(parts) == 3 &&
		strings.EqualFold(strings.TrimSpace(parts[0]), "AMD-Action") &&
		strings.EqualFold(strings.TrimSpace(parts[1]), "volumeStoreDownloadProduct") &&
		strings.EqualFold(strings.TrimSpace(parts[2]), "SM")
}

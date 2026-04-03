package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/targets"
)

var (
	// ErrSecretNotFound indicates the referenced secret cannot be found.
	ErrSecretNotFound = errors.New("secret not found")
	// ErrSecretRefInvalid indicates malformed secret reference format.
	ErrSecretRefInvalid = errors.New("invalid secret reference")
	// ErrProviderUnsupported indicates provider/scheme is not available on this host.
	ErrProviderUnsupported = errors.New("secret provider unsupported")
	// ErrOAuthValidation indicates oauth validation failed.
	ErrOAuthValidation = errors.New("oauth validation failed")
)

// SecretResolver resolves secret refs like env://FOO, file:///tmp/s, keychain://svc/account.
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// OAuthValidator validates OAuth configuration/readiness and unattended viability.
type OAuthValidator interface {
	Validate(ctx context.Context, in OAuthValidationInput) (OAuthValidationResult, error)
}

// OAuthValidationInput describes data needed for oauth readiness checks.
type OAuthValidationInput struct {
	ClientID     string
	TokenURL     string
	RedirectURL  string
	CallbackID   string
	Scopes       []string
	ClientSecret string
}

// OAuthValidationResult is a normalized oauth validation result.
type OAuthValidationResult struct {
	Ready     bool
	Message   string
	ExpiresAt *time.Time
	Metadata  map[string]string
}

// HeaderMaterial contains auth headers and optional oauth metadata.
type HeaderMaterial struct {
	Headers         map[string]string
	TokenExpiresAt  *time.Time
	OAuthValidation OAuthValidationResult
}

// Manager coordinates auth material creation and validation hooks.
type Manager struct {
	secrets SecretResolver
	oauth   OAuthValidator
}

// NewManager constructs an auth manager with chain resolvers and default oauth validator.
func NewManager() *Manager {
	return &Manager{
		secrets: NewChainResolver(
			EnvResolver{},
			FileResolver{},
			NewKeyringResolver(),
		),
		oauth: DefaultOAuthValidator{},
	}
}

// NewManagerWithOverrides allows swapping secret/oAuth backends.
func NewManagerWithOverrides(secretResolver SecretResolver, oauthValidator OAuthValidator) *Manager {
	m := NewManager()
	if secretResolver != nil {
		m.secrets = secretResolver
	}
	if oauthValidator != nil {
		m.oauth = oauthValidator
	}
	return m
}

// BuildHeaders resolves auth material from target auth config.
func (m *Manager) BuildHeaders(ctx context.Context, cfg targets.AuthConfig) (HeaderMaterial, error) {
	headers := make(map[string]string)

	switch cfg.Type {
	case "", targets.AuthNone:
		return HeaderMaterial{Headers: headers}, nil

	case targets.AuthBearer:
		if cfg.SecretRef == "" {
			return HeaderMaterial{}, fmt.Errorf("bearer auth requires secret_ref")
		}
		secret, err := m.secrets.Resolve(ctx, cfg.SecretRef)
		if err != nil {
			return HeaderMaterial{}, fmt.Errorf("resolve bearer secret: %w", err)
		}
		h := cfg.Header
		if h == "" {
			h = "Authorization"
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(secret)), "bearer ") {
			headers[h] = secret
		} else {
			headers[h] = "Bearer " + secret
		}
		return HeaderMaterial{Headers: headers}, nil

	case targets.AuthAPIKey:
		if cfg.SecretRef == "" || cfg.KeyName == "" {
			return HeaderMaterial{}, fmt.Errorf("apikey auth requires key_name and secret_ref")
		}
		secret, err := m.secrets.Resolve(ctx, cfg.SecretRef)
		if err != nil {
			return HeaderMaterial{}, fmt.Errorf("resolve api key secret: %w", err)
		}
		headers[cfg.KeyName] = secret
		return HeaderMaterial{Headers: headers}, nil

	case targets.AuthOAuth:
		if cfg.ClientID == "" || cfg.TokenURL == "" || cfg.RedirectURL == "" || cfg.CallbackID == "" {
			return HeaderMaterial{}, fmt.Errorf("oauth auth requires client_id, token_url, redirect_url, and callback_id")
		}
		var clientSecret string
		if cfg.SecretRef != "" {
			secret, err := m.secrets.Resolve(ctx, cfg.SecretRef)
			if err != nil {
				return HeaderMaterial{}, fmt.Errorf("resolve oauth client secret: %w", err)
			}
			clientSecret = secret
		}

		result, err := m.oauth.Validate(ctx, OAuthValidationInput{
			ClientID:     cfg.ClientID,
			TokenURL:     cfg.TokenURL,
			RedirectURL:  cfg.RedirectURL,
			CallbackID:   cfg.CallbackID,
			Scopes:       cfg.Scopes,
			ClientSecret: clientSecret,
		})
		if err != nil {
			return HeaderMaterial{}, fmt.Errorf("%w: %v", ErrOAuthValidation, err)
		}
		if !result.Ready {
			msg := result.Message
			if msg == "" {
				msg = "oauth validator reported not ready"
			}
			return HeaderMaterial{}, fmt.Errorf("%w: %s", ErrOAuthValidation, msg)
		}

		// Hook point only: this manager validates readiness but does not exchange tokens.
		// Token retrieval/refresh can be layered in later and then set Authorization here.
		return HeaderMaterial{
			Headers:         headers,
			TokenExpiresAt:  result.ExpiresAt,
			OAuthValidation: result,
		}, nil

	default:
		return HeaderMaterial{}, fmt.Errorf("unsupported auth type %q", cfg.Type)
	}
}

// DefaultOAuthValidator performs syntactic/readiness validation only.
// It intentionally avoids network/token exchange to keep footprint minimal.
type DefaultOAuthValidator struct{}

func (d DefaultOAuthValidator) Validate(_ context.Context, in OAuthValidationInput) (OAuthValidationResult, error) {
	if strings.TrimSpace(in.ClientID) == "" {
		return OAuthValidationResult{}, errors.New("client_id is required")
	}
	if strings.TrimSpace(in.TokenURL) == "" {
		return OAuthValidationResult{}, errors.New("token_url is required")
	}
	u, err := url.Parse(in.TokenURL)
	if err != nil {
		return OAuthValidationResult{}, fmt.Errorf("invalid token_url: %w", err)
	}
	if u.Scheme != "https" && !isLocalHTTPTokenURL(u) {
		return OAuthValidationResult{}, errors.New("token_url must use https (http allowed only for localhost)")
	}

	if strings.TrimSpace(in.RedirectURL) == "" {
		return OAuthValidationResult{}, errors.New("redirect_url is required")
	}
	ru, err := url.Parse(in.RedirectURL)
	if err != nil {
		return OAuthValidationResult{}, fmt.Errorf("invalid redirect_url: %w", err)
	}
	if ru.Scheme != "https" && !isLocalHTTPTokenURL(ru) {
		return OAuthValidationResult{}, errors.New("redirect_url must use https (http allowed only for localhost)")
	}

	if strings.TrimSpace(in.CallbackID) == "" {
		return OAuthValidationResult{}, errors.New("callback_id is required")
	}

	if in.ClientSecret == "" {
		return OAuthValidationResult{
			Ready:    false,
			Message:  "missing client secret",
			Metadata: map[string]string{"reason": "missing_secret"},
		}, nil
	}
	return OAuthValidationResult{
		Ready:   true,
		Message: "oauth config appears valid for unattended flow hook",
		Metadata: map[string]string{
			"validator":   "default",
			"callback_id": in.CallbackID,
		},
	}, nil
}

func isLocalHTTPTokenURL(u *url.URL) bool {
	if u.Scheme != "http" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// HookOAuthValidator allows callers to inject custom oauth validation logic.
type HookOAuthValidator struct {
	ValidateFunc func(ctx context.Context, in OAuthValidationInput) (OAuthValidationResult, error)
}

func (h HookOAuthValidator) Validate(ctx context.Context, in OAuthValidationInput) (OAuthValidationResult, error) {
	if h.ValidateFunc == nil {
		return DefaultOAuthValidator{}.Validate(ctx, in)
	}
	return h.ValidateFunc(ctx, in)
}

// ChainResolver tries multiple resolvers in sequence.
type ChainResolver struct {
	resolvers []SecretResolver
}

// NewChainResolver creates a fallback chain.
func NewChainResolver(resolvers ...SecretResolver) ChainResolver {
	return ChainResolver{resolvers: resolvers}
}

func (c ChainResolver) Resolve(ctx context.Context, ref string) (string, error) {
	var errs []error
	for _, r := range c.resolvers {
		v, err := r.Resolve(ctx, ref)
		if err == nil {
			return v, nil
		}
		// Keep trying only for unsupported/invalid ref/no match errors.
		if errors.Is(err, ErrProviderUnsupported) || errors.Is(err, ErrSecretRefInvalid) || errors.Is(err, ErrSecretNotFound) {
			errs = append(errs, err)
			continue
		}
		return "", err
	}
	if len(errs) == 0 {
		return "", fmt.Errorf("%w: %q", ErrSecretNotFound, ref)
	}
	return "", errors.Join(errs...)
}

// EnvResolver supports env://VAR_NAME
type EnvResolver struct{}

func (EnvResolver) Resolve(_ context.Context, ref string) (string, error) {
	const prefix = "env://"
	if !strings.HasPrefix(ref, prefix) {
		return "", ErrProviderUnsupported
	}
	key := strings.TrimPrefix(ref, prefix)
	if key == "" {
		return "", ErrSecretRefInvalid
	}
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}
	return val, nil
}

// FileResolver supports file:///absolute/path or file://relative/path.
type FileResolver struct{}

func (FileResolver) Resolve(_ context.Context, ref string) (string, error) {
	const prefix = "file://"
	if !strings.HasPrefix(ref, prefix) {
		return "", ErrProviderUnsupported
	}
	path := strings.TrimPrefix(ref, prefix)
	if path == "" {
		return "", ErrSecretRefInvalid
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrSecretNotFound, path)
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// KeyringResolver resolves keychain://service/account using host-native mechanisms.
//
// macOS: security find-generic-password -s <service> -a <account> -w
// Linux: secret-tool lookup service <service> account <account>
type KeyringResolver struct{}

func NewKeyringResolver() KeyringResolver { return KeyringResolver{} }

func (KeyringResolver) Resolve(ctx context.Context, ref string) (string, error) {
	service, account, err := parseKeyringRef(ref)
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return resolveFromMacKeychain(ctx, service, account)
	case "linux":
		return resolveFromSecretService(ctx, service, account)
	default:
		return "", ErrProviderUnsupported
	}
}

func parseKeyringRef(ref string) (service, account string, err error) {
	const prefix = "keychain://"
	if !strings.HasPrefix(ref, prefix) {
		return "", "", ErrProviderUnsupported
	}
	rest := strings.TrimPrefix(ref, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: expected keychain://<service>/<account>", ErrSecretRefInvalid)
	}
	service = strings.TrimSpace(parts[0])
	account = strings.TrimSpace(parts[1])
	if service == "" || account == "" {
		return "", "", fmt.Errorf("%w: empty service/account", ErrSecretRefInvalid)
	}
	return service, account, nil
}

func resolveFromMacKeychain(ctx context.Context, service, account string) (string, error) {
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-a", account, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: mac keychain lookup failed for %s/%s: %v", ErrSecretNotFound, service, account, err)
	}
	secret := strings.TrimSpace(string(out))
	if secret == "" {
		return "", fmt.Errorf("%w: empty value for %s/%s", ErrSecretNotFound, service, account)
	}
	return secret, nil
}

func resolveFromSecretService(ctx context.Context, service, account string) (string, error) {
	cmd := exec.CommandContext(ctx, "secret-tool", "lookup", "service", service, "account", account)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: secret-service lookup failed for %s/%s: %v", ErrSecretNotFound, service, account, err)
	}
	secret := strings.TrimSpace(string(out))
	if secret == "" {
		return "", fmt.Errorf("%w: empty value for %s/%s", ErrSecretNotFound, service, account)
	}
	return secret, nil
}

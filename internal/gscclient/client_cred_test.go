package gscclient

import (
	"strings"
	"testing"

	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"google.golang.org/api/option"
)

func TestCredentialOptions_ServiceAccount(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		CredentialsJSON: []byte(`{"type":"service_account"}`),
		CredType:        config.CredTypeServiceAccount,
	}
	opts, err := credentialOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) < 2 {
		t.Fatalf("expected scopes + credentials, got %d opts", len(opts))
	}
}

func TestCredentialOptions_AuthorizedUserWithQuota(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		CredentialsJSON: []byte(`{"type":"authorized_user"}`),
		CredType:        config.CredTypeAuthorizedUser,
		QuotaProjectID:  "my-quota-project",
	}
	opts, err := credentialOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// authorized_user: credentials + quota project, no WithScopes
	if len(opts) != 2 {
		t.Fatalf("expected 2 opts (creds + quota), got %d", len(opts))
	}
	// Apply to dial settings to assert quota project is present.
	// We only check that WithQuotaProject is among options by counting.
	_ = option.WithQuotaProject
}

func TestCredentialOptions_UnsupportedType(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		CredentialsJSON: []byte(`{}`),
		CredType:        config.CredType("external_account"),
	}
	_, err := credentialOptions(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported credential type") {
		t.Fatalf("got %v", err)
	}
}

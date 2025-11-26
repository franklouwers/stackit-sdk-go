# CLI Provider Authentication Integration Guide

This guide explains how to integrate STACKIT CLI provider authentication with the SDK.

## Overview

The SDK provides an interface-based approach for CLI authentication that avoids circular dependencies. The integration happens in the consumer (e.g., Terraform Provider), not in the SDK itself.

## Architecture

```
┌─────────────────────────────────────────────┐
│ Terraform Provider (Integration Layer)      │
│ - Imports CLI package                       │
│ - Imports SDK package                       │
│ - Creates CLIAuthProvider implementation    │
└─────────────────────────────────────────────┘
            │                    │
            │                    │
            ▼                    ▼
┌─────────────────┐    ┌─────────────────┐
│ STACKIT CLI     │    │ STACKIT SDK     │
│ - Auth storage  │    │ - CLIAuthProvider│
│ - OAuth flows   │    │   interface      │
│ - Token refresh │    │ - No CLI import  │
└─────────────────┘    └─────────────────┘
```

## Implementation in Terraform Provider

### Step 1: Create the Adapter

Create a file `internal/provider/cli_auth_adapter.go`:

```go
package provider

import (
    "net/http"

    cliAuth "github.com/stackitcloud/stackit-cli/pkg/auth"
    "github.com/stackitcloud/stackit-cli/internal/pkg/print"
    sdkConfig "github.com/stackitcloud/stackit-sdk-go/core/config"
    "github.com/spf13/cobra"
)

// CLIAuthAdapter implements sdkConfig.CLIAuthProvider to bridge
// between the STACKIT CLI and STACKIT SDK.
type CLIAuthAdapter struct {
    printer *print.Printer
}

// NewCLIAuthAdapter creates a new CLI auth adapter.
// The printer parameter is optional; pass nil for no debug output.
func NewCLIAuthAdapter(printer *print.Printer) *CLIAuthAdapter {
    return &CLIAuthAdapter{printer: printer}
}

// IsAuthenticated checks if CLI provider credentials exist.
func (a *CLIAuthAdapter) IsAuthenticated() bool {
    return cliAuth.IsProviderAuthenticated()
}

// GetAuthFlow returns an http.RoundTripper configured with CLI authentication.
func (a *CLIAuthAdapter) GetAuthFlow() (http.RoundTripper, error) {
    return cliAuth.ProviderAuthFlow(a.printer)
}

// Helper function to create a minimal printer for CLI integration
func createPrinter() *print.Printer {
    cmd := &cobra.Command{}
    // Configure output if needed
    return &print.Printer{Cmd: cmd}
}
```

### Step 2: Update Provider Configuration

In your provider configuration (e.g., `internal/provider/provider.go`):

```go
package provider

import (
    "context"
    "fmt"

    sdkConfig "github.com/stackitcloud/stackit-sdk-go/core/config"
    "github.com/stackitcloud/stackit-sdk-go/services/dns"
)

// ProviderConfig represents the provider configuration
type ProviderConfig struct {
    ServiceAccountToken   string
    ServiceAccountKeyPath string
    CLIAuth               bool  // New field
}

// ConfigureClient creates and configures an SDK client based on provider config
func ConfigureClient(config *ProviderConfig) error {
    var authOption sdkConfig.ConfigurationOption

    // Authentication priority order:
    // 1. Explicit service account token
    // 2. Explicit service account key
    // 3. CLI provider auth (if enabled)
    // 4. Environment variables (handled by SDK default auth)

    if config.ServiceAccountToken != "" {
        authOption = sdkConfig.WithToken(config.ServiceAccountToken)
    } else if config.ServiceAccountKeyPath != "" {
        authOption = sdkConfig.WithServiceAccountKeyPath(config.ServiceAccountKeyPath)
    } else if config.CLIAuth {
        // Use CLI authentication
        adapter := NewCLIAuthAdapter(createPrinter())

        if !adapter.IsAuthenticated() {
            return fmt.Errorf("CLI authentication enabled but not authenticated. Please run: stackit auth provider login")
        }

        authOption = sdkConfig.WithCLIProviderAuth(adapter)
    }
    // else: Let SDK use default auth (env vars, credentials file)

    // Create client with auth option
    client, err := dns.NewAPIClient(authOption)
    if err != nil {
        return fmt.Errorf("failed to create SDK client: %w", err)
    }

    // Store client for use by resources/data sources
    // ... (your implementation)

    return nil
}
```

### Step 3: Update Provider Schema

Add the `cli_auth` attribute to your provider schema:

```go
func Provider() *schema.Provider {
    return &schema.Provider{
        Schema: map[string]*schema.Schema{
            "service_account_token": {
                Type:        schema.TypeString,
                Optional:    true,
                Sensitive:   true,
                Description: "Service account access token",
            },
            "service_account_key_path": {
                Type:        schema.TypeString,
                Optional:    true,
                Description: "Path to service account key JSON file",
            },
            "cli_auth": {
                Type:        schema.TypeBool,
                Optional:    true,
                Default:     false,
                Description: "Use STACKIT CLI provider authentication. Run 'stackit auth provider login' first.",
            },
        },
        // ... other configuration
    }
}
```

## Usage Example

Users can now configure the Terraform provider to use CLI authentication:

```hcl
terraform {
  required_providers {
    stackit = {
      source  = "stackitcloud/stackit"
      version = "~> 1.0"
    }
  }
}

provider "stackit" {
  cli_auth = true  # Use CLI authentication
}

resource "stackit_dns_zone" "example" {
  name = "example.com"
  # ... other configuration
}
```

Before running Terraform, users authenticate via CLI:

```bash
$ stackit auth provider login
# Opens browser for OAuth authentication
# Credentials stored in keyring/file

$ terraform plan
# Uses CLI credentials automatically
```

## Authentication Priority

The recommended authentication priority order:

1. **Explicit credentials** (highest priority)
   - `service_account_token`
   - `service_account_key_path`

2. **CLI authentication** (when `cli_auth = true`)
   - Checks `IsProviderAuthenticated()`
   - Uses credentials from `stackit auth provider login`

3. **Environment variables** (lowest priority)
   - `STACKIT_SERVICE_ACCOUNT_TOKEN`
   - `STACKIT_SERVICE_ACCOUNT_KEY_PATH`
   - Handled by SDK's default authentication

## Error Handling

Handle CLI auth errors appropriately:

```go
adapter := NewCLIAuthAdapter(printer)

if !adapter.IsAuthenticated() {
    return &AuthError{
        Summary: "CLI Authentication Not Found",
        Detail: "The provider is configured to use CLI authentication (cli_auth = true), " +
                "but no authentication was found. Please run: stackit auth provider login",
    }
}

authOption := sdkConfig.WithCLIProviderAuth(adapter)
client, err := dns.NewAPIClient(authOption)
if err != nil {
    var authErr *sdkConfig.AuthenticationError
    if errors.As(err, &authErr) {
        return &AuthError{
            Summary: "CLI Authentication Failed",
            Detail:  fmt.Sprintf("Failed to initialize CLI authentication: %v", authErr),
        }
    }
    return err
}
```

## Testing

### Unit Tests

Test the adapter implementation:

```go
func TestCLIAuthAdapter_IsAuthenticated(t *testing.T) {
    adapter := NewCLIAuthAdapter(nil)

    // Test will depend on actual CLI state
    // For unit tests, consider mocking the CLI package
    isAuth := adapter.IsAuthenticated()

    // Assert based on test environment
    t.Logf("IsAuthenticated: %v", isAuth)
}
```

### Integration Tests

Test the full provider configuration:

```go
func TestProvider_CLIAuth(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    config := &ProviderConfig{
        CLIAuth: true,
    }

    err := ConfigureClient(config)

    // Skip if CLI not authenticated (for CI/CD)
    if err != nil && strings.Contains(err.Error(), "not authenticated") {
        t.Skip("CLI authentication not available")
    }

    if err != nil {
        t.Fatalf("Failed to configure client: %v", err)
    }
}
```

## Benefits

### For Users
- ✅ No service account needed for local development
- ✅ Use same authentication as CLI
- ✅ Automatic token refresh
- ✅ Credentials stored securely (keyring)

### For Maintainers
- ✅ No circular dependencies
- ✅ Clean separation of concerns
- ✅ Interface-based design (testable)
- ✅ Minimal code (~15 lines)

## Troubleshooting

### "CLI authentication enabled but not authenticated"

**Cause:** User has `cli_auth = true` but hasn't run `stackit auth provider login`.

**Solution:**
```bash
stackit auth provider login
```

### "Failed to initialize CLI authentication"

**Cause:** CLI credentials exist but are invalid/expired and refresh failed.

**Solution:**
```bash
stackit auth provider logout
stackit auth provider login
```

### "Import cycle" errors during build

**Cause:** Incorrect import structure or direct CLI import in SDK.

**Solution:** Ensure the SDK doesn't import the CLI package. Only the provider should import both.

## See Also

- [STACKIT CLI Documentation](https://github.com/stackitcloud/stackit-cli)
- [STACKIT SDK Documentation](https://github.com/stackitcloud/stackit-sdk-go)
- [SDK CLI Auth API Reference](./cli_auth.go)

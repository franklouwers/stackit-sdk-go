# Full Integration Testing Guide

This guide walks through building and testing the complete CLI → SDK → Terraform Provider flow.

## Prerequisites

```bash
# Directory structure
~/projects/
├── stackit-cli/           # CLI fork
├── stackit-sdk-go/        # SDK fork
└── terraform-provider-stackit/  # Provider fork
```

## Part 1: Build & Test CLI

### 1.1 Build the CLI

```bash
cd ~/projects/stackit-cli

# Ensure you're on the correct branch
git checkout claude/terraform-provider-login-015Z7hLQUGxJDbBdv9HKYEhT

# Build the CLI
go build -o stackit

# Verify build
./stackit version
```

### 1.2 Test CLI Unit Tests

```bash
# Run all tests
go test ./...

# Run auth package tests specifically
go test -v ./internal/pkg/auth
go test -v ./pkg/auth

# Run provider auth tests
go test -v ./pkg/auth -run TestProviderAuthFlow
go test -v ./pkg/auth -run TestIsProviderAuthenticated
```

### 1.3 Manual CLI Test - Provider Login

```bash
# Test provider authentication flow
./stackit auth provider login

# Expected: Opens browser for OAuth2 authentication
# After auth: Should see success message

# Verify authentication status
./stackit auth provider status
# Expected output: "Provider Authentication Status: Authenticated"

# Get access token (verifies token retrieval works)
./stackit auth provider get-access-token
# Expected: Should print a JWT token

# Check where credentials are stored
# Keyring (if available):
# - macOS: Keychain Access -> search "stackit-cli-provider"
# - Windows: Credential Manager -> search "stackit-cli-provider"
# - Linux: Secret Service

# File fallback:
cat ~/.stackit/cli-provider-auth-storage.txt | base64 -d | jq
# Should show: auth_flow_type, access_token, refresh_token, etc.
```

## Part 2: Build & Test SDK

### 2.1 Build the SDK

```bash
cd ~/projects/stackit-sdk-go

# Ensure you're on the correct branch
git checkout claude/add-external-auth-terraform-01HnhMdS5HrkMXZuRNW4NjXz

# Update go.work (if needed)
# Make sure go version is 1.24.0

# Build the SDK
go build ./core/...

# Verify no import cycle
go build ./core/config
```

### 2.2 Test SDK Unit Tests

```bash
# Run all SDK config tests
go test -v ./core/config

# Run CLI auth tests specifically
go test -v ./core/config -run TestWithCLIProviderAuth
go test -v ./core/config -run TestAuthenticationError
go test -v ./core/config -run TestCLIAuthProvider

# Verify all tests pass
go test ./core/...
```

### 2.3 SDK Integration Test (Manual)

Create a test file to verify the interface works:

```bash
# Create test file
cat > /tmp/sdk_cli_test.go << 'EOF'
package main

import (
    "fmt"
    "net/http"

    sdkConfig "github.com/stackitcloud/stackit-sdk-go/core/config"
)

// Mock implementation for testing
type mockProvider struct{}

func (m *mockProvider) IsAuthenticated() bool {
    fmt.Println("✓ IsAuthenticated() called")
    return true
}

func (m *mockProvider) GetAuthFlow() (http.RoundTripper, error) {
    fmt.Println("✓ GetAuthFlow() called")
    return http.DefaultTransport, nil
}

func main() {
    fmt.Println("Testing SDK CLIAuthProvider interface...")

    provider := &mockProvider{}

    cfg := &sdkConfig.Configuration{}
    opt := sdkConfig.WithCLIProviderAuth(provider)

    if err := opt(cfg); err != nil {
        fmt.Printf("✗ Error: %v\n", err)
        return
    }

    fmt.Println("✓ SDK configuration successful")

    if cfg.CustomAuth == nil {
        fmt.Println("✗ CustomAuth not set")
        return
    }

    fmt.Println("✓ CustomAuth configured correctly")
    fmt.Println("\n✅ SDK interface test PASSED")
}
EOF

# Run the test
cd ~/projects/stackit-sdk-go
go run /tmp/sdk_cli_test.go
```

Expected output:
```
Testing SDK CLIAuthProvider interface...
✓ IsAuthenticated() called
✓ GetAuthFlow() called
✓ SDK configuration successful
✓ CustomAuth configured correctly

✅ SDK interface test PASSED
```

## Part 3: Build & Test Terraform Provider

### 3.1 Setup Provider with Local Modules

Update provider's `go.mod` to use local forks:

```bash
cd ~/projects/terraform-provider-stackit

# Add replace directives for local development
cat >> go.mod << 'EOF'

// Local development - point to your forks
replace github.com/stackitcloud/stackit-sdk-go/core => ../stackit-sdk-go/core
replace github.com/stackitcloud/stackit-cli => ../stackit-cli
EOF

go mod tidy
```

### 3.2 Implement the Adapter

Create `internal/provider/cli_auth_adapter.go`:

```go
package provider

import (
    "net/http"

    cliAuth "github.com/stackitcloud/stackit-cli/pkg/auth"
    sdkConfig "github.com/stackitcloud/stackit-sdk-go/core/config"
)

// CLIAuthAdapter implements sdkConfig.CLIAuthProvider
type CLIAuthAdapter struct{}

func (a *CLIAuthAdapter) IsAuthenticated() bool {
    return cliAuth.IsProviderAuthenticated()
}

func (a *CLIAuthAdapter) GetAuthFlow() (http.RoundTripper, error) {
    // Pass nil for printer since provider doesn't have CLI output
    return cliAuth.ProviderAuthFlow(nil)
}
```

### 3.3 Update Provider Configuration

In `internal/provider/provider.go`, add CLI auth support:

```go
// In your provider config struct
type ProviderConfig struct {
    // ... existing fields ...
    CLIAuth bool
}

// In your authentication setup
func setupAuth(config *ProviderConfig) (sdkConfig.ConfigurationOption, error) {
    // Priority order
    if config.ServiceAccountToken != "" {
        return sdkConfig.WithToken(config.ServiceAccountToken), nil
    }

    if config.ServiceAccountKeyPath != "" {
        return sdkConfig.WithServiceAccountKeyPath(config.ServiceAccountKeyPath), nil
    }

    if config.CLIAuth {
        adapter := &CLIAuthAdapter{}

        if !adapter.IsAuthenticated() {
            return nil, fmt.Errorf("CLI authentication enabled but not authenticated. Run: stackit auth provider login")
        }

        return sdkConfig.WithCLIProviderAuth(adapter), nil
    }

    // Default: let SDK handle env vars
    return nil, nil
}
```

### 3.4 Add Schema Field

In your provider schema:

```go
"cli_auth": {
    Type:        schema.TypeBool,
    Optional:    true,
    Default:     false,
    Description: "Use STACKIT CLI provider authentication",
},
```

### 3.5 Build the Provider

```bash
cd ~/projects/terraform-provider-stackit

# Build provider
go build

# Or install for Terraform to use
go install
```

### 3.6 Test Provider Build

```bash
# Verify no import cycles
go build ./...

# Run provider unit tests
go test ./internal/provider/...

# Build and check version
./terraform-provider-stackit version
```

## Part 4: End-to-End Integration Test

### 4.1 Setup Test Terraform Configuration

```bash
# Create test directory
mkdir -p ~/test-cli-auth
cd ~/test-cli-auth

# Create terraform config
cat > main.tf << 'EOF'
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

# Simple test resource
resource "stackit_project" "test" {
  name = "cli-auth-test"
}

output "project_id" {
  value = stackit_project.test.id
}
EOF
```

### 4.2 Configure Terraform to Use Local Provider

Option A - Dev overrides (recommended):

```bash
cat > ~/.terraformrc << 'EOF'
provider_installation {
  dev_overrides {
    "stackitcloud/stackit" = "/path/to/your/terraform-provider-stackit"
  }

  direct {}
}
EOF
```

Option B - Local provider directory:

```bash
# Copy provider binary to local plugins directory
mkdir -p ~/.terraform.d/plugins/stackitcloud/stackit/99.0.0/$(go env GOOS)_$(go env GOARCH)/
cp ~/projects/terraform-provider-stackit/terraform-provider-stackit \
   ~/.terraform.d/plugins/stackitcloud/stackit/99.0.0/$(go env GOOS)_$(go env GOARCH)/
```

### 4.3 Run End-to-End Test

```bash
cd ~/test-cli-auth

# Step 1: Authenticate with CLI
~/projects/stackit-cli/stackit auth provider login
# Authenticate in browser when prompted

# Step 2: Verify CLI authentication
~/projects/stackit-cli/stackit auth provider status
# Should show: "Authenticated"

# Step 3: Initialize Terraform
terraform init

# Step 4: Run Terraform plan (uses CLI auth!)
terraform plan

# Expected: Should successfully authenticate and show plan
# Should NOT ask for service account credentials

# Step 5: Check Terraform debug logs
TF_LOG=DEBUG terraform plan 2>&1 | grep -i "auth\|token\|cli"
# Look for signs of CLI auth being used
```

### 4.4 Verify Token Refresh

```bash
# Get initial token
INITIAL_TOKEN=$(~/projects/stackit-cli/stackit auth provider get-access-token)
echo "Initial token (first 50 chars): ${INITIAL_TOKEN:0:50}"

# Run terraform (triggers token validation)
terraform plan

# Check if token was refreshed (should be same or new)
NEW_TOKEN=$(~/projects/stackit-cli/stackit auth provider get-access-token)
echo "New token (first 50 chars): ${NEW_TOKEN:0:50}"

# Tokens should be valid JWT format
echo $NEW_TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | jq .exp
# Should show expiration timestamp
```

## Part 5: Testing Different Scenarios

### 5.1 Test Not Authenticated

```bash
# Logout from CLI
~/projects/stackit-cli/stackit auth provider logout

# Try to run terraform
cd ~/test-cli-auth
terraform plan

# Expected: Should fail with clear error message
# "CLI authentication enabled but not authenticated"
```

### 5.2 Test Authentication Priority

```bash
# Re-authenticate
~/projects/stackit-cli/stackit auth provider login

# Test 1: CLI auth takes precedence over env vars
export STACKIT_SERVICE_ACCOUNT_TOKEN="fake-token"
terraform plan
# Should use CLI auth, NOT env var

# Test 2: Explicit token overrides CLI auth
cat > main.tf << 'EOF'
provider "stackit" {
  service_account_token = "explicit-token"
  cli_auth              = true  # Should be ignored
}
EOF

terraform plan
# Should try to use explicit token (will fail if invalid)
```

### 5.3 Test Token Expiration

```bash
# Manually expire token by editing storage
# WARNING: This is for testing only!

# Get token storage
TOKEN_FILE=~/.stackit/cli-provider-auth-storage.txt

if [ -f "$TOKEN_FILE" ]; then
    # Backup
    cp $TOKEN_FILE ${TOKEN_FILE}.backup

    # Decode, modify expiration to past, encode
    CONTENT=$(cat $TOKEN_FILE | base64 -d)
    echo "$CONTENT" | jq '.session_expires_at_unix = "0"' | base64 > $TOKEN_FILE

    # Try terraform - should trigger refresh
    terraform plan

    # Restore backup
    mv ${TOKEN_FILE}.backup $TOKEN_FILE
fi
```

## Part 6: Debugging

### 6.1 Enable Debug Logging

```bash
# CLI debug output
~/projects/stackit-cli/stackit --verbosity debug auth provider status

# SDK debug output (if provider supports it)
TF_LOG=DEBUG terraform plan 2>&1 | less

# Check token contents
~/projects/stackit-cli/stackit auth provider get-access-token | \
  cut -d. -f2 | base64 -d 2>/dev/null | jq .
```

### 6.2 Verify Storage Locations

```bash
# Check keyring (macOS)
security find-generic-password -s "stackit-cli-provider" -w 2>/dev/null || \
  echo "Not in keyring, checking file..."

# Check file storage
if [ -f ~/.stackit/cli-provider-auth-storage.txt ]; then
    echo "✓ Found file storage"
    cat ~/.stackit/cli-provider-auth-storage.txt | base64 -d | jq '
        {
            auth_flow: .auth_flow_type,
            user_email: .user_email,
            has_access_token: (.access_token != null),
            has_refresh_token: (.refresh_token != null),
            expires_at: .session_expires_at_unix
        }
    '
else
    echo "✗ No file storage found"
fi
```

### 6.3 Common Issues

**Issue: "Import cycle"**
```bash
# Check if SDK imports CLI
cd ~/projects/stackit-sdk-go
grep -r "stackit-cli" --include="*.go" core/config/
# Should ONLY be in documentation comments, not in import statements
```

**Issue: "Provider not found"**
```bash
# Verify provider installation
ls -la ~/.terraform.d/plugins/stackitcloud/stackit/

# Check Terraform can find it
terraform providers
```

**Issue: "Token refresh fails"**
```bash
# Check IDP endpoint is accessible
curl -s https://accounts.stackit.cloud/.well-known/openid-configuration | jq .token_endpoint

# Verify refresh token exists
~/projects/stackit-cli/stackit auth provider get-access-token
```

## Summary Checklist

- [ ] CLI builds and tests pass
- [ ] CLI provider login works (browser auth)
- [ ] CLI stores credentials (keyring or file)
- [ ] SDK builds with no import cycle
- [ ] SDK unit tests pass (8/8)
- [ ] Provider builds successfully
- [ ] Provider has adapter implementation
- [ ] Provider has cli_auth schema field
- [ ] Terraform init succeeds
- [ ] Terraform plan uses CLI credentials
- [ ] Token refresh works automatically
- [ ] Error messages are clear
- [ ] Authentication priority works correctly

## Quick Test Script

Save this as `test-full-flow.sh`:

```bash
#!/bin/bash
set -e

echo "=== Full Integration Test ==="
echo ""

# 1. Test CLI
echo "1. Testing CLI..."
cd ~/projects/stackit-cli
go test ./pkg/auth -run TestIsProviderAuthenticated -v
echo "✓ CLI tests passed"
echo ""

# 2. Test SDK
echo "2. Testing SDK..."
cd ~/projects/stackit-sdk-go
go test ./core/config -run TestWithCLIProviderAuth -v
echo "✓ SDK tests passed"
echo ""

# 3. Check CLI auth status
echo "3. Checking CLI authentication..."
if ~/projects/stackit-cli/stackit auth provider status | grep -q "Authenticated"; then
    echo "✓ CLI authenticated"
else
    echo "✗ CLI not authenticated - run: stackit auth provider login"
    exit 1
fi
echo ""

# 4. Build provider
echo "4. Building provider..."
cd ~/projects/terraform-provider-stackit
go build
echo "✓ Provider built"
echo ""

# 5. Test terraform
echo "5. Testing Terraform integration..."
cd ~/test-cli-auth
terraform plan -no-color | head -20
echo "✓ Terraform plan succeeded"
echo ""

echo "=== ✅ All tests passed! ==="
```

Run it:
```bash
chmod +x test-full-flow.sh
./test-full-flow.sh
```

This comprehensive guide covers building, unit testing, and full end-to-end integration testing!

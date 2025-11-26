package config

import (
	"fmt"
	"net/http"
)

// CLIAuthProvider is an interface for external CLI authentication providers.
// This interface allows the SDK to use CLI-stored credentials without creating
// a circular dependency between the SDK and CLI packages.
//
// Implementations should provide access to credentials stored by authentication
// tools like the STACKIT CLI's provider authentication (`stackit auth provider login`).
//
// Example implementation (in Terraform Provider or other CLI consumer):
//
//	import (
//	    cliAuth "github.com/stackitcloud/stackit-cli/pkg/auth"
//	    sdkConfig "github.com/stackitcloud/stackit-sdk-go/core/config"
//	)
//
//	type CLIAuthAdapter struct{}
//
//	func (a *CLIAuthAdapter) IsAuthenticated() bool {
//	    return cliAuth.IsProviderAuthenticated()
//	}
//
//	func (a *CLIAuthAdapter) GetAuthFlow() (http.RoundTripper, error) {
//	    return cliAuth.ProviderAuthFlow(nil)
//	}
//
//	// Usage:
//	client, err := dns.NewAPIClient(
//	    sdkConfig.WithCLIProviderAuth(&CLIAuthAdapter{}),
//	)
type CLIAuthProvider interface {
	// IsAuthenticated checks if CLI provider credentials are available.
	// Returns true if credentials exist and can be used for authentication.
	IsAuthenticated() bool

	// GetAuthFlow returns an http.RoundTripper configured with CLI authentication.
	// The RoundTripper handles token refresh and authentication headers automatically.
	// Returns an error if credentials cannot be loaded or initialized.
	GetAuthFlow() (http.RoundTripper, error)
}

// WithCLIProviderAuth returns a ConfigurationOption that configures authentication
// using a CLI authentication provider.
//
// This option enables the SDK to use credentials stored by external CLI tools
// (like the STACKIT CLI) without creating a direct dependency on those tools.
// Instead, the caller provides an implementation of CLIAuthProvider that bridges
// between the SDK and the CLI.
//
// The authentication flow:
//   - Checks if CLI credentials are available via provider.IsAuthenticated()
//   - Retrieves an authentication RoundTripper via provider.GetAuthFlow()
//   - Configures the SDK client to use this RoundTripper
//   - Tokens are automatically refreshed by the RoundTripper
//   - Refreshed tokens are written back to CLI storage (bidirectional sync)
//
// Returns an AuthenticationError if no CLI credentials are found or if
// initialization fails.
//
// Example usage in Terraform Provider:
//
//	import (
//	    cliAuth "github.com/stackitcloud/stackit-cli/pkg/auth"
//	    sdkConfig "github.com/stackitcloud/stackit-sdk-go/core/config"
//	)
//
//	// Create adapter
//	type adapter struct{}
//	func (a *adapter) IsAuthenticated() bool {
//	    return cliAuth.IsProviderAuthenticated()
//	}
//	func (a *adapter) GetAuthFlow() (http.RoundTripper, error) {
//	    return cliAuth.ProviderAuthFlow(nil)
//	}
//
//	// Check authentication
//	adapter := &adapter{}
//	if !adapter.IsAuthenticated() {
//	    return fmt.Errorf("not authenticated: please run 'stackit auth provider login'")
//	}
//
//	// Create API client with CLI auth
//	client, err := dns.NewAPIClient(sdkConfig.WithCLIProviderAuth(adapter))
//	if err != nil {
//	    return fmt.Errorf("failed to create client: %w", err)
//	}
func WithCLIProviderAuth(provider CLIAuthProvider) ConfigurationOption {
	return func(c *Configuration) error {
		if provider == nil {
			return &AuthenticationError{
				msg: "CLI auth provider cannot be nil",
			}
		}

		// Check if CLI credentials are available
		if !provider.IsAuthenticated() {
			return &AuthenticationError{
				msg: "not authenticated with CLI provider credentials: please run authentication command (e.g., 'stackit auth provider login')",
			}
		}

		// Get the authentication RoundTripper from CLI
		authFlow, err := provider.GetAuthFlow()
		if err != nil {
			return &AuthenticationError{
				msg:   "failed to initialize CLI provider authentication",
				cause: err,
			}
		}

		// Configure the SDK to use CLI authentication
		return WithCustomAuth(authFlow)(c)
	}
}

// AuthenticationError indicates that CLI provider authentication failed.
// This error is returned when credentials are not found or cannot be initialized.
type AuthenticationError struct {
	msg   string
	cause error
}

// Error implements the error interface.
func (e *AuthenticationError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.msg, e.cause)
	}
	return e.msg
}

// Unwrap returns the underlying cause of the authentication error, if any.
// This allows errors.Is and errors.As to work with wrapped errors.
func (e *AuthenticationError) Unwrap() error {
	return e.cause
}

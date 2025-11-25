package config

import (
	"fmt"

	cliAuth "github.com/stackitcloud/stackit-cli/pkg/auth"
)

// IsCLIProviderAuthenticated checks if the STACKIT CLI provider credentials are available.
// Returns true if the user has run `stackit auth provider login`.
//
// Example:
//
//	if !config.IsCLIProviderAuthenticated() {
//	    return fmt.Errorf("not authenticated: please run 'stackit auth provider login'")
//	}
func IsCLIProviderAuthenticated() bool {
	return cliAuth.IsProviderAuthenticated()
}

// WithCLIProviderAuth returns a ConfigurationOption that configures authentication
// using STACKIT CLI provider credentials.
//
// This option enables the SDK to use credentials stored by the STACKIT CLI via
// `stackit auth provider login`, eliminating the need for service accounts during
// local development.
//
// The authentication flow:
//   - Checks if CLI provider credentials exist
//   - Retrieves the credentials and configures automatic token refresh
//   - Tokens are automatically refreshed before expiration
//   - Refreshed tokens are written back to CLI storage (bidirectional sync)
//
// Returns an AuthenticationError if no CLI provider credentials are found.
// Users should run `stackit auth provider login` to authenticate.
//
// Example:
//
//	import (
//	    "github.com/stackitcloud/stackit-sdk-go/core/config"
//	    "github.com/stackitcloud/stackit-sdk-go/services/dns"
//	)
//
//	// Check if authenticated
//	if !config.IsCLIProviderAuthenticated() {
//	    return fmt.Errorf("please run: stackit auth provider login")
//	}
//
//	// Create API client with CLI auth
//	client, err := dns.NewAPIClient(config.WithCLIProviderAuth())
//	if err != nil {
//	    return fmt.Errorf("failed to create client: %w", err)
//	}
//
// Note: This function does not require external dependencies. The CLI's internal
// printer is managed automatically for token refresh operations.
func WithCLIProviderAuth() ConfigurationOption {
	return func(c *Configuration) error {
		// Check if CLI provider credentials exist
		if !cliAuth.IsProviderAuthenticated() {
			return &AuthenticationError{
				msg: "not authenticated with STACKIT CLI provider credentials: please run 'stackit auth provider login'",
			}
		}

		// Get the authentication RoundTripper from CLI
		// We pass nil for the printer since we don't have access to it from the SDK
		// The CLI handles nil printers gracefully by skipping debug output
		authFlow, err := cliAuth.ProviderAuthFlow(nil)
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
func (e *AuthenticationError) Unwrap() error {
	return e.cause
}

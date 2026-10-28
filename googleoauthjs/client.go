package googleoauthjs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"golang.org/x/oauth2"
)

// TokenClient is a client for the Google OAuth2 API that provides a token back to the user.
type TokenClient struct {
	ctx            context.Context    // This is the context to use for background operations.
	client         app.Value          // This is the JavaScript Google OAuth2 client.
	callback       func(oauth2.Token) // This is the callback to use when a token is received.
	errorCallback  func(error)        // This is the callback to use when an error occurs.
	connectChannel chan error         // This is the channel to use when a connection is established.
	currentToken   oauth2.Token       // This is the current token.
}

// TokenClientConfig is a configuration for the TokenClient.
type TokenClientConfig struct {
	ClientID             string             // This is the client ID to use for the OAuth2 client.
	Callback             func(oauth2.Token) // This is the callback to use when a token is received.
	ErrorCallback        func(error)        // This is the callback to use when an error occurs.
	Scopes               []string           // This is the scopes to use for the OAuth2 client.
	IncludeGrantedScopes *bool              // This is the include granted scopes to use for the OAuth2 client.
	Prompt               *string            // This is the prompt to use for the OAuth2 client.
	LoginHint            *string            // This is the login hint to use for the OAuth2 client.
	HD                   *string            // This is the HD to use for the OAuth2 client.
}

const (
	PromptEmptyString   = ""
	PromptNone          = "none"
	PromptConsent       = "consent"
	PromptSelectAccount = "select_account"
)

// TokenClientOption is an option for the TokenClient.
type TokenClientOption func(*TokenClientConfig)

// WithConfig sets the configuration for the TokenClient.
func WithConfig(newConfig *TokenClientConfig) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		*cfg = *newConfig
	}
}

// WithClientID sets the client ID for the TokenClient.
func WithClientID(clientID string) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.ClientID = clientID
	}
}

// WithCallback sets the callback for the TokenClient.
func WithCallback(callback func(token oauth2.Token)) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.Callback = callback
	}
}

// WithErrorCallback sets the error callback for the TokenClient.
func WithErrorCallback(errorCallback func(error)) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.ErrorCallback = errorCallback
	}
}

// WithScopes sets the scopes for the TokenClient.
func WithScopes(scopes []string) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.Scopes = scopes
	}
}

// WithIncludeGrantedScopes sets the include granted scopes for the TokenClient.
func WithIncludeGrantedScopes(includeGrantedScopes bool) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.IncludeGrantedScopes = &includeGrantedScopes
	}
}

// WithPrompt sets the prompt for the TokenClient.
func WithPrompt(prompt string) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.Prompt = &prompt
	}
}

// WithLoginHint sets the login hint for the TokenClient.
func WithLoginHint(loginHint string) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.LoginHint = &loginHint
	}
}

// WithHD sets the HD for the TokenClient.
func WithHD(hd string) TokenClientOption {
	return func(cfg *TokenClientConfig) {
		cfg.HD = &hd
	}
}

// TokenClientError is an error that occurs when a token is not received.
type TokenClientError struct {
	Message string
	Stack   string
	Type    string
}

// Error returns the error message.
func (e *TokenClientError) Error() string {
	return e.Type + ": " + e.Message
}

// NewTokenClient creates a new TokenClient.
func NewTokenClient(ctx context.Context, opts ...TokenClientOption) *TokenClient {
	cfg := &TokenClientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	c := &TokenClient{
		ctx:           ctx,
		callback:      cfg.Callback,
		errorCallback: cfg.ErrorCallback,
	}

	jsConfig := app.ValueOf(map[string]any{})
	jsConfig.Set("client_id", cfg.ClientID)
	jsConfig.Set("scope", strings.Join(cfg.Scopes, " "))
	jsConfig.Set("callback", app.FuncOf(c.jsCallback))
	jsConfig.Set("error_callback", app.FuncOf(c.jsErrorCallback))
	if cfg.IncludeGrantedScopes != nil {
		jsConfig.Set("include_granted_scopes", *cfg.IncludeGrantedScopes)
	}
	if cfg.Prompt != nil {
		jsConfig.Set("prompt", *cfg.Prompt)
	}
	if cfg.LoginHint != nil {
		jsConfig.Set("login_hint", *cfg.LoginHint)
	}
	if cfg.HD != nil {
		jsConfig.Set("hd", *cfg.HD)
	}

	c.client = app.Window().Get("google").Get("accounts").Get("oauth2").Call("initTokenClient", jsConfig)

	return c
}

// jsCallback is the internal JavaScript callback for the TokenClient.
// This is called when a token is received.
//
// This will call the user-specified callback with the token.
func (c *TokenClient) jsCallback(this app.Value, args []app.Value) any {
	slog.InfoContext(c.ctx, "TokenClient: jsCallback", "args", len(args))
	if len(args) == 0 {
		if c.connectChannel != nil {
			c.connectChannel <- fmt.Errorf("no arguments received")
		}
		return nil
	}

	info := args[0]
	slog.InfoContext(c.ctx, "TokenClient: jsCallback: info", "JSON", app.Window().Get("JSON").Call("stringify", info))

	token := oauth2.Token{
		AccessToken: info.Get("access_token").String(),
		ExpiresIn:   int64(info.Get("expires_in").Int()),
		Expiry:      time.Now().Add(time.Duration(info.Get("expires_in").Int()) * time.Second),
		TokenType:   info.Get("token_type").String(),
	}
	c.currentToken = token

	if c.callback != nil {
		c.callback(token)
	}

	if c.connectChannel != nil {
		c.connectChannel <- nil
	}

	return nil
}

// jsErrorCallback is the internal JavaScript error callback for the TokenClient.
// This is called when an error occurs.
//
// This will call the user-specified error callback with the error.
func (c *TokenClient) jsErrorCallback(this app.Value, args []app.Value) any {
	slog.ErrorContext(c.ctx, "TokenClient: jsErrorCallback", "args", len(args))
	if len(args) == 0 {
		return nil
	}

	info := args[0]
	slog.ErrorContext(c.ctx, "TokenClient: jsErrorCallback: info", "JSON", app.Window().Get("JSON").Call("stringify", info))

	e := &TokenClientError{
		Message: info.Get("message").String(),
		Stack:   info.Get("stack").String(),
		Type:    info.Get("type").String(),
	}

	if c.errorCallback != nil {
		c.errorCallback(e)
	}

	if c.connectChannel != nil {
		c.connectChannel <- e
	}

	return nil
}

// Token returns the current token.
func (c *TokenClient) Token() oauth2.Token {
	return c.currentToken
}

// OverridableTokenClientConfig is a configuration for the OverridableTokenClient.
type OverridableTokenClientConfig struct {
	Scopes               []string // This is the scopes to use for the OAuth2 client.
	IncludeGrantedScopes *bool    // This is the include granted scopes to use for the OAuth2 client.
	Prompt               *string  // This is the prompt to use for the OAuth2 client.
	LoginHint            *string  // This is the login hint to use for the OAuth2 client.
}

// OverridableTokenClientConfigOption is an option for the OverridableTokenClientConfig.
type OverridableTokenClientConfigOption func(*OverridableTokenClientConfig)

// WithOverridableScopes sets the scopes for the OverridableTokenClient.
func WithOverridableScopes(scopes []string) OverridableTokenClientConfigOption {
	return func(cfg *OverridableTokenClientConfig) {
		cfg.Scopes = scopes
	}
}

// WithOverridableIncludeGrantedScopes sets the include granted scopes for the OverridableTokenClient.
func WithOverridableIncludeGrantedScopes(includeGrantedScopes bool) OverridableTokenClientConfigOption {
	return func(cfg *OverridableTokenClientConfig) {
		cfg.IncludeGrantedScopes = &includeGrantedScopes
	}
}

// WithOverridablePrompt sets the prompt for the OverridableTokenClient.
func WithOverridablePrompt(prompt string) OverridableTokenClientConfigOption {
	return func(cfg *OverridableTokenClientConfig) {
		cfg.Prompt = &prompt
	}
}

// WithOverridableLoginHint sets the login hint for the OverridableTokenClient.
func WithOverridableLoginHint(loginHint string) OverridableTokenClientConfigOption {
	return func(cfg *OverridableTokenClientConfig) {
		cfg.LoginHint = &loginHint
	}
}

// RequestAccessToken requests an access token from the Google OAuth2 API.
// Any additional overrides may be specified, but it will work without any.
func (c *TokenClient) RequestAccessToken(opts ...OverridableTokenClientConfigOption) error {
	cfg := &OverridableTokenClientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	jsConfig := app.ValueOf(map[string]any{})
	if len(cfg.Scopes) > 0 {
		jsConfig.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	if cfg.IncludeGrantedScopes != nil {
		jsConfig.Set("include_granted_scopes", *cfg.IncludeGrantedScopes)
	}
	if cfg.Prompt != nil {
		jsConfig.Set("prompt", *cfg.Prompt)
	}
	if cfg.LoginHint != nil {
		jsConfig.Set("login_hint", *cfg.LoginHint)
	}

	c.connectChannel = make(chan error, 1) // This is the channel to use when a connection is established.

	c.client.Call("requestAccessToken", jsConfig)

	err := <-c.connectChannel
	close(c.connectChannel)
	c.connectChannel = nil
	if err != nil {
		return fmt.Errorf("failed to request access token: %w", err)
	}

	return nil
}

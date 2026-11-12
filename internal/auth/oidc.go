package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCClient struct {
	issuerURL     string
	clientID      string
	adminGroup    string
	usernameClaim string
	provider      *oidc.Provider
	verifier      *oidc.IDTokenVerifier
	oauthConfig   oauth2.Config
	devMode       bool
}

type UserClaims struct {
	Username string
	Groups   []string
}

func NewOIDCClient(ctx context.Context, issuerURL, clientID, clientSecret, redirectURL string, scopes []string, adminGroup, usernameClaim string, devMode bool) (*OIDCClient, error) {
	client := &OIDCClient{
		issuerURL:     issuerURL,
		clientID:      clientID,
		adminGroup:    adminGroup,
		usernameClaim: usernameClaim,
		devMode:       devMode,
	}

	if devMode && issuerURL == "" {
		return client, nil
	}

	if issuerURL == "" {
		return nil, errors.New("OIDC_ISSUER_URL is required")
	}

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC provider %s: %w", issuerURL, err)
	}

	client.provider = provider
	client.verifier = provider.Verifier(&oidc.Config{ClientID: clientID})
	client.oauthConfig = oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}

	return client, nil
}

func (o *OIDCClient) AuthCodeURL(state string) string {
	if o.devMode && o.provider == nil {
		return "/auth/callback?code=dev-code&state=" + state
	}
	return o.oauthConfig.AuthCodeURL(state)
}

func (o *OIDCClient) ExchangeAndVerify(ctx context.Context, code string) (*UserClaims, error) {
	if o.devMode && o.provider == nil {
		return &UserClaims{
			Username: "devuser",
			Groups:   []string{o.adminGroup},
		}, nil
	}

	token, err := o.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("no id_token in token response")
	}

	idToken, err := o.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Extract username from specified claim, with fallbacks
	username := ""
	if val, ok := claims[o.usernameClaim].(string); ok && val != "" {
		username = val
	} else if val, ok := claims["preferred_username"].(string); ok && val != "" {
		username = val
	} else if val, ok := claims["sub"].(string); ok && val != "" {
		username = val
	}

	if username == "" {
		return nil, errors.New("could not find valid username in OIDC claims")
	}

	// Extract groups
	var groups []string
	if rawGroups, ok := claims["groups"].([]interface{}); ok {
		for _, g := range rawGroups {
			if s, ok := g.(string); ok {
				groups = append(groups, s)
			}
		}
	}

	return &UserClaims{
		Username: username,
		Groups:   groups,
	}, nil
}

func GenerateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (o *OIDCClient) IsAdmin(groups []string) bool {
	for _, g := range groups {
		if g == o.adminGroup {
			return true
		}
	}
	return false
}

package warden

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/t0ggl/andvari/internal/store"
)

type Client struct {
	socketPath string
	client     *http.Client
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: false,
		MaxIdleConns:      10,
		IdleConnTimeout:   30 * time.Second,
	}

	return &Client{
		socketPath: socketPath,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (c *Client) SocketPath() string {
	return c.socketPath
}

func (c *Client) post(path string, reqPayload interface{}, respPayload interface{}) (int, error) {
	var bodyReader io.Reader
	if reqPayload != nil {
		buf, err := json.Marshal(reqPayload)
		if err != nil {
			return 0, fmt.Errorf("marshal request error: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(http.MethodPost, "http://unix"+path, bodyReader)
	if err != nil {
		return 0, fmt.Errorf("create request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read response error: %w", err)
	}

	if respPayload != nil && len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, respPayload); err != nil {
			return resp.StatusCode, fmt.Errorf("unmarshal error (status %d): %w (body: %s)", resp.StatusCode, err, string(bodyBytes))
		}
	}

	return resp.StatusCode, nil
}

func (c *Client) AuthenticateWebDAV(username, token string) (*WebDAVAuthResponse, error) {
	var resp WebDAVAuthResponse
	code, err := c.post("/warden/auth/webdav", WebDAVAuthRequest{Username: username, Token: token}, &resp)
	if err != nil {
		return nil, err
	}
	if code == http.StatusUnauthorized || !resp.Authenticated {
		return &WebDAVAuthResponse{Authenticated: false, Error: resp.Error}, nil
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return &resp, nil
}

func (c *Client) AuthenticateSession(sessionCookie string) (*SessionAuthResponse, error) {
	var resp SessionAuthResponse
	code, err := c.post("/warden/auth/session", SessionAuthRequest{SessionCookie: sessionCookie}, &resp)
	if err != nil {
		return nil, err
	}
	if code == http.StatusUnauthorized || !resp.Authenticated {
		return nil, errors.New("unauthenticated")
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return &resp, nil
}

func (c *Client) CreateToken(sessionCookie, label string) (*CreateTokenResponse, error) {
	var resp CreateTokenResponse
	code, err := c.post("/warden/tokens/create", CreateTokenRequest{SessionCookie: sessionCookie, Label: label}, &resp)
	if err != nil {
		return nil, err
	}
	if code == http.StatusUnauthorized {
		return nil, errors.New("unauthenticated")
	}
	if code == http.StatusBadRequest && resp.Error != "" {
		return &resp, nil
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return &resp, nil
}

func (c *Client) RevokeToken(sessionCookie, tokenID string) error {
	var resp GenericResponse
	code, err := c.post("/warden/tokens/revoke", RevokeTokenRequest{SessionCookie: sessionCookie, TokenID: tokenID}, &resp)
	if err != nil {
		return err
	}
	if code == http.StatusUnauthorized {
		return errors.New("unauthenticated")
	}
	if code == http.StatusNotFound {
		return store.ErrTokenNotFound
	}
	if code != http.StatusOK {
		return fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return nil
}

func (c *Client) RenameToken(sessionCookie, tokenID, newLabel string) (*TokenResponse, error) {
	var resp struct {
		Success   bool          `json:"success"`
		TokenInfo TokenResponse `json:"token_info"`
		Error     string        `json:"error"`
	}
	code, err := c.post("/warden/tokens/rename", RenameTokenRequest{SessionCookie: sessionCookie, TokenID: tokenID, Label: newLabel}, &resp)
	if err != nil {
		return nil, err
	}
	if code == http.StatusUnauthorized {
		return nil, errors.New("unauthenticated")
	}
	if code == http.StatusNotFound {
		return nil, store.ErrTokenNotFound
	}
	if code == http.StatusBadRequest {
		return nil, errors.New(resp.Error)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return &resp.TokenInfo, nil
}

func (c *Client) GetUserInfo(sessionCookie string) (*UserInfoResponse, error) {
	var resp UserInfoResponse
	code, err := c.post("/warden/me", SessionAuthRequest{SessionCookie: sessionCookie}, &resp)
	if err != nil {
		return nil, err
	}
	if code == http.StatusUnauthorized {
		return nil, errors.New("unauthenticated")
	}
	if code == http.StatusNotFound {
		return nil, errors.New("user not found")
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return &resp, nil
}

func (c *Client) CompleteOnboarding(sessionCookie string) error {
	var resp GenericResponse
	code, err := c.post("/warden/onboarding/complete", OnboardingCompleteRequest{SessionCookie: sessionCookie}, &resp)
	if err != nil {
		return err
	}
	if code == http.StatusUnauthorized {
		return errors.New("unauthenticated")
	}
	if code != http.StatusOK {
		return fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return nil
}

func (c *Client) AdminListUsers(sessionCookie string) ([]string, error) {
	var resp AdminListUsersResponse
	code, err := c.post("/warden/admin/users", SessionAuthRequest{SessionCookie: sessionCookie}, &resp)
	if err != nil {
		return nil, err
	}
	if code == http.StatusUnauthorized {
		return nil, errors.New("unauthenticated")
	}
	if code == http.StatusForbidden {
		return nil, errors.New("forbidden")
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return resp.Users, nil
}

func (c *Client) AdminImpersonate(sessionCookie, targetUser string) (*AdminImpersonateResponse, error) {
	var resp AdminImpersonateResponse
	code, err := c.post("/warden/admin/impersonate", AdminImpersonateRequest{SessionCookie: sessionCookie, TargetUser: targetUser}, &resp)
	if err != nil {
		return nil, err
	}
	if code == http.StatusUnauthorized {
		return nil, errors.New("unauthenticated")
	}
	if code == http.StatusForbidden {
		return nil, errors.New("forbidden")
	}
	if code == http.StatusNotFound {
		return nil, errors.New("target user not found")
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return &resp, nil
}

func (c *Client) AdminStopImpersonate(sessionCookie string) (*AdminImpersonateResponse, error) {
	return c.AdminImpersonate(sessionCookie, "")
}

func (c *Client) AdminSetQuota(sessionCookie, targetUser string, quotaBytes *int64) error {
	var resp GenericResponse
	code, err := c.post("/warden/admin/quota", AdminSetQuotaRequest{SessionCookie: sessionCookie, TargetUser: targetUser, QuotaBytes: quotaBytes}, &resp)
	if err != nil {
		return err
	}
	if code == http.StatusUnauthorized {
		return errors.New("unauthenticated")
	}
	if code == http.StatusForbidden {
		return errors.New("forbidden")
	}
	if code == http.StatusNotFound {
		return store.ErrUserNotFound
	}
	if code != http.StatusOK {
		return fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return nil
}

func (c *Client) CreateSession(username string, groups []string, flashToken string) (string, error) {
	var resp CreateSessionResponse
	code, err := c.post("/warden/session/login", CreateSessionRequest{Username: username, Groups: groups, FlashToken: flashToken}, &resp)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return resp.SessionCookie, nil
}

func (c *Client) AddUsageDelta(username string, delta int64) error {
	var resp GenericResponse
	code, err := c.post("/warden/usage/delta", UsageDeltaRequest{Username: username, Delta: delta}, &resp)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("warden returned code %d: %s", code, resp.Error)
	}
	return nil
}

func (c *Client) CheckHealth() error {
	req, err := http.NewRequest(http.MethodGet, "http://unix/warden/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("warden health check failed (code %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

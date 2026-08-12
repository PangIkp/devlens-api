package githubapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
)

const (
	defaultPerPage = 30
)

type Installation struct {
	ID                  int64
	AccountLogin        string
	AccountType         string
	TargetType          string
	Permissions         map[string]string
	SuspendedAt         *time.Time
	InstalledByGithubID int64
}

type AccessibleRepository struct {
	GithubRepositoryID int64
	Name               string
	OwnerLogin         string
	FullName           string
	Private            bool
	DefaultBranch      *string
}

type Token struct {
	Value     string
	ExpiresAt time.Time
}

type RepositoryPage struct {
	Items      []AccessibleRepository
	TotalCount int
	NextPage   int
}

type Client interface {
	Enabled() bool
	InstallURL(state string, redirectURL string) (string, error)
	GetInstallation(ctx context.Context, installationID int64) (Installation, error)
	CreateInstallationToken(ctx context.Context, installationID int64) (Token, error)
	ListInstallationRepositories(ctx context.Context, installationID int64, page, perPage int) (RepositoryPage, error)
}

type HTTPClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
	appID      int64
	installURL string
	privateKey *rsa.PrivateKey
	tokenMu    sync.Mutex
	tokenCache map[int64]Token
}

type installationResponse struct {
	ID                  int64             `json:"id"`
	Account             installationActor `json:"account"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
	SuspendedAt         *time.Time        `json:"suspended_at"`
	Sender              installationActor `json:"sender"`
}

type installationActor struct {
	Login string `json:"login"`
	Type  string `json:"type"`
	ID    int64  `json:"id"`
}

type accessTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type listRepositoriesResponse struct {
	TotalCount   int                 `json:"total_count"`
	Repositories []repositoryPayload `json:"repositories"`
}

type repositoryPayload struct {
	ID            int64             `json:"id"`
	Name          string            `json:"name"`
	FullName      string            `json:"full_name"`
	Private       bool              `json:"private"`
	DefaultBranch string            `json:"default_branch"`
	Owner         installationActor `json:"owner"`
}

func New(cfg config.GitHubConfig) (*HTTPClient, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse github base url: %w", err)
	}

	client := &HTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
		userAgent:  cfg.UserAgent,
		appID:      cfg.App.AppID,
		installURL: strings.TrimSpace(cfg.App.InstallURL),
		tokenCache: make(map[int64]Token),
	}

	privateKey := strings.TrimSpace(cfg.App.PrivateKey)
	if privateKey != "" {
		key, err := parsePrivateKey(privateKey)
		if err != nil {
			return nil, err
		}
		client.privateKey = key
	}

	return client, nil
}

func (c *HTTPClient) Enabled() bool {
	return c != nil && c.appID > 0 && c.privateKey != nil && strings.TrimSpace(c.installURL) != ""
}

func (c *HTTPClient) InstallURL(state string, redirectURL string) (string, error) {
	if !c.Enabled() {
		return "", errors.New("github app is not configured")
	}

	target, err := url.Parse(c.installURL)
	if err != nil {
		return "", fmt.Errorf("parse install url: %w", err)
	}

	query := target.Query()
	if strings.TrimSpace(state) != "" {
		query.Set("state", strings.TrimSpace(state))
	}
	if strings.TrimSpace(redirectURL) != "" {
		query.Set("redirect_url", strings.TrimSpace(redirectURL))
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (c *HTTPClient) GetInstallation(ctx context.Context, installationID int64) (Installation, error) {
	var payload installationResponse
	if err := c.doAppRequest(ctx, http.MethodGet, fmt.Sprintf("/app/installations/%d", installationID), nil, &payload); err != nil {
		return Installation{}, err
	}

	return Installation{
		ID:                  payload.ID,
		AccountLogin:        payload.Account.Login,
		AccountType:         payload.Account.Type,
		TargetType:          payload.RepositorySelection,
		Permissions:         payload.Permissions,
		SuspendedAt:         payload.SuspendedAt,
		InstalledByGithubID: payload.Sender.ID,
	}, nil
}

func (c *HTTPClient) CreateInstallationToken(ctx context.Context, installationID int64) (Token, error) {
	if token, ok := c.cachedInstallationToken(installationID); ok {
		return token, nil
	}

	var payload accessTokenResponse
	if err := c.doAppRequest(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", installationID), strings.NewReader("{}"), &payload); err != nil {
		return Token{}, err
	}

	token := Token{
		Value:     strings.TrimSpace(payload.Token),
		ExpiresAt: payload.ExpiresAt.UTC(),
	}
	c.storeInstallationToken(installationID, token)
	return token, nil
}

func (c *HTTPClient) ListInstallationRepositories(ctx context.Context, installationID int64, page, perPage int) (RepositoryPage, error) {
	token, err := c.CreateInstallationToken(ctx, installationID)
	if err != nil {
		return RepositoryPage{}, err
	}

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = defaultPerPage
	}

	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("per_page", strconv.Itoa(perPage))

	var payload listRepositoriesResponse
	if err := c.doInstallationRequest(ctx, token.Value, http.MethodGet, "/installation/repositories", query, &payload); err != nil {
		return RepositoryPage{}, err
	}

	items := make([]AccessibleRepository, 0, len(payload.Repositories))
	for _, repo := range payload.Repositories {
		items = append(items, AccessibleRepository{
			GithubRepositoryID: repo.ID,
			Name:               repo.Name,
			OwnerLogin:         repo.Owner.Login,
			FullName:           repo.FullName,
			Private:            repo.Private,
			DefaultBranch:      optionalString(repo.DefaultBranch),
		})
	}

	nextPage := 0
	if len(items) == perPage {
		nextPage = page + 1
	}

	return RepositoryPage{
		Items:      items,
		TotalCount: payload.TotalCount,
		NextPage:   nextPage,
	}, nil
}

func (c *HTTPClient) doAppRequest(ctx context.Context, method, path string, body io.Reader, target any) error {
	jwtToken, err := c.createJWT()
	if err != nil {
		return err
	}

	req, err := c.newRequest(ctx, method, path, nil, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	return c.execute(req, target)
}

func (c *HTTPClient) doInstallationRequest(ctx context.Context, token, method, path string, query url.Values, target any) error {
	req, err := c.newRequest(ctx, method, path, query, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))

	return c.execute(req, target)
}

func (c *HTTPClient) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: path, RawQuery: query.Encode()})
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create github app request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.userAgent)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *HTTPClient) execute(req *http.Request, target any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("perform github app request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github app request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if target == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode github app response: %w", err)
	}
	return nil
}

func (c *HTTPClient) createJWT() (string, error) {
	if c.appID <= 0 || c.privateKey == nil {
		return "", errors.New("github app is not configured")
	}

	now := time.Now().UTC()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-30 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": c.appID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal github app jwt claims: %w", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(claims)
	signingInput := header + "." + payload
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign github app jwt: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("decode github app private key: invalid PEM")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse github app private key: %w", err)
	}

	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("parse github app private key: expected RSA private key")
	}
	return key, nil
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (c *HTTPClient) cachedInstallationToken(installationID int64) (Token, bool) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	token, ok := c.tokenCache[installationID]
	if !ok {
		return Token{}, false
	}
	if time.Until(token.ExpiresAt) <= 30*time.Second {
		delete(c.tokenCache, installationID)
		return Token{}, false
	}
	return token, true
}

func (c *HTTPClient) storeInstallationToken(installationID int64, token Token) {
	if strings.TrimSpace(token.Value) == "" || token.ExpiresAt.IsZero() {
		return
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.tokenCache[installationID] = token
}

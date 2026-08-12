package githubapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/githubclient"
)

type InstallationLookup interface {
	FindInstallationIDByRepositoryFullName(context.Context, string) (*int64, error)
}

type SyncClient struct {
	cfg      config.GitHubConfig
	app      Client
	lookup   InstallationLookup
	fallback githubclient.Client
}

func NewSyncClient(cfg config.GitHubConfig, app Client, lookup InstallationLookup, fallback githubclient.Client) *SyncClient {
	return &SyncClient{
		cfg:      cfg,
		app:      app,
		lookup:   lookup,
		fallback: fallback,
	}
}

func (c *SyncClient) GetRepository(ctx context.Context, owner, repo string) (githubclient.Repository, error) {
	client, err := c.clientForRepository(ctx, owner, repo)
	if err != nil {
		return githubclient.Repository{}, err
	}
	return client.GetRepository(ctx, owner, repo)
}

func (c *SyncClient) GetPullRequest(ctx context.Context, owner, repo string, pullNumber int) (githubclient.PullRequest, error) {
	client, err := c.clientForRepository(ctx, owner, repo)
	if err != nil {
		return githubclient.PullRequest{}, err
	}
	return client.GetPullRequest(ctx, owner, repo, pullNumber)
}

func (c *SyncClient) ListPullRequests(ctx context.Context, owner, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
	client, err := c.clientForRepository(ctx, owner, repo)
	if err != nil {
		return githubclient.Page[githubclient.PullRequest]{}, err
	}
	return client.ListPullRequests(ctx, owner, repo, options)
}

func (c *SyncClient) ListReviews(ctx context.Context, owner, repo string, pullNumber int, options githubclient.ListOptions) (githubclient.Page[githubclient.Review], error) {
	client, err := c.clientForRepository(ctx, owner, repo)
	if err != nil {
		return githubclient.Page[githubclient.Review]{}, err
	}
	return client.ListReviews(ctx, owner, repo, pullNumber, options)
}

func (c *SyncClient) ListCommits(ctx context.Context, owner, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error) {
	client, err := c.clientForRepository(ctx, owner, repo)
	if err != nil {
		return githubclient.Page[githubclient.Commit]{}, err
	}
	return client.ListCommits(ctx, owner, repo, options)
}

func (c *SyncClient) clientForRepository(ctx context.Context, owner, repo string) (githubclient.Client, error) {
	fullName := strings.TrimSpace(owner) + "/" + strings.TrimSpace(repo)
	if c.app != nil && c.app.Enabled() && c.lookup != nil {
		installationID, err := c.lookup.FindInstallationIDByRepositoryFullName(ctx, fullName)
		if err != nil {
			return nil, err
		}
		if installationID != nil {
			token, err := c.app.CreateInstallationToken(ctx, *installationID)
			if err != nil {
				return nil, err
			}
			client, err := githubclient.New(githubclient.Config{
				BaseURL:        c.cfg.BaseURL,
				UserAgent:      c.cfg.UserAgent,
				HTTPTimeout:    c.cfg.HTTPTimeout,
				MaxRetries:     c.cfg.MaxRetries,
				InitialBackoff: c.cfg.InitialBackoff,
				MaxBackoff:     c.cfg.MaxBackoff,
			}, githubclient.StaticTokenProvider{Value: token.Value})
			if err != nil {
				return nil, fmt.Errorf("create installation github client: %w", err)
			}
			return client, nil
		}
	}

	return c.fallback, nil
}

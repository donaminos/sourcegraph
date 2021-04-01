package sources

import (
	"context"
	"testing"

	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/extsvc"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

// func TestLoadChangesetSource(t *testing.T) {
// 	ctx := context.Background()
// 	sourcer := repos.NewSourcer(httpcli.NewFactory(
// 		func(cli httpcli.Doer) httpcli.Doer {
// 			return httpcli.DoerFunc(func(req *http.Request) (*http.Response, error) {
// 				// Don't actually execute the request, just dump the authorization header
// 				// in the error, so we can assert on it further down.
// 				return nil, errors.New(req.Header.Get("Authorization"))
// 			})
// 		},
// 		httpcli.NewTimeoutOpt(1*time.Second),
// 	))

// 	externalService := types.ExternalService{
// 		ID:          1,
// 		Kind:        extsvc.KindGitHub,
// 		DisplayName: "GitHub.com",
// 		Config:      `{"url": "https://github.com", "token": "123", "authorization": {}}`,
// 	}
// 	repo := &types.Repo{
// 		Name:    api.RepoName("test-repo"),
// 		URI:     "test-repo",
// 		Private: true,
// 		ExternalRepo: api.ExternalRepoSpec{
// 			ID:          "external-id-123",
// 			ServiceType: extsvc.TypeGitHub,
// 			ServiceID:   "https://github.com/",
// 		},
// 		Sources: map[string]*types.SourceInfo{
// 			externalService.URN(): {
// 				ID:       externalService.URN(),
// 				CloneURL: "https://123@github.com/sourcegraph/sourcegraph",
// 			},
// 		},
// 	}

// 	// Store mocks.
// 	database.Mocks.ExternalServices.List = func(opt database.ExternalServicesListOptions) ([]*types.ExternalService, error) {
// 		return []*types.ExternalService{&externalService}, nil
// 	}
// 	t.Cleanup(func() {
// 		database.Mocks.ExternalServices.List = nil
// 	})
// 	hasCredential := false
// 	syncStore := &MockSyncStore{
// 		getSiteCredential: func(ctx context.Context, opts store.GetSiteCredentialOpts) (*store.SiteCredential, error) {
// 			if hasCredential {
// 				return &store.SiteCredential{Credential: &auth.OAuthBearerToken{Token: "456"}}, nil
// 			}
// 			return nil, store.ErrNoResults
// 		},
// 	}

// 	// If no site-credential exists, the token from the external service should be used.
// 	src, err := loadChangesetSource(ctx, sourcer, syncStore, repo)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if err := src.(*repos.GithubSource).ValidateAuthenticator(ctx); err == nil {
// 		t.Fatal("unexpected nil error")
// 	} else if have, want := err.Error(), "Bearer 123"; have != want {
// 		t.Fatalf("invalid token used, want=%q have=%q", want, have)
// 	}

// 	// If one exists, prefer that one over the external service config ones.
// 	hasCredential = true
// 	src, err = loadChangesetSource(ctx, sourcer, syncStore, repo)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if err := src.(*repos.GithubSource).ValidateAuthenticator(ctx); err == nil {
// 		t.Fatal("unexpected nil error")
// 	} else if have, want := err.Error(), "Bearer 456"; have != want {
// 		t.Fatalf("invalid token used, want=%q have=%q", want, have)
// 	}
// }

func TestLoadExternalService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	noToken := types.ExternalService{
		ID:          1,
		Kind:        extsvc.KindGitHub,
		DisplayName: "GitHub no token",
		Config:      `{"url": "https://github.com", "authorization": {}}`,
	}
	userOwnedWithToken := types.ExternalService{
		ID:              2,
		Kind:            extsvc.KindGitHub,
		DisplayName:     "GitHub user owned",
		NamespaceUserID: 1234,
		Config:          `{"url": "https://github.com", "token": "123", "authorization": {}}`,
	}
	withToken := types.ExternalService{
		ID:          3,
		Kind:        extsvc.KindGitHub,
		DisplayName: "GitHub token",
		Config:      `{"url": "https://github.com", "token": "123", "authorization": {}}`,
	}
	withTokenNewer := types.ExternalService{
		ID:          4,
		Kind:        extsvc.KindGitHub,
		DisplayName: "GitHub newer token",
		Config:      `{"url": "https://github.com", "token": "123456", "authorization": {}}`,
	}

	repo := &types.Repo{
		Name:    api.RepoName("test-repo"),
		URI:     "test-repo",
		Private: true,
		ExternalRepo: api.ExternalRepoSpec{
			ID:          "external-id-123",
			ServiceType: extsvc.TypeGitHub,
			ServiceID:   "https://github.com/",
		},
		Sources: map[string]*types.SourceInfo{
			noToken.URN(): {
				ID:       noToken.URN(),
				CloneURL: "https://github.com/sourcegraph/sourcegraph",
			},
			userOwnedWithToken.URN(): {
				ID:       userOwnedWithToken.URN(),
				CloneURL: "https://123@github.com/sourcegraph/sourcegraph",
			},
			withToken.URN(): {
				ID:       withToken.URN(),
				CloneURL: "https://123@github.com/sourcegraph/sourcegraph",
			},
			withTokenNewer.URN(): {
				ID:       withTokenNewer.URN(),
				CloneURL: "https://123456@github.com/sourcegraph/sourcegraph",
			},
		},
	}

	database.Mocks.ExternalServices.List = func(opt database.ExternalServicesListOptions) ([]*types.ExternalService, error) {
		sources := make([]*types.ExternalService, 0)
		if _, ok := repo.Sources[noToken.URN()]; ok {
			sources = append(sources, &noToken)
		}
		if _, ok := repo.Sources[userOwnedWithToken.URN()]; ok {
			sources = append(sources, &userOwnedWithToken)
		}
		if _, ok := repo.Sources[withToken.URN()]; ok {
			sources = append(sources, &withToken)
		}
		if _, ok := repo.Sources[withTokenNewer.URN()]; ok {
			sources = append(sources, &withTokenNewer)
		}
		return sources, nil
	}
	t.Cleanup(func() {
		database.Mocks.ExternalServices.List = nil
	})

	// Expect the newest public external service with a token to be returned.
	svc, err := loadExternalService(ctx, &database.ExternalServiceStore{}, repo)
	if err != nil {
		t.Fatalf("invalid error, expected nil, got %v", err)
	}
	if have, want := svc.ID, withTokenNewer.ID; have != want {
		t.Fatalf("invalid external service returned, want=%d have=%d", want, have)
	}

	// Now delete the global external services and expect the user owned external service to be returned.
	delete(repo.Sources, withTokenNewer.URN())
	delete(repo.Sources, withToken.URN())
	svc, err = loadExternalService(ctx, &database.ExternalServiceStore{}, repo)
	if err != nil {
		t.Fatalf("invalid error, expected nil, got %v", err)
	}
	if have, want := svc.ID, userOwnedWithToken.ID; have != want {
		t.Fatalf("invalid external service returned, want=%d have=%d", want, have)
	}
}

package sources

import (
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"

	"github.com/sourcegraph/sourcegraph/enterprise/internal/batches/store"
	"github.com/sourcegraph/sourcegraph/internal/actor"
	"github.com/sourcegraph/sourcegraph/internal/batches"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/errcode"
	"github.com/sourcegraph/sourcegraph/internal/extsvc/auth"
	"github.com/sourcegraph/sourcegraph/internal/repos"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/schema"
)

type Sourcer struct {
	sourcer repos.Sourcer
	store   *store.Store
}

func NewSourcer(sourcer repos.Sourcer, store *store.Store) *Sourcer {
	return &Sourcer{
		sourcer,
		store,
	}
}

func (s *Sourcer) ForChangeset(ctx context.Context, ch *batches.Changeset) (repos.ChangesetSource, error) {
	repo, err := s.store.Repos().Get(ctx, ch.RepoID)
	if err != nil {
		return nil, err
	}
	return s.ForRepo(ctx, repo)
}

func (s *Sourcer) ForRepo(ctx context.Context, repo *types.Repo) (repos.ChangesetSource, error) {
	extSvc, err := loadExternalService(ctx, s.store.ExternalServices(), repo)
	if err != nil {
		return nil, err
	}
	css, err := buildChangesetSource(s.sourcer, extSvc)
	if err != nil {
		return nil, err
	}
	// TODO: Should this be the default?
	// cred, err := loadSiteCredential(ctx, s.store, repo)
	// if err != nil {
	// 	return nil, err
	// }
	// if cred != nil {
	// 	return s.WithAuthenticator(css, cred)
	// }
	return css, nil
}

func (s *Sourcer) WithAuthenticatorForActor(ctx context.Context, css repos.ChangesetSource, repo *types.Repo) (repos.ChangesetSource, error) {
	act := actor.FromContext(ctx)
	if !act.IsAuthenticated() {
		return nil, errors.New("cannot get authenticator from actor: no user in context")
	}
	return s.WithAuthenticatorForUser(ctx, css, act.UID, repo)
}

func (s *Sourcer) WithAuthenticatorForUser(ctx context.Context, css repos.ChangesetSource, userID int32, repo *types.Repo) (repos.ChangesetSource, error) {
	cred, err := loadUserCredential(ctx, s.store, userID, repo)
	if err != nil {
		return nil, err
	}
	if cred != nil {
		return s.WithAuthenticator(css, cred)
	}

	cred, err = loadSiteCredential(ctx, s.store, repo)
	if err != nil {
		return nil, err
	}
	if cred != nil {
		return s.WithAuthenticator(css, cred)
	}
	// For now, default to the internal authenticator of the source.
	// This is either a site-credential or the external service token.

	// If neither exist, we need to check if the user is an admin: if they are,
	// then we can use the nil return from loadUserCredential() to fall
	// back to the global credentials used for the code host. If
	// not, then we need to error out.
	// Once we tackle https://github.com/sourcegraph/sourcegraph/issues/16814,
	// this code path should be removed.
	user, err := database.UsersWith(s.store).GetByID(ctx, userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load user")
	}
	if user.SiteAdmin {
		return css, nil
	}

	// Otherwise, we can't authenticate the given ChangesetSource, so we need to bail out.
	return nil, ErrMissingCredentials
}

// WithSiteAuthenticator uses the site credential of the code host of the passed-in repo.
// If no credential is found, the original source is returned and uses the external service
// config.
func (s *Sourcer) WithSiteAuthenticator(ctx context.Context, css repos.ChangesetSource, repo *types.Repo) (repos.ChangesetSource, error) {
	cred, err := loadSiteCredential(ctx, s.store, repo)
	if err != nil {
		return nil, err
	}
	if cred != nil {
		return s.WithAuthenticator(css, cred)
	}
	return css, nil
}

var ErrMissingCredentials = errors.New("no credential found")

func (s *Sourcer) WithAuthenticator(css repos.ChangesetSource, au auth.Authenticator) (repos.ChangesetSource, error) {
	return authenticateChangesetSource(css, au)
}

// loadExternalService looks up all external services that are connected to the given repo.
// The first external service to have a token configured will be returned then.
// If no external service matching the above criteria is found, an error is returned.
func loadExternalService(ctx context.Context, s *database.ExternalServiceStore, repo *types.Repo) (*types.ExternalService, error) {
	es, err := s.List(ctx, database.ExternalServicesListOptions{
		// Consider all available external services for this repo.
		IDs: repo.ExternalServiceIDs(),
	})
	if err != nil {
		return nil, err
	}

	// Sort the external services so user owned external service go last.
	// This also retains the initial ORDER BY ID DESC.
	sort.SliceStable(es, func(i, j int) bool {
		return es[i].NamespaceUserID == 0 && es[i].ID > es[j].ID
	})

	for _, e := range es {
		cfg, err := e.Configuration()
		if err != nil {
			return nil, err
		}

		switch cfg := cfg.(type) {
		case *schema.GitHubConnection:
			if cfg.Token != "" {
				return e, nil
			}
		case *schema.BitbucketServerConnection:
			if cfg.Token != "" {
				return e, nil
			}
		case *schema.GitLabConnection:
			if cfg.Token != "" {
				return e, nil
			}
		}
	}

	return nil, errors.Errorf("no external services found for repo %q", repo.Name)
}

// buildChangesetSource get an authenticated ChangesetSource for the given repo
// to load the changeset state from.
func buildChangesetSource(sourcer repos.Sourcer, externalService *types.ExternalService) (repos.ChangesetSource, error) {
	// Then, use the external service to build a ChangesetSource.
	sources, err := sourcer(externalService)
	if err != nil {
		return nil, err
	}
	if len(sources) != 1 {
		return nil, fmt.Errorf("got no Source for external service of kind %q", externalService.Kind)
	}
	source := sources[0]
	css, ok := source.(repos.ChangesetSource)
	if !ok {
		return nil, fmt.Errorf("cannot create ChangesetSource from external service of kind %q", externalService.Kind)
	}
	return css, nil
}

func authenticateChangesetSource(src repos.ChangesetSource, au auth.Authenticator) (repos.ChangesetSource, error) {
	userSource, ok := src.(repos.UserSource)
	if !ok {
		return nil, fmt.Errorf("cannot create UserSource from external service of kind %q", "externalService.Kind")
	}
	repoSource, err := userSource.WithAuthenticator(au)
	if err != nil {
		return nil, err
	}
	css, ok := repoSource.(repos.ChangesetSource)
	// This should never happen.
	if !ok {
		return nil, fmt.Errorf("cannot create ChangesetSource from external service of kind %q", "externalService.Kind")
	}
	return css, nil
}

func loadUserCredential(ctx context.Context, s *store.Store, userID int32, repo *types.Repo) (auth.Authenticator, error) {
	cred, err := s.UserCredentials().GetByScope(ctx, database.UserCredentialScope{
		Domain:              database.UserCredentialDomainBatches,
		UserID:              userID,
		ExternalServiceType: repo.ExternalRepo.ServiceType,
		ExternalServiceID:   repo.ExternalRepo.ServiceID,
	})
	if err != nil && !errcode.IsNotFound(err) {
		return nil, err
	}
	if cred != nil {
		return cred.Credential, nil
	}
	return nil, nil
}

func loadSiteCredential(ctx context.Context, s *store.Store, repo *types.Repo) (auth.Authenticator, error) {
	cred, err := s.GetSiteCredential(ctx, store.GetSiteCredentialOpts{
		ExternalServiceType: repo.ExternalRepo.ServiceType,
		ExternalServiceID:   repo.ExternalRepo.ServiceID,
	})
	if err != nil && err != store.ErrNoResults {
		return nil, err
	}
	if cred != nil {
		return cred.Credential, nil
	}
	return nil, nil
}

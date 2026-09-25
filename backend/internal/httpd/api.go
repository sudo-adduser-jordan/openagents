package httpd

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/attachmentstore"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/cdc"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/config"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apispec"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/controllers"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/envelope"
	prsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/pr"
	projectsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/project"
	reviewsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/review"
)

// APIDeps bundles every service the API layer's controllers depend on.
type APIDeps struct {
	Agents             controllers.AgentCatalog
	Projects           projectsvc.Manager
	Sessions           controllers.SessionService
	DesktopWorkspaces  controllers.DesktopWorkspaceService
	Activity           controllers.ActivityRecorder
	UsageHooks         controllers.UsageHookRecorder
	UsageSummary       controllers.UsageSummaryService
	PRs                prsvc.ActionManager
	Reviews            reviewsvc.Manager
	Notifications      controllers.NotificationService
	NotificationStream controllers.NotificationStream
	Import             controllers.ImportService
	ShellTerminals     controllers.ShellTerminalService
	// Conversations is nil until a Chat driver is wired; the controller then
	// answers 501 rather than panicking, matching the other optional surfaces.
	Conversations controllers.ConversationService
	// Settings is the daemon-owned preference surface.
	Settings controllers.SettingsService
	// OpencodeConfig reads and writes the user's own opencode configuration.
	OpencodeConfig      controllers.OpencodeConfigService
	DevImport           controllers.DevImportService
	CDC                 cdc.Source
	Events              cdcSubscriber
	Browser             controllers.BrowserService
	PreviewServer       controllers.ManagedPreviewServer
	SessionCapabilities controllers.SessionCapabilityValidator
	SystemChecks        controllers.SystemChecker
	Installer           controllers.Installer
	AgentAuth           controllers.AgentAuthService
}

// API owns one controller per resource and is the single Register call the
// router invokes to mount the /api/v1 surface.
type API struct {
	cfg            config.Config
	deps           APIDeps
	agents         *controllers.AgentsController
	projects       *controllers.ProjectsController
	sessions       *controllers.SessionsController
	desktop        *controllers.DesktopWorkspaceController
	usage          *controllers.UsageController
	prs            *controllers.PRsController
	reviews        *controllers.ReviewsController
	notifications  *controllers.NotificationsController
	imports        *controllers.ImportController
	shellTerms     *controllers.ShellTerminalsController
	conversations  *controllers.ConversationsController
	settings       *controllers.SettingsController
	opencodeConfig *controllers.OpencodeConfigController
	dev            *controllers.DevController
	browser        *controllers.BrowserController
	system         *controllers.SystemController
	systemInstall  *controllers.SystemInstallController
	agentAuth      *controllers.AgentAuthController
	events         *EventsController
}

// NewAPI constructs the API surface from its dependencies. cfg carries the
// per-request timeout so the REST group can apply it without re-reading the
// environment.
func NewAPI(cfg config.Config, deps APIDeps) *API {
	return &API{
		cfg:  cfg,
		deps: deps,
		agents: &controllers.AgentsController{
			Catalog: deps.Agents,
		},
		projects: &controllers.ProjectsController{
			Mgr: deps.Projects,
		},
		sessions: &controllers.SessionsController{
			Svc:           deps.Sessions,
			Activity:      deps.Activity,
			Usage:         deps.UsageHooks,
			Attachments:   attachmentstore.New(cfg.DataDir),
			PreviewServer: deps.PreviewServer,
			Capabilities:  deps.SessionCapabilities,
		},
		desktop:        &controllers.DesktopWorkspaceController{Svc: deps.DesktopWorkspaces},
		usage:          &controllers.UsageController{Svc: deps.UsageSummary},
		prs:            &controllers.PRsController{Svc: deps.PRs},
		reviews:        &controllers.ReviewsController{Svc: deps.Reviews},
		notifications:  &controllers.NotificationsController{Svc: deps.Notifications, Stream: deps.NotificationStream},
		imports:        &controllers.ImportController{Svc: deps.Import},
		shellTerms:     &controllers.ShellTerminalsController{Svc: deps.ShellTerminals},
		conversations:  &controllers.ConversationsController{Svc: deps.Conversations},
		settings:       &controllers.SettingsController{Svc: deps.Settings},
		opencodeConfig: &controllers.OpencodeConfigController{Svc: deps.OpencodeConfig},
		dev:            &controllers.DevController{Import: deps.DevImport},
		browser:        &controllers.BrowserController{Svc: deps.Browser},
		system:         &controllers.SystemController{Checks: deps.SystemChecks},
		systemInstall:  &controllers.SystemInstallController{Installer: deps.Installer},
		agentAuth:      &controllers.AgentAuthController{Svc: deps.AgentAuth},
		events:         &EventsController{Source: deps.CDC, Live: deps.Events},
	}
}

// Register mounts the bounded /api/v1 REST surface. Long-lived surfaces such
// as muxed terminal streams stay outside this timeout group.
func (a *API) Register(root chi.Router) {
	timeout := a.cfg.RequestTimeout
	if timeout <= 0 {
		timeout = config.DefaultRequestTimeout
	}
	root.Route("/api/v1", func(r chi.Router) {
		// Serve the OpenAPI document from the same origin as the routes it describes.
		r.Get("/openapi.yaml", apispec.ServeYAML)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(timeout))
			a.agents.Register(r)
			a.projects.Register(r)
			a.sessions.Register(r)
			a.desktop.Register(r)
			a.usage.Register(r)
			a.prs.Register(r)
			a.reviews.Register(r)
			a.notifications.Register(r)
			a.imports.Register(r)
			a.shellTerms.Register(r)
			a.conversations.Register(r)
			a.settings.Register(r)
			a.opencodeConfig.Register(r)
			a.dev.Register(r)
			a.browser.Register(r)
			a.system.Register(r)
			a.systemInstall.Register(r)
			a.agentAuth.Register(r)
			// Sibling REST controllers plug in here.
		})
		// Long-lived streams intentionally bypass the REST timeout middleware.
		a.notifications.RegisterStream(r)
		a.sessions.RegisterStreams(r)
		a.events.Register(r)
	})
}

// notFoundJSON returns the locked envelope for unmatched routes. Chi's default
// 404 is a text/plain body; the API surface must answer JSON so consumers can
// parse it uniformly.
func notFoundJSON(w http.ResponseWriter, r *http.Request) {
	envelope.WriteAPIError(w, r, http.StatusNotFound, "not_found", "ROUTE_NOT_FOUND",
		r.Method+" "+r.URL.Path+" has no handler", nil)
}

// methodNotAllowedJSON returns the locked envelope when a method probes a
// known path without a matching verb (e.g. PUT /projects/{id} after we drop
// the legacy PUT alias).
func methodNotAllowedJSON(w http.ResponseWriter, r *http.Request) {
	envelope.WriteAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "METHOD_NOT_ALLOWED",
		r.Method+" not allowed on "+r.URL.Path, nil)
}

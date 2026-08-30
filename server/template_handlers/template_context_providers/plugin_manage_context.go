package template_context_providers

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/plugin_system"
)

type pluginDisplay struct {
	Name        string
	Version     string
	Description string
	Enabled     bool
	HasDocs     bool

	// AllowScopedPrincipals is the operator's separate decision about *who* may
	// reach this plugin, as opposed to what it may do: group-limited users and
	// guests are refused its pages, endpoints, shortcodes and slots unless this
	// is on.
	AllowScopedPrincipals bool
	Settings              []plugin_system.SettingDefinition
	Values                map[string]any

	// Legacy is true for a plugin that declares no manifest at all: it keeps
	// the full mah surface, which is why Capabilities below lists everything.
	Legacy     bool
	APIVersion int

	// Capabilities is the effective set (db:write implies db:read), and
	// CapabilityLabels is the human sentence for each one, so an operator reads
	// a described power rather than a slug.
	Capabilities     []string
	CapabilityLabels map[string]string

	// Network is the declared egress allowlist in canonical display form. An
	// empty list means "any public host" — the broadest policy, not the absence
	// of network access.
	Network           []string
	AllowPrivateHosts bool

	Dependencies []string

	// MinAppVersion is informational only: it is parsed and displayed, never
	// enforced.
	MinAppVersion string

	// Schedules is what this plugin has recorded as recurring work. Rendered
	// from the stored rows rather than from the live registry, because the two
	// differ in exactly the cases an operator needs to see: a row whose plugin
	// is disabled is inert but still there, and a row with no owner never runs.
	Schedules []scheduleDisplay

	// ScheduledDownloads are one-shot deferred host downloads a plugin submitted.
	// They are stored rows until the scheduler turns them into queue jobs.
	ScheduledDownloads []scheduledDownloadDisplay
}

// scheduleRefRE is `<plugin>/<schedule>` in the grammars the two halves are
// actually validated against at registration: a plugin name, and a schedule id.
// Anything else is not a schedule reference and is dropped rather than shown.
var scheduleRefRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}/[A-Za-z0-9_-]{1,100}$`)

// scheduleDisplay is one PluginSchedule row, flattened for the template.
type scheduleDisplay struct {
	ScheduleID string
	Every      string
	Overlap    string
	NextDueAt  time.Time
	LastRunAt  *time.Time
	LastStatus string
	LastError  string
	Runs       int64

	// Owned is false when the operator who enabled the plugin has since been
	// deleted. The schedule is then stopped, not merely unattributed, so the
	// page has to say so rather than leaving a blank column.
	Owned bool

	// Registered is false when the row exists but the plugin no longer declares
	// this id -- disabled, renamed, or removed from plugin.lua.
	Registered bool
}

// scheduleLister is the optional capability this page uses to read schedules.
//
// A type assertion rather than a method on PluginManagePageContext: widening
// that interface would force every test mock of it to grow a method it does not
// care about, and a page that cannot read schedules should render without them
// rather than fail to compile its callers.
type scheduleLister interface {
	PluginSchedulesFor(pluginName string) ([]models.PluginSchedule, error)
}

// scheduledDownloadLister is the optional capability this page uses to read
// one-shot deferred plugin downloads, kept optional for the same reason as
// scheduleLister.
type scheduledDownloadLister interface {
	PluginScheduledDownloadsFor(pluginName string) ([]models.ScheduledDownload, error)
}

// buildScheduleDisplays reads one plugin's rows and marks each against the live
// registry.
func buildScheduleDisplays(appCtx PluginManagePageContext, pm *plugin_system.PluginManager, name string) []scheduleDisplay {
	lister, ok := appCtx.(scheduleLister)
	if !ok {
		return nil
	}
	rows, err := lister.PluginSchedulesFor(name)
	if err != nil {
		log.Printf("[plugin] warning: failed to load schedules for %q: %v", name, err)
		return nil
	}
	out := make([]scheduleDisplay, 0, len(rows))
	for _, row := range rows {
		out = append(out, scheduleDisplay{
			ScheduleID: row.ScheduleID,
			Every:      (time.Duration(row.EverySeconds) * time.Second).String(),
			Overlap:    row.Overlap,
			NextDueAt:  row.NextDueAt,
			LastRunAt:  row.LastRunAt,
			LastStatus: row.LastStatus,
			LastError:  row.LastError,
			Runs:       row.Runs,
			Owned:      row.CreatedByUserId != nil,
			Registered: pm.ScheduleIsRegistered(name, row.ScheduleID),
		})
	}
	return out
}

// scheduledDownloadDisplay is one ScheduledDownload row, flattened for the
// plugin management template.
type scheduledDownloadDisplay struct {
	ID          uint
	URL         string
	DueAt       time.Time
	Status      string
	StatusLabel string
	JobID       string
	LastError   string
	Attempts    int
	Owned       bool
}

func scheduledDownloadStatusLabel(status string, owned bool) string {
	if !owned && status == models.ScheduledDownloadStatusPending {
		return "stopped"
	}
	return status
}

func buildScheduledDownloadDisplays(appCtx PluginManagePageContext, name string) []scheduledDownloadDisplay {
	lister, ok := appCtx.(scheduledDownloadLister)
	if !ok {
		return nil
	}
	rows, err := lister.PluginScheduledDownloadsFor(name)
	if err != nil {
		log.Printf("[plugin] warning: failed to load scheduled downloads for %q: %v", name, err)
		return nil
	}
	out := make([]scheduledDownloadDisplay, 0, len(rows))
	for _, row := range rows {
		owned := row.CreatedByUserId != nil
		out = append(out, scheduledDownloadDisplay{
			ID:          row.ID,
			URL:         row.URL,
			DueAt:       row.DueAt,
			Status:      row.Status,
			StatusLabel: scheduledDownloadStatusLabel(row.Status, owned),
			JobID:       row.JobID,
			LastError:   row.LastError,
			Attempts:    row.Attempts,
			Owned:       owned,
		})
	}
	return out
}

// capabilityLabels maps each capability to its human sentence, skipping any
// that has no label.
func capabilityLabels(caps []string) map[string]string {
	labels := make(map[string]string, len(caps))
	for _, c := range caps {
		if label, ok := plugin_system.CapabilityLabels[c]; ok {
			labels[c] = label
		}
	}
	return labels
}

func PluginManageContextProvider(appCtx PluginManagePageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := StaticTemplateCtx(request)
		ctx["pageTitle"] = "Manage Plugins"

		// What a manual run just started, if the operator arrived here from the
		// run-now button without JavaScript. The run is asynchronous, so the row
		// below still reads "never run" at this point and the page would
		// otherwise look as though the button did nothing.
		//
		// Matched against the grammar the two halves actually have rather than
		// echoed and escaped. Escaping alone would be enough to make it safe, but
		// this is a URL parameter rendered on an admin page, and "safe because
		// the template escapes it" is a property of every template that renders
		// it — a narrower promise than "it cannot be anything but a name".
		if started := strings.TrimSpace(request.URL.Query().Get("started")); scheduleRefRE.MatchString(started) {
			ctx["scheduleStarted"] = started
		}

		pm := appCtx.PluginManager()
		if pm == nil {
			ctx["plugins"] = []pluginDisplay{}
			return ctx
		}

		discovered := pm.DiscoveredPlugins()
		states, err := appCtx.GetPluginStates()
		if err != nil {
			log.Printf("[plugin] warning: failed to load plugin states: %v", err)
		}

		stateMap := make(map[string]struct {
			enabled     bool
			settings    string
			allowScoped bool
		})
		for _, s := range states {
			stateMap[s.PluginName] = struct {
				enabled     bool
				settings    string
				allowScoped bool
			}{s.Enabled, s.SettingsJSON, s.AllowScopedPrincipals}
		}

		var plugins []pluginDisplay
		for _, dp := range discovered {
			caps := dp.Manifest.Capabilities().Sorted()
			pd := pluginDisplay{
				Name:               dp.Name,
				Version:            dp.Version,
				Description:        dp.Description,
				HasDocs:            pm.PluginHasDocs(dp.Name),
				Settings:           dp.Settings,
				Values:             make(map[string]any),
				Legacy:             !dp.Manifest.Declared,
				APIVersion:         dp.Manifest.APIVersion,
				Capabilities:       caps,
				CapabilityLabels:   capabilityLabels(caps),
				Network:            dp.Manifest.NetworkDisplay(),
				AllowPrivateHosts:  dp.Manifest.AllowPrivateHosts,
				Dependencies:       dp.Manifest.Dependencies,
				MinAppVersion:      dp.Manifest.MinAppVersion,
				Schedules:          buildScheduleDisplays(appCtx, pm, dp.Name),
				ScheduledDownloads: buildScheduledDownloadDisplays(appCtx, dp.Name),
			}
			if s, ok := stateMap[dp.Name]; ok {
				pd.Enabled = s.enabled
				pd.AllowScopedPrincipals = s.allowScoped
				if s.settings != "" {
					json.Unmarshal([]byte(s.settings), &pd.Values)
				}
			}
			plugins = append(plugins, pd)
		}

		ctx["plugins"] = plugins
		return ctx
	}
}

package plugin_system

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	lua "github.com/yuin/gopher-lua"
)

// PluginAPIVersion is the plugin API generation this host implements.
//
// A plugin declares the generation it was written against; one declaring a
// newer generation is refused rather than allowed to fail deep inside init()
// against functions that do not exist yet. A plugin declaring an older
// generation is accepted: there is only one generation, so a compatibility
// branch here could not be tested honestly. The first bump has to decide that
// deliberately.
const PluginAPIVersion = 1

// Capability names. Each one corresponds to a seam the mah table is already
// built from, so an ungranted capability is a key that is never set rather than
// a check at every call site.
const (
	CapDBRead  = "db:read"  // the mah.db readers (everything built on querierFor)
	CapDBWrite = "db:write" // the mah.db writers (everything built on writerFor)
	CapHTTP    = "http"     // mah.http
	CapKV      = "kv"       // mah.kv
	CapImage   = "image"    // mah.image
	// CapMedia is separate from CapImage for the reason CapSchedule is separate
	// from CapJobs. mah.image transforms bytes the plugin already holds and
	// touches nothing else; mah.media reads video and audio *out of the user's
	// library* and spends an ffmpeg process doing it. Folding them together
	// would silently widen every plugin already consented to image transforms.
	CapMedia   = "media"   // mah.media
	CapHooks   = "hooks"   // mah.on
	CapInject  = "inject"  // mah.inject
	CapRender  = "render"  // mah.shortcode, mah.block_type, mah.display_type
	CapPages   = "pages"   // mah.page, mah.menu
	CapAPI     = "api"     // mah.api
	CapActions = "actions" // mah.action
	CapJobs    = "jobs"    // mah.start_job and the job_* reporters
	// CapSchedule is deliberately not part of CapJobs. Folding it in would
	// silently turn every plugin already consented to "jobs" into one that can
	// run unattended work on a timer, which is a different power from running
	// work a user just asked for. A separate name makes CompareGrants report a
	// widening and refuse the load until an operator re-enables, which is the
	// entire point of storing consent.
	CapSchedule = "schedule" // mah.schedule
	// CapJobEvents is separate from CapHooks for the reason CapSchedule is
	// separate from CapJobs. The entity hooks fire on a write the caller just
	// made, so a plugin holding "hooks" observes what its own users are doing.
	// A job event fires for every background job in the deployment, whoever
	// submitted it, including work the plugin had nothing to do with — that is
	// unattended observation of other people's activity, which is a different
	// power and gets its own name so CompareGrants reports the widening.
	CapJobEvents = "job_events" // mah.on for the after_job_* events
)

// AllCapabilities is every grantable capability, in the order the manage UI
// lists them.
var AllCapabilities = []string{
	CapDBRead, CapDBWrite, CapHTTP, CapKV, CapImage, CapMedia,
	CapHooks, CapInject, CapRender, CapPages, CapAPI, CapActions, CapJobs, CapSchedule,
	CapJobEvents,
}

// CapabilityLabels are the human sentences the manage UI renders instead of
// slugs, so consent is given to a described power rather than to a name.
var CapabilityLabels = map[string]string{
	CapDBRead:    "Read your library (resources, notes, groups, tags)",
	CapDBWrite:   "Create, change and delete library content, read it, and fetch a URL of its choosing into it",
	CapHTTP:      "Make outbound network requests",
	CapKV:        "Store its own key-value data",
	CapImage:     "Transform images",
	CapMedia:     "Read and cut the video and audio in your library",
	CapHooks:     "React to entity changes, and veto them",
	CapInject:    "Inject HTML and scripts into every page",
	CapRender:    "Render shortcodes, note blocks and metadata displays",
	CapPages:     "Serve its own pages and add menu entries (every plugin also has an auto-generated docs page)",
	CapAPI:       "Serve its own JSON endpoints",
	CapActions:   "Add actions to resources, notes and groups",
	CapJobs:      "Run background jobs",
	CapSchedule:  "Run its own work on a repeating schedule, without anyone asking",
	CapJobEvents: "See every background job in this deployment finish, whoever started it",
}

// CapabilitySurfaces names what each capability installs, for the one log line
// emitted per withheld module at load. Without it, an ungranted capability
// surfaces only as "attempt to index a nil value" somewhere inside init().
var CapabilitySurfaces = map[string]string{
	CapDBRead:    "mah.db readers",
	CapDBWrite:   "mah.db writers, mah.download",
	CapHTTP:      "mah.http",
	CapKV:        "mah.kv",
	CapImage:     "mah.image",
	CapMedia:     "mah.media",
	CapHooks:     "mah.on",
	CapInject:    "mah.inject",
	CapRender:    "mah.shortcode, mah.block_type, mah.display_type",
	CapPages:     "mah.page, mah.menu",
	CapAPI:       "mah.api",
	CapActions:   "mah.action, and the job_* reporters an async action needs",
	CapJobs:      "mah.start_job, and the job_* reporters",
	CapSchedule:  "mah.schedule",
	CapJobEvents: "mah.on for the after_job_* events",
}

// CapabilitySurfacesWithoutReporters is what to say about actions or jobs when
// the *other* one is granted: the job_* reporters come with either, so naming
// them as withheld would be false.
var CapabilitySurfacesWithoutReporters = map[string]string{
	CapActions: "mah.action",
	CapJobs:    "mah.start_job",
}

// CapabilitySet is a set of granted capability names.
type CapabilitySet map[string]struct{}

// Has reports whether the capability is granted.
func (s CapabilitySet) Has(name string) bool {
	_, ok := s[name]
	return ok
}

// Sorted returns the granted capabilities in AllCapabilities order.
func (s CapabilitySet) Sorted() []string {
	out := make([]string, 0, len(s))
	for _, c := range AllCapabilities {
		if s.Has(c) {
			out = append(out, c)
		}
	}
	return out
}

func newCapabilitySet(names []string) CapabilitySet {
	set := make(CapabilitySet, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

// Manifest is the package metadata a plugin declares on its `plugin` global.
//
// Declared is false for a plugin that predates the manifest ("legacy"): it
// keeps the full mah surface, with a warning. Legacy is *not* an exemption from
// the network rules — those are a vulnerability fix, and exempting the plugins
// that exist today would exempt exactly the population that has the hole.
type Manifest struct {
	Declared          bool            `json:"declared"`
	APIVersion        int             `json:"api_version,omitempty"`
	CapabilityList    []string        `json:"capabilities,omitempty"`
	Network           []string        `json:"network,omitempty"`
	DownloadLimits    []DownloadLimit `json:"download_limits,omitempty"`
	AllowPrivateHosts bool            `json:"allow_private_hosts,omitempty"`
	Dependencies      []string        `json:"dependencies,omitempty"`
	MinAppVersion     string          `json:"min_app_version,omitempty"`

	// rules is Network parsed, kept so matching never re-parses and never has
	// to handle a parse failure at request time.
	rules []NetworkRule
}

// DownloadLimit is one manifest-declared per-domain pacing rule. Concurrency 0
// means unlimited, and zero durations mean the corresponding wait is disabled.
// The rule field is the parsed Host and keeps matching on the same grammar as
// plugin network allowlists.
type DownloadLimit struct {
	Host        string        `json:"host"`
	Concurrency int           `json:"concurrency,omitempty"`
	MinInterval time.Duration `json:"min_interval,omitempty"`
	Backoff     time.Duration `json:"backoff,omitempty"`

	rule NetworkRule
}

// DownloadPolicy is the effective pacing rule for a submitted download host.
type DownloadPolicy struct {
	Concurrency int
	MinInterval time.Duration
	Backoff     time.Duration
}

// Capabilities returns the set this manifest grants: everything for a legacy
// plugin, and otherwise what was declared, plus what that implies.
//
// db:write implies db:read. The writers already return the entity they wrote —
// patch_note(id, {}) changes nothing and hands back the whole note — so a
// write-only grant was a read of anything by id, wearing a write's clothes.
// Rather than strip the return values (a plugin needs them, and it would only
// move the leak to the next accessor that reads state to write it), the
// implication is made explicit, so the label an operator consents to is what
// they actually grant. The reverse never holds: db:read installs no writer,
// which is the direction that matters.
func (m Manifest) Capabilities() CapabilitySet {
	if !m.Declared {
		return newCapabilitySet(AllCapabilities)
	}
	set := newCapabilitySet(m.CapabilityList)
	if set.Has(CapDBWrite) {
		set[CapDBRead] = struct{}{}
	}
	return set
}

// NetworkPolicy returns the egress policy this manifest describes.
func (m Manifest) NetworkPolicy() NetworkPolicy {
	return NetworkPolicy{
		Rules:        m.rules,
		AllowPrivate: m.AllowPrivateHosts,
		Unrestricted: len(m.rules) == 0,
	}
}

// Matches reports whether this download limit applies to host, using the same
// grammar as the plugin network allowlist.
func (l DownloadLimit) Matches(host string) bool {
	return l.rule.Matches(host)
}

// DownloadPolicy returns the first manifest download limit that matches host.
// First match wins because rule order is the author's way to put a specific
// exception before a broader wildcard.
func (m Manifest) DownloadPolicy(host string) (DownloadPolicy, bool) {
	for _, limit := range m.DownloadLimits {
		if limit.rule.Matches(host) {
			return DownloadPolicy{
				Concurrency: limit.Concurrency,
				MinInterval: limit.MinInterval,
				Backoff:     limit.Backoff,
			}, true
		}
	}
	return DownloadPolicy{}, false
}

// Equal reports whether two manifests declare the same thing. Used to detect a
// plugin.lua that changed between discovery and load: the grants were built
// from the discovered read, so a different file is a different package.
func (m Manifest) Equal(other Manifest) bool {
	if m.Declared != other.Declared ||
		m.APIVersion != other.APIVersion ||
		m.AllowPrivateHosts != other.AllowPrivateHosts ||
		m.MinAppVersion != other.MinAppVersion {
		return false
	}
	return sameStrings(m.CapabilityList, other.CapabilityList) &&
		sameStrings(m.Network, other.Network) &&
		sameStrings(m.Dependencies, other.Dependencies) &&
		sameDownloadLimits(m.DownloadLimits, other.DownloadLimits)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func sameDownloadLimits(a, b []DownloadLimit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Host != b[i].Host ||
			a[i].Concurrency != b[i].Concurrency ||
			a[i].MinInterval != b[i].MinInterval ||
			a[i].Backoff != b[i].Backoff {
			return false
		}
	}
	return true
}

// manifestKeys are the fields that only mean something inside a manifest.
// Declaring one without api_version is an error rather than a silent grant of
// everything, because the plugin plainly meant to declare a manifest.
var manifestKeys = []string{"capabilities", "network", "download_limits", "allow_private_hosts", "dependencies", "min_app_version"}

// ParseManifest reads the manifest fields off a plugin's `plugin` table.
// A table with no api_version is legacy, provided it declares no other manifest
// field.
func ParseManifest(tbl *lua.LTable) (Manifest, error) {
	var m Manifest

	// Every field below is read with RawGetString, which ignores __index. A
	// plugin table that inherits api_version through a metatable would
	// therefore read as legacy — the full surface — while the file appears to
	// declare a narrow manifest. Honouring __index instead is worse: an __index
	// *function* can answer the parse and the reader differently, and can
	// answer differently on each call. So a metatable is refused outright; a
	// manifest has no use for one.
	if tbl.Metatable != nil && tbl.Metatable != lua.LNil {
		return m, fmt.Errorf("the plugin table must not have a metatable: the manifest is read field by field, and an inherited field would be read as absent")
	}

	if err := rejectNearMissKeys(tbl); err != nil {
		return m, err
	}

	selfName := ""
	if v := tbl.RawGetString("name"); v != lua.LNil {
		selfName = v.String()
	}

	apiVersion := tbl.RawGetString("api_version")
	if apiVersion == lua.LNil {
		for _, key := range manifestKeys {
			if tbl.RawGetString(key) != lua.LNil {
				return m, fmt.Errorf("plugin declares %q but no api_version: add api_version = %d", key, PluginAPIVersion)
			}
		}
		return m, nil
	}

	num, ok := apiVersion.(lua.LNumber)
	if !ok {
		return m, fmt.Errorf("api_version must be a number, got %s", apiVersion.Type())
	}
	if float64(num) != float64(int(num)) || int(num) < 1 {
		return m, fmt.Errorf("api_version must be a positive whole number, got %v", num)
	}
	m.Declared = true
	m.APIVersion = int(num)

	caps, err := manifestStrings(tbl, "capabilities")
	if err != nil {
		return m, err
	}
	known := newCapabilitySet(AllCapabilities)
	seen := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		if !known.Has(c) {
			return m, fmt.Errorf("unknown capability %q (known: %s)", c, strings.Join(AllCapabilities, ", "))
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		m.CapabilityList = append(m.CapabilityList, c)
	}

	hosts, err := manifestStrings(tbl, "network")
	if err != nil {
		return m, err
	}
	// An empty list would leave m.rules nil, and no rules is how a manifest
	// says "any public host" — the opposite of what an author writing
	// `network = {}` means. Neither reading can be inferred, so the manifest
	// has to pick one.
	if tbl.RawGetString("network") != lua.LNil && len(hosts) == 0 {
		return m, fmt.Errorf("network is empty: remove the field to allow any public host, or list at least one entry")
	}
	for _, h := range hosts {
		rule, err := parseNetworkRule(h)
		if err != nil {
			return m, fmt.Errorf("network entry %q: %w", h, err)
		}
		m.Network = append(m.Network, h)
		m.rules = append(m.rules, rule)
	}

	limits, err := manifestDownloadLimits(tbl)
	if err != nil {
		return m, err
	}
	m.DownloadLimits = limits

	if v := tbl.RawGetString("allow_private_hosts"); v != lua.LNil {
		b, ok := v.(lua.LBool)
		if !ok {
			return m, fmt.Errorf("allow_private_hosts must be a boolean, got %s", v.Type())
		}
		m.AllowPrivateHosts = bool(b)
	}
	// allow_private_hosts relaxes the dial-time deny for hosts that already
	// matched the allowlist. With no allowlist everything matches, so on its
	// own it would grant unrestricted access to loopback, link-local and
	// RFC1918 — the whole hole this package closes, in two lines and one
	// checkbox. A plugin that legitimately needs a LAN service can name it.
	if m.AllowPrivateHosts {
		if len(m.rules) == 0 {
			return m, fmt.Errorf("allow_private_hosts needs a network list: name the private hosts this plugin may reach")
		}
		// Counting rules is not enough: one broad CIDR is one rule, and
		// "0.0.0.0/1" would satisfy a count while naming nothing. The flag
		// lifts the dial-time deny for whatever matched, so what matched has to
		// be specific.
		for _, r := range m.rules {
			if broad, why := r.tooBroadForPrivateHosts(); broad {
				return m, fmt.Errorf("network entry %q is too broad to combine with allow_private_hosts: %s", r.raw, why)
			}
		}
	}

	deps, err := manifestStrings(tbl, "dependencies")
	if err != nil {
		return m, err
	}
	for _, d := range deps {
		if d == selfName {
			return m, fmt.Errorf("plugin %q depends on itself", d)
		}
		m.Dependencies = append(m.Dependencies, d)
	}

	if v := tbl.RawGetString("min_app_version"); v != lua.LNil {
		s, ok := v.(lua.LString)
		if !ok {
			return m, fmt.Errorf("min_app_version must be a string, got %s", v.Type())
		}
		// Nothing compares this against anything — there is no application
		// version constant to compare with — but a value that cannot be a
		// version at all is a mistake worth naming rather than storing.
		if !validVersion.MatchString(string(s)) {
			return m, fmt.Errorf("min_app_version %q is not a version number", string(s))
		}
		m.MinAppVersion = string(s)
	}

	return m, nil
}

// knownPluginKeys is every key this format reads off the plugin table.
var knownPluginKeys = append([]string{"name", "version", "description", "api_version"}, manifestKeys...)

// rejectNearMissKeys catches a misspelled manifest field.
//
// Every other mistake in this parser fails loudly, but a misspelled `network`
// is just an unknown key — and no rules reads as "any public host", so that one
// typo failed open. Unknown keys stay legal (a plugin may keep its own state on
// the table); a key one edit away from a real one is a typo, not state.
func rejectNearMissKeys(tbl *lua.LTable) error {
	var bad error
	tbl.ForEach(func(k, _ lua.LValue) {
		if bad != nil {
			return
		}
		key, ok := k.(lua.LString)
		if !ok {
			return
		}
		name := string(key)
		// Non-ASCII first, and unconditionally. Every rule below compares a
		// name to a field name, and a confusable defeats comparison by
		// construction: one Cyrillic "і" in "apі_version" is caught by the
		// normalized edit distance, two are not, and there is no number of
		// them that a distance threshold catches. The plugin table is metadata
		// — its field names are ASCII — so a key that is not ASCII is either a
		// mistake or a disguise, and neither should read as "unknown key,
		// therefore legacy, therefore every capability".
		for _, r := range name {
			if r > unicode.MaxASCII {
				bad = fmt.Errorf("plugin table key %q contains a non-ASCII character; manifest field names "+
					"are plain ASCII, and a lookalike would read as an unknown key and silently mean "+
					"'no manifest'", name)
				return
			}
		}
		for _, known := range knownPluginKeys {
			if name == known {
				return
			}
		}
		// Spelling first, on a normalized form: lower case, letters and digits
		// only. That catches SCREAMING_CASE, camelCase, hyphens and any
		// zero-width or confusable character someone pastes in — all of which
		// are several edits from the real name, so the near-miss rule below read
		// them as unknown keys, which meant no api_version, which meant legacy,
		// which meant all twelve capabilities.
		//
		// The failure mode this closes is specific: `apiVersion = 1` is plainly
		// an attempt to declare a manifest, and it was granted the opposite of
		// what it asked for.
		for _, known := range knownPluginKeys {
			if normalizeKey(name) == normalizeKey(known) {
				bad = fmt.Errorf("plugin.%s is not a manifest field — field names are lower case with "+
					"underscores; did you mean %q?", name, known)
				return
			}
		}
		for _, known := range knownPluginKeys {
			// On the raw name, and on the normalized one. normalizeKey drops
			// characters it does not recognise, so a confusable — Cyrillic "і"
			// in "apі_version" — survives the raw comparison as several bytes of
			// difference and survives normalization as a *missing* letter. The
			// normalized pair is one edit apart, which is what catches it. A
			// key that reads as a manifest field to a human has to be treated
			// as one, or it silently means legacy and all twelve capabilities.
			if editDistanceWithin1(name, known) || editDistanceWithin1(normalizeKey(name), normalizeKey(known)) {
				bad = fmt.Errorf("plugin.%s is not a manifest field — did you mean %q?", name, known)
				return
			}
		}
	})
	return bad
}

// normalizeKey reduces a field name to lower-case letters and digits, so that
// every way of writing one field name compares equal: API_VERSION, apiVersion,
// api-version, and anything with a zero-width character wedged into it.
func normalizeKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// editDistanceWithin1 reports whether a and b are at most one edit apart,
// counting a swap of two adjacent characters as one edit.
//
// Transposition has to count: "netowrk" is the typo people actually make, and
// plain Levenshtein scores it 2.
func editDistanceWithin1(a, b string) bool {
	if a == b {
		return false // identical is not a near miss
	}
	la, lb := len(a), len(b)
	if la > lb {
		a, b = b, a
		la, lb = lb, la
	}
	if lb-la > 1 {
		return false
	}
	for i := 0; i < la; i++ {
		if a[i] == b[i] {
			continue
		}
		if la != lb {
			// One insertion in b.
			return a[i:] == b[i+1:]
		}
		// One substitution, or one adjacent transposition.
		if a[i+1:] == b[i+1:] {
			return true
		}
		return i+1 < la && a[i] == b[i+1] && a[i+1] == b[i] && a[i+2:] == b[i+2:]
	}
	return lb == la+1
}

// manifestStrings reads an optional array-of-strings field.
//
// It insists on a dense array of consecutive integer keys. ForEach would
// happily hand back the values of a hash — so `capabilities = {secret =
// "db:write"}` would grant a write capability while reading like anything but a
// capability list — and would silently skip the tail of a sparse one.
func manifestStrings(tbl *lua.LTable, key string) ([]string, error) {
	v := tbl.RawGetString(key)
	if v == lua.LNil {
		return nil, nil
	}
	arr, ok := v.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings, got %s", key, v.Type())
	}

	n := arr.Len()
	// Len() reports the array part only; anything else in the table is a key
	// this format has no meaning for, and guessing at it is how a typo becomes
	// a grant.
	var extra error
	arr.ForEach(func(k, _ lua.LValue) {
		if extra != nil {
			return
		}
		idx, ok := k.(lua.LNumber)
		if !ok || float64(idx) != float64(int(idx)) || int(idx) < 1 || int(idx) > n {
			extra = fmt.Errorf("%s must be an array: unexpected key %v", key, k)
		}
	})
	if extra != nil {
		return nil, extra
	}

	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		item := arr.RawGetInt(i)
		s, ok := item.(lua.LString)
		if !ok {
			return nil, fmt.Errorf("%s entries must be strings, got %s", key, item.Type())
		}
		trimmed := strings.TrimSpace(string(s))
		if trimmed == "" {
			return nil, fmt.Errorf("%s entries must not be empty", key)
		}
		out = append(out, trimmed)
	}
	return out, nil
}

func manifestDownloadLimits(tbl *lua.LTable) ([]DownloadLimit, error) {
	v := tbl.RawGetString("download_limits")
	if v == lua.LNil {
		return nil, nil
	}
	arr, ok := v.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("download_limits must be an array of tables, got %s", v.Type())
	}

	n := arr.Len()
	var extra error
	arr.ForEach(func(k, _ lua.LValue) {
		if extra != nil {
			return
		}
		idx, ok := k.(lua.LNumber)
		if !ok || float64(idx) != float64(int(idx)) || int(idx) < 1 || int(idx) > n {
			extra = fmt.Errorf("download_limits must be an array: unexpected key %v", k)
		}
	})
	if extra != nil {
		return nil, extra
	}

	out := make([]DownloadLimit, 0, n)
	for i := 1; i <= n; i++ {
		item := arr.RawGetInt(i)
		entry, ok := item.(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("download_limits[%d] must be a table, got %s", i, item.Type())
		}
		limit, err := parseDownloadLimit(entry, i)
		if err != nil {
			return nil, err
		}
		out = append(out, limit)
	}
	return out, nil
}

func parseDownloadLimit(tbl *lua.LTable, idx int) (DownloadLimit, error) {
	if err := rejectDownloadLimitKeys(tbl, idx); err != nil {
		return DownloadLimit{}, err
	}

	label := fmt.Sprintf("download_limits[%d]", idx)
	hostValue := tbl.RawGetString("host")
	if hostValue == lua.LNil {
		return DownloadLimit{}, fmt.Errorf("%s.host is required", label)
	}
	hostString, ok := hostValue.(lua.LString)
	if !ok {
		return DownloadLimit{}, fmt.Errorf("%s.host must be a string, got %s", label, hostValue.Type())
	}
	host := strings.TrimSpace(string(hostString))
	if host == "" {
		return DownloadLimit{}, fmt.Errorf("%s.host must not be empty", label)
	}
	rule, err := parseNetworkRule(host)
	if err != nil {
		return DownloadLimit{}, fmt.Errorf("%s.host %q: %w", label, host, err)
	}

	concurrency, err := optionalPositiveWholeNumber(tbl, "concurrency", label)
	if err != nil {
		return DownloadLimit{}, err
	}
	minInterval, err := optionalDuration(tbl, "min_interval", label)
	if err != nil {
		return DownloadLimit{}, err
	}
	backoff, err := optionalDuration(tbl, "backoff", label)
	if err != nil {
		return DownloadLimit{}, err
	}

	return DownloadLimit{
		Host:        host,
		Concurrency: concurrency,
		MinInterval: minInterval,
		Backoff:     backoff,
		rule:        rule,
	}, nil
}

var downloadLimitKeys = []string{"host", "concurrency", "min_interval", "backoff"}

func rejectDownloadLimitKeys(tbl *lua.LTable, idx int) error {
	var bad error
	tbl.ForEach(func(k, _ lua.LValue) {
		if bad != nil {
			return
		}
		key, ok := k.(lua.LString)
		if !ok {
			bad = fmt.Errorf("download_limits[%d] has unexpected key %v", idx, k)
			return
		}
		name := string(key)
		for _, known := range downloadLimitKeys {
			if name == known {
				return
			}
		}
		for _, known := range downloadLimitKeys {
			if normalizeKey(name) == normalizeKey(known) ||
				editDistanceWithin1(name, known) ||
				editDistanceWithin1(normalizeKey(name), normalizeKey(known)) {
				bad = fmt.Errorf("download_limits[%d].%s is not a download limit field — did you mean %q?", idx, name, known)
				return
			}
		}
		bad = fmt.Errorf("download_limits[%d].%s is not a download limit field", idx, name)
	})
	return bad
}

func optionalPositiveWholeNumber(tbl *lua.LTable, key, label string) (int, error) {
	v := tbl.RawGetString(key)
	if v == lua.LNil {
		return 0, nil
	}
	n, ok := v.(lua.LNumber)
	if !ok {
		return 0, fmt.Errorf("%s.%s must be a positive whole number, got %s", label, key, v.Type())
	}
	if float64(n) != float64(int(n)) || int(n) <= 0 {
		return 0, fmt.Errorf("%s.%s must be a positive whole number when present, got %v", label, key, n)
	}
	return int(n), nil
}

func optionalDuration(tbl *lua.LTable, key, label string) (time.Duration, error) {
	v := tbl.RawGetString(key)
	if v == lua.LNil {
		return 0, nil
	}
	s, ok := v.(lua.LString)
	if !ok {
		return 0, fmt.Errorf("%s.%s must be a duration string such as \"5s\", got %s", label, key, v.Type())
	}
	d, err := time.ParseDuration(string(s))
	if err != nil {
		return 0, fmt.Errorf("%s.%s %q is not a duration: %v", label, key, string(s), err)
	}
	if d < 0 {
		return 0, fmt.Errorf("%s.%s must not be negative, got %q", label, key, string(s))
	}
	return d, nil
}

// --- network rules ---

type networkRuleKind int

const (
	ruleHost networkRuleKind = iota
	ruleWildcard
	ruleIP
	ruleCIDR
)

// NetworkRule is one entry of a plugin's declared `network` allowlist.
//
// Four forms: an exact hostname, a `*.suffix` wildcard, an IP literal, and a
// CIDR block. Ports are deliberately not part of a rule — a host is either
// reachable or it is not, and a port-level allowlist invites the belief that it
// confines a plugin more than it does.
type NetworkRule struct {
	raw  string
	kind networkRuleKind
	host string // normalized, for ruleHost; the suffix (without "*.") for ruleWildcard
	ip   net.IP
	cidr *net.IPNet
}

// validVersion is deliberately loose: it only rejects values that cannot be a
// version at all.
var validVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*([-+][0-9A-Za-z.-]+)?$`)

var validHostname = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

func parseNetworkRule(raw string) (NetworkRule, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return NetworkRule{}, fmt.Errorf("empty entry")
	}

	if strings.Contains(s, "/") {
		_, cidr, err := net.ParseCIDR(s)
		if err != nil {
			return NetworkRule{}, fmt.Errorf("not a valid CIDR block")
		}
		// A default route as an allowlist entry says "everything", which is
		// what omitting the field already says. Its only distinct effect is to
		// satisfy the rule that private access must name what it reaches.
		//
		// Worded without naming a manifest field: this parser now serves the
		// operator flag as well, and "omit the network field" is advice about a
		// file an operator configuring -allow-private-fetch does not have.
		if ones, _ := cidrPrefix(cidr); ones == 0 {
			return NetworkRule{}, fmt.Errorf("a default route allows everything: name the addresses or blocks that are actually needed")
		}
		// ParseCIDR discards host bits, so "10.0.0.5/8" would be shown to an
		// operator as a single host and enforced as all of 10.0.0.0/8.
		if ip, _, _ := net.ParseCIDR(s); !ip.Equal(cidr.IP) {
			return NetworkRule{}, fmt.Errorf("%s has host bits set: write it as %s, or as a single address", s, canonicalCIDR(cidr))
		}
		// Shown in the family it is matched in. "::ffff:a00:0/104" is enforced
		// as 10.0.0.0/8, and an operator reading a manifest — or batch 5's
		// manage UI, which renders these strings — should not have to decode
		// that.
		return NetworkRule{raw: canonicalCIDR(cidr), kind: ruleCIDR, cidr: cidr}, nil
	}

	if strings.HasPrefix(s, "*.") {
		suffix := normalizeHost(s[2:])
		if !validHostname.MatchString(suffix) {
			return NetworkRule{}, fmt.Errorf("wildcard must be *.hostname")
		}
		// Numeric labels are legal in DNS, so "*.0.0.1" parses as a hostname
		// pattern — and read as a suffix it would cover 127.0.0.1. Matches()
		// refuses to apply a name rule to an address, but a rule that can only
		// have been meant as an address pattern should not be accepted in the
		// first place: no top-level domain is numeric.
		if looksLikeAnAddress(suffix) {
			return NetworkRule{}, fmt.Errorf("wildcard suffix must be a domain, not an address; use a CIDR block for address ranges")
		}
		return NetworkRule{raw: s, kind: ruleWildcard, host: suffix}, nil
	}

	if ip := net.ParseIP(unbracket(s)); ip != nil {
		return NetworkRule{raw: s, kind: ruleIP, ip: ip}, nil
	}

	host := normalizeHost(s)
	if !validHostname.MatchString(host) || len(host) > 253 {
		return NetworkRule{}, fmt.Errorf("not a hostname, wildcard, IP address or CIDR block")
	}
	if looksLikeAnAddress(host) {
		return NetworkRule{}, fmt.Errorf("not a hostname, wildcard, IP address or CIDR block")
	}
	return NetworkRule{raw: s, kind: ruleHost, host: host}, nil
}

// looksLikeAnAddress reports whether a name is an address in disguise.
//
// net.ParseIP is not the whole test. A resolver will happily turn "0x7f000001"
// into 127.0.0.1 and "2130706433" into the same, so a rule written in either
// form would be presented to an operator as a hostname and behave as loopback.
// A name whose last label is all digits, or any label of which is a hex
// literal, is a half-written address and is refused rather than reinterpreted.
func looksLikeAnAddress(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			continue
		}
		if strings.HasPrefix(label, "0x") || strings.HasPrefix(label, "0X") {
			if isHexDigits(label[2:]) {
				return true
			}
		}
	}
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]
	if last == "" {
		return false
	}
	return isDigits(last)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) == -1
}

func isHexDigits(s string) bool {
	if s == "" {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F')
	}) == -1
}

// normalizeHost lowercases, drops a trailing root dot and strips IPv6 brackets,
// so two spellings of one name compare equal.
//
// The lowering is ASCII-only on purpose. strings.ToLower folds U+212A (KELVIN
// SIGN) onto "k", so a host spelled with it would match a rule written with a
// plain k — a rule matching more than it says. Rules are ASCII by construction
// (validHostname), so restricting the candidate to ASCII costs nothing.
func normalizeHost(host string) string {
	h := asciiLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(h, ".")
	return unbracket(h)
}

// unbracket removes one matching pair of IPv6 brackets, and only a matching
// pair. strings.Trim(h, "[]") strips arbitrary runs from either end, so
// "[[::1]]" and even "]::1[" would both normalize to "::1" — a declared rule
// whose text says something different from the address it grants.
func unbracket(h string) string {
	if len(h) >= 2 && h[0] == '[' && h[len(h)-1] == ']' {
		return h[1 : len(h)-1]
	}
	return h
}

func asciiLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// Matches reports whether the rule covers the given request host (which may
// carry a port, brackets or a trailing dot).
//
// Address rules match addresses and name rules match names, never across. A
// host that parses as an IP is matched only by ruleIP/ruleCIDR, so no spelling
// of a literal can be caught by a domain pattern; and a name is never matched
// by an address rule, so "127.0.0.1.example.com" is not 127.0.0.1.
func (r NetworkRule) Matches(host string) bool {
	h := normalizeHost(stripPort(host))
	if h == "" {
		return false
	}
	ip := net.ParseIP(h)

	switch r.kind {
	case ruleHost:
		return ip == nil && h == r.host
	case ruleWildcard:
		if ip != nil || !validHostname.MatchString(h) {
			// validHostname also rejects the empty labels in ".fal.ai", which
			// would otherwise satisfy the suffix test below.
			return false
		}
		// A label is required: *.fal.ai covers queue.fal.ai, not fal.ai.
		return strings.HasSuffix(h, "."+r.host)
	case ruleIP:
		return ip != nil && ip.Equal(r.ip)
	case ruleCIDR:
		return ip != nil && r.cidr.Contains(ip)
	}
	return false
}

// stripPort removes a :port suffix, leaving bare IPv6 literals intact.
func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// tooBroadForPrivateHosts reports whether a rule is too coarse to be a
// meaningful statement of which private hosts a plugin may reach.
//
// Names and single addresses are always specific enough — a name resolves to
// whatever the operator's DNS says, which is their decision to make. A CIDR is
// a range, and a wide one is a way of writing "everything" without saying so:
// /8 covers 10.0.0.0/8, which is the widest real case.
func (r NetworkRule) tooBroadForPrivateHosts() (bool, string) {
	if r.kind == ruleWildcard {
		// A wildcard matches unboundedly many names, and wildcard-DNS services
		// (nip.io, sslip.io and friends) resolve an address embedded in the
		// name — so "*.nip.io" reaches loopback, RFC1918 and the metadata
		// endpoint while reading as one narrow allowlist entry. The same
		// argument that rules out a broad CIDR rules this out: it names nothing.
		return true, "a wildcard matches unboundedly many names, and wildcard-DNS services resolve any address embedded in one; name the hosts exactly"
	}
	if r.kind != ruleCIDR || r.cidr == nil {
		// Exact names and single addresses are specific enough to be a
		// statement of intent, which is all this rule asks for.
		//
		// Be precise about what a *name* is worth, though: "a name resolves to
		// whatever the operator's DNS says" is only true for a name the
		// operator controls. For a public name it resolves to whatever the
		// plugin author's DNS says, so "attacker-owned.example" with an A
		// record pointing at 169.254.169.254 satisfies this rule completely.
		// That is why the dial-time deny in batch 3 must be lifted on the
		// *resolved address* matching an address rule, never on the request
		// having matched a name — see the plan, §4.
		return false, ""
	}
	ones, bits := cidrPrefix(r.cidr)
	minimum := 8
	if bits > 32 {
		minimum = 32
	}
	if ones < minimum {
		return true, fmt.Sprintf("a /%d spans too much of the address space; name the hosts, or use a prefix of /%d or longer", ones, minimum)
	}
	return false, ""
}

// cidrPrefix reports a CIDR's prefix length in the address family Go will
// actually match it in.
//
// An IPv4-mapped IPv6 block reports its size over 128 bits — "::ffff:0:0/96"
// looks like a /96 — while net.IPNet.Contains converts and matches it as
// 0.0.0.0/0. Reading the mask literally therefore accepts the default route
// through the door built to reject it, and shows the operator a prefix that is
// not the one enforced.
func cidrPrefix(cidr *net.IPNet) (ones, bits int) {
	ones, bits = cidr.Mask.Size()
	if bits == 128 && cidr.IP.To4() != nil {
		return ones - 96, 32
	}
	return ones, bits
}

// canonicalCIDR renders a block the way it is matched: an IPv4-mapped IPv6
// prefix is an IPv4 prefix, because that is what net.IPNet.Contains compares.
func canonicalCIDR(cidr *net.IPNet) string {
	if v4 := cidr.IP.To4(); v4 != nil {
		ones, _ := cidrPrefix(cidr)
		return fmt.Sprintf("%s/%d", v4.String(), ones)
	}
	return cidr.String()
}

// canonical renders a rule in a normalized form, so two spellings of one rule
// compare and fingerprint equal.
func (r NetworkRule) canonical() string {
	switch r.kind {
	case ruleHost:
		return "host:" + r.host
	case ruleWildcard:
		return "wild:" + r.host
	case ruleIP:
		return "ip:" + r.ip.String()
	case ruleCIDR:
		return "cidr:" + r.cidr.String()
	}
	return "?:" + r.raw
}

func dedupeSorted(in []string) []string {
	out := in[:0]
	for i, s := range in {
		if i == 0 || s != in[i-1] {
			out = append(out, s)
		}
	}
	return out
}

// NetworkPolicy is the egress policy applied to one plugin's outbound requests.
//
// Unrestricted means no allowlist was declared: any *public* host is reachable.
// It never means private addresses are reachable — that needs AllowPrivate,
// which is consented separately and only relaxes the dial-time deny for hosts
// that already matched the allowlist.
type NetworkPolicy struct {
	Rules        []NetworkRule
	AllowPrivate bool
	Unrestricted bool

	// PrivateAdvice is the remediation clause appended to a dial-time refusal:
	// how *this* policy's owner is meant to permit the address, if they should.
	// Empty means a plugin's manifest, which is the only origin that existed
	// when this was written. HostFetchPolicy sets it to name the operator flag
	// instead, because telling an operator to edit `network` in a manifest is
	// wrong advice for a download the application itself started.
	PrivateAdvice string
}

// Allows reports whether the policy permits a request to this host.
func (p NetworkPolicy) Allows(host string) bool {
	if p.Unrestricted {
		return true
	}
	for _, r := range p.Rules {
		if r.Matches(host) {
			return true
		}
	}
	return false
}

// Fingerprint identifies policies that are interchangeable. Two plugins share
// an HTTP client — and therefore a connection pool — only when their
// fingerprints match, because a pooled connection carries no policy of its own.
func (p NetworkPolicy) Fingerprint() string {
	// Normalized, not raw: "Fal.Run", "fal.run." and "fal.run" are one rule, and
	// fingerprinting the input text would give three policies three connection
	// pools for no reason.
	raws := make([]string, 0, len(p.Rules))
	for _, r := range p.Rules {
		raws = append(raws, r.canonical())
	}
	sort.Strings(raws)
	raws = dedupeSorted(raws)
	// PrivateAdvice is included even though it changes no enforcement: two
	// policies that enforce identically but explain themselves differently must
	// not share a client, or a refusal would hand one origin's remediation to
	// the other's caller.
	return fmt.Sprintf("unrestricted=%t;private=%t;advice=%q;hosts=%s",
		p.Unrestricted, p.AllowPrivate, p.PrivateAdvice, strings.Join(raws, ","))
}

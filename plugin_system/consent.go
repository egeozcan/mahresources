package plugin_system

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// ErrGrantsChanged is returned when a plugin declares more than the operator
// consented to. It is a refusal to load, not a warning: the whole point of
// storing consent is that editing plugin.lua must not widen a grant.
var ErrGrantsChanged = errors.New("plugin declares more than was consented to")

// Grants is what an operator consented to when they enabled a plugin, stored in
// PluginState.GrantsJSON.
//
// It is deliberately not a Manifest. A manifest is what a file currently says;
// this is a record of a decision, and it has to survive the file changing
// underneath it. Storing the manifest here would make the two drift into each
// other and lose exactly the distinction that makes consent mean anything.
type Grants struct {
	// Legacy records that what was consented to was a plugin with no manifest
	// at all — the full mah surface. It is the maximum grant, so nothing a
	// manifest later declares exceeds it, with one exception: AllowPrivateHosts,
	// which legacy never had.
	Legacy bool `json:"legacy,omitempty"`

	APIVersion int `json:"api_version,omitempty"`

	// Capabilities is the *effective* set, after db:write implies db:read.
	// Storing the declared list instead would prompt an operator when a plugin
	// spells out an implication that was always in force.
	Capabilities []string `json:"capabilities,omitempty"`

	// Network holds rules in their canonical display form. Empty means the
	// consent was to reach any public host, which is the *broadest* policy —
	// see CompareGrants, where this field's containment test is inverted.
	Network []string `json:"network,omitempty"`

	AllowPrivateHosts bool `json:"allow_private_hosts,omitempty"`
}

// GrantsFromManifest builds the record written when a plugin is enabled.
func GrantsFromManifest(m Manifest) Grants {
	if !m.Declared {
		return Grants{Legacy: true}
	}
	return Grants{
		APIVersion:        m.APIVersion,
		Capabilities:      m.Capabilities().Sorted(),
		Network:           m.NetworkDisplay(),
		AllowPrivateHosts: m.AllowPrivateHosts,
	}
}

// NetworkDisplay renders the declared allowlist in canonical form, so a rule
// respelled between two loads compares equal instead of prompting an operator
// about a rename.
func (m Manifest) NetworkDisplay() []string {
	if len(m.rules) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.rules))
	for _, r := range m.rules {
		out = append(out, r.Display())
	}
	sort.Strings(out)
	return dedupeSorted(out)
}

// Display renders a rule the way an operator should read it.
func (r NetworkRule) Display() string {
	switch r.kind {
	case ruleHost:
		return r.host
	case ruleWildcard:
		return "*." + r.host
	case ruleIP:
		return r.ip.String()
	case ruleCIDR:
		return r.raw
	}
	return r.raw
}

// Marshal renders the record for storage.
func (g Grants) Marshal() (string, error) {
	b, err := json.Marshal(g)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ParseGrants reads a stored consent record.
//
// The second return distinguishes "no record" from "a record granting nothing",
// and the distinction decides an upgrade: absent is grandfathered (§2), while an
// empty record is a real decision and is enforced. A record that is present but
// unreadable is an error rather than an absence — grandfathering corruption
// would re-grant whatever the file now asks for, which is what anyone able to
// corrupt the column would want.
func ParseGrants(stored string) (Grants, bool, error) {
	trimmed := strings.TrimSpace(stored)
	if trimmed == "" || trimmed == "null" {
		return Grants{}, false, nil
	}

	var g Grants
	if err := json.Unmarshal([]byte(trimmed), &g); err != nil {
		return Grants{}, true, fmt.Errorf("stored consent is not readable: %w", err)
	}
	// The network entries are matched against, so an entry that cannot be
	// parsed cannot be enforced. Refusing here means such a record can never
	// silently behave as "no restriction".
	for _, entry := range g.Network {
		if _, err := canonicalNetworkKey(entry); err != nil {
			return Grants{}, true, fmt.Errorf("stored consent names an unusable network rule %q: %w", entry, err)
		}
	}
	return g, true, nil
}

// GrantDelta is the amount by which a declaration exceeds what was consented to.
// Every field is a widening; narrowings are not recorded because they load.
type GrantDelta struct {
	NewCapabilities []string
	NewNetwork      []string
	// NetworkWidened is the inverted case: consent named specific hosts and the
	// declaration now names none, which means any public host.
	NetworkWidened   bool
	PrivateHosts     bool
	APIVersionRaised bool
	// LostManifest is a plugin that deleted its api_version, jumping back to the
	// full mah surface.
	LostManifest bool
}

// Empty reports whether the declaration is covered by the consent.
func (d GrantDelta) Empty() bool {
	return len(d.NewCapabilities) == 0 &&
		len(d.NewNetwork) == 0 &&
		!d.NetworkWidened &&
		!d.PrivateHosts &&
		!d.APIVersionRaised &&
		!d.LostManifest
}

// Describe renders the delta for an operator, as the manage UI's
// "requires re-consent" line and as the load-refusal log entry.
func (d GrantDelta) Describe() string {
	var parts []string
	if d.LostManifest {
		parts = append(parts, "it no longer declares a manifest, so it is asking for the full plugin surface again (legacy mode)")
	}
	if len(d.NewCapabilities) > 0 {
		labels := make([]string, 0, len(d.NewCapabilities))
		for _, c := range d.NewCapabilities {
			if label, ok := CapabilityLabels[c]; ok {
				labels = append(labels, fmt.Sprintf("%s (%s)", c, label))
				continue
			}
			labels = append(labels, c)
		}
		parts = append(parts, "new capabilities: "+strings.Join(labels, ", "))
	}
	if d.NetworkWidened {
		parts = append(parts, "it dropped its network allowlist, so it may now reach any public host")
	}
	if len(d.NewNetwork) > 0 {
		parts = append(parts, "new network hosts: "+strings.Join(d.NewNetwork, ", "))
	}
	if d.PrivateHosts {
		parts = append(parts, "it now asks to reach private network addresses, including services on this machine")
	}
	if d.APIVersionRaised {
		parts = append(parts, "it raised its api_version")
	}
	if len(parts) == 0 {
		return "no change"
	}
	return strings.Join(parts, "; ")
}

// CompareGrants reports how far a declaration exceeds a stored consent.
//
// The containment test is not the same relation on every field, and two of them
// are not plain subset:
//
//   - Network inverts. An empty list means "any public host" — the broadest
//     policy — so a plugin that drops its list has widened to the whole
//     internet, while subset logic would read the empty list as contained.
//   - Legacy is the maximum grant, so legacy → manifest is a narrowing and
//     loads, while manifest → legacy is the largest widening there is. The one
//     thing legacy never had is AllowPrivateHosts, which therefore widens even
//     out of legacy.
func CompareGrants(consented Grants, declared Manifest) GrantDelta {
	var d GrantDelta
	declaredGrants := GrantsFromManifest(declared)

	if declaredGrants.Legacy && !consented.Legacy {
		// Everything else is subsumed: legacy grants all of it.
		d.LostManifest = true
		return d
	}

	// Checked before the legacy short-circuit below, because it is the one
	// widening that legacy consent does not already cover.
	if declaredGrants.AllowPrivateHosts && !consented.AllowPrivateHosts {
		d.PrivateHosts = true
	}

	if consented.Legacy {
		// The operator consented to the full surface. Nothing a manifest
		// declares about capabilities, hosts or api_version exceeds that.
		return d
	}

	consentedCaps := make(map[string]struct{}, len(consented.Capabilities))
	for _, c := range consented.Capabilities {
		consentedCaps[c] = struct{}{}
	}
	for _, c := range declaredGrants.Capabilities {
		if _, ok := consentedCaps[c]; !ok {
			d.NewCapabilities = append(d.NewCapabilities, c)
		}
	}
	sort.Strings(d.NewCapabilities)

	switch {
	case len(consented.Network) == 0:
		// Consent was unrestricted; no list can exceed it.
	case len(declaredGrants.Network) == 0:
		d.NetworkWidened = true
	default:
		allowed := make(map[string]struct{}, len(consented.Network))
		for _, entry := range consented.Network {
			key, err := canonicalNetworkKey(entry)
			if err != nil {
				// ParseGrants refuses these, so reaching here means a record
				// built in process. Treat it as granting nothing rather than
				// as granting everything.
				continue
			}
			allowed[key] = struct{}{}
		}
		for _, entry := range declaredGrants.Network {
			key, err := canonicalNetworkKey(entry)
			if err != nil {
				d.NewNetwork = append(d.NewNetwork, entry)
				continue
			}
			if _, ok := allowed[key]; !ok {
				d.NewNetwork = append(d.NewNetwork, entry)
			}
		}
		sort.Strings(d.NewNetwork)
	}

	if declaredGrants.APIVersion > consented.APIVersion {
		d.APIVersionRaised = true
	}

	return d
}

// ConsentStore is where an operator's consent outlives the process.
//
// It is an interface for the same reason EntityQuerier and KVStore are: the
// manager must not reach up into the layer that owns the database. The
// application_context implementation reads and writes PluginState.GrantsJSON.
type ConsentStore interface {
	// ConsentFor returns the stored record. The bool distinguishes "no record"
	// — which is grandfathered on first load — from "a record granting
	// nothing", which is enforced.
	ConsentFor(pluginName string) (Grants, bool, error)
	// RecordConsent persists what the operator agreed to.
	RecordConsent(pluginName string, granted Grants) error
}

// memoryConsentStore is the default, so a manager built without a database
// still runs the same code path rather than a skip-the-check branch.
//
// The alternative — treating an unset store as "consent disabled" — is a
// fail-open default in security code, and the one deployment that forgot to
// wire it would silently enforce nothing. Here an unwired manager behaves like
// a fresh database instead: the first load grandfathers and records, and every
// later widening in the same process is refused.
type memoryConsentStore struct {
	mu      sync.Mutex
	records map[string]Grants
}

func newMemoryConsentStore() *memoryConsentStore {
	return &memoryConsentStore{records: make(map[string]Grants)}
}

func (s *memoryConsentStore) ConsentFor(pluginName string) (Grants, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.records[pluginName]
	return g, ok, nil
}

func (s *memoryConsentStore) RecordConsent(pluginName string, granted Grants) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[pluginName] = granted
	return nil
}

// SetConsentStore installs the persistent consent store. Called once at wiring
// time, like SetEntityQuerier.
func (pm *PluginManager) SetConsentStore(store ConsentStore) {
	if store == nil {
		return
	}
	pm.consent.Store(store)
}

func (pm *PluginManager) consentStore() ConsentStore {
	if v := pm.consent.Load(); v != nil {
		if store, ok := v.(ConsentStore); ok && store != nil {
			return store
		}
	}
	return pm.fallbackConsent
}

// enforceConsent refuses a load that declares more than was consented to.
//
// The manifest passed here must be the LOAD-TIME one — the manifest the running
// VM actually declared, already proved equal to both the discovered copy and
// the live re-read — never DiscoveredPlugin.Manifest. The two can differ
// without the file changing, because the declaration is Lua that runs twice,
// and comparing against the discovered copy would check one manifest while
// installing the grants of another.
func (pm *PluginManager) enforceConsent(name string, loadTime Manifest) error {
	store := pm.consentStore()

	// Not re-wrapped with the plugin name: a real ConsentStore already names it,
	// and doubling the prefix makes the message read like two failures.
	consented, present, err := store.ConsentFor(name)
	if err != nil {
		return err
	}

	if !present {
		// Grandfather. Every row predating the GrantsJSON column reads this
		// way, and refusing them would turn the release that tightens plugin
		// security into an outage that disables every plugin in every
		// deployment. It is sound rather than merely convenient: before this
		// release those plugins ran with the entire mah surface and no egress
		// control, so recording their declared set is strictly a reduction.
		granted := GrantsFromManifest(loadTime)
		if err := store.RecordConsent(name, granted); err != nil {
			// Fail closed. If the record cannot be written, the next load
			// grandfathers again, and a widening in between would be accepted
			// silently — which is the one thing consent exists to prevent.
			return err
		}
		log.Printf("[plugin] %s: no consent on record — granting what it declares now (%s) "+
			"and recording it. Any later widening will need re-enabling.",
			name, describeGrants(granted))
		return nil
	}

	if delta := CompareGrants(consented, loadTime); !delta.Empty() {
		return fmt.Errorf("%w: %s. Re-enable %s to consent to it",
			ErrGrantsChanged, delta.Describe(), name)
	}
	return nil
}

// describeGrants renders a record for the load log.
func describeGrants(g Grants) string {
	if g.Legacy {
		return "no manifest, full access"
	}
	parts := []string{"capabilities: " + strings.Join(g.Capabilities, ", ")}
	if len(g.Network) == 0 {
		parts = append(parts, "network: any public host")
	} else {
		parts = append(parts, "network: "+strings.Join(g.Network, ", "))
	}
	if g.AllowPrivateHosts {
		parts = append(parts, "may reach private addresses")
	}
	return strings.Join(parts, "; ")
}

// canonicalNetworkKey renders one stored entry in the same normalized,
// kind-tagged form the policy fingerprint uses, so two spellings of one rule
// compare equal and a hostname never compares equal to an address.
func canonicalNetworkKey(entry string) (string, error) {
	rule, err := parseNetworkRule(entry)
	if err != nil {
		return "", err
	}
	return rule.canonical(), nil
}

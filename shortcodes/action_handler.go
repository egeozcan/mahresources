package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
)

// ActionButton is what an ActionResolver answers for an action the viewer may
// run on the current entity: the registration's label, confirmation text and
// sync/async mode, and the params the button posts, already coerced and
// validated against the registration.
type ActionButton struct {
	Label   string
	Confirm string
	Async   bool
	Params  map[string]any
}

// ActionResolver decides whether the plugin action plugin/name can run on the
// entity ctx describes, as the request's principal, and builds its button.
// params holds only the shortcode's param-<name> attributes, prefix stripped.
//
// It answers ErrActionUnavailable for every reason the viewer may not see the
// button (no such plugin or action, no access, wrong entity, filters that do
// not match), and any other error for a mistake the template author has to
// fix. It is injected per render with WithActionResolver, because the plugin
// registry and the access rule live above this package.
type ActionResolver func(plugin, name string, params map[string]string, ctx MetaShortcodeContext) (ActionButton, error)

// ErrActionUnavailable is an ActionResolver's "not for this viewer". Every
// cause renders one comment, byte for byte the one a surface with no resolver
// renders, so a page cannot be read for which plugins or actions exist or
// which ones its viewer may run. It mirrors ErrPluginUnavailable.
var ErrActionUnavailable = errors.New("action unavailable in this context")

type actionResolverKey struct{}

// WithActionResolver returns a context carrying r for [action]. Only render
// surfaces that know the viewer and the plugin registry install one: the
// process_shortcodes tag, the lightbox's JSON render and the deferred-render
// endpoint. Everywhere else [action] renders its unavailable comment.
func WithActionResolver(ctx context.Context, r ActionResolver) context.Context {
	return context.WithValue(ctx, actionResolverKey{}, r)
}

// actionResolverFrom answers nil inside a tooltip: a hover card cannot be
// entered with the keyboard, so it holds no controls.
func actionResolverFrom(ctx context.Context) ActionResolver {
	if ctx == nil || withinTooltip(ctx) {
		return nil
	}
	r, _ := ctx.Value(actionResolverKey{}).(ActionResolver)
	return r
}

// RenderActionShortcode renders [action plugin="…" name="…"]: a button that
// runs a registered plugin action on the current entity through
// POST /v1/jobs/action/run, as the person who clicks it. The server decides
// only whether to draw it; the run endpoint enforces every rule again on the
// click, so the button grants nothing a hand-made POST could not already do.
//
// It fails closed: a read-only render (share pages, principals that cannot
// write), a render that started in user-written text, a surface with no
// resolver and every refusal of the resolver all render the same comment.
func RenderActionShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext) string {
	if sc.IsBlock {
		return shortcodeErrorMarker("action", "[action] does not take a body")
	}
	// A <button> inside a <button> is invalid interactive nesting.
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("action", "[action] cannot be used inside a [reload] button face")
	}
	plugin := strings.TrimSpace(sc.Attrs["plugin"])
	name := strings.TrimSpace(sc.Attrs["name"])
	if plugin == "" || name == "" {
		return shortcodeErrorMarker("action", "[action] requires plugin= and name=")
	}

	unavailable := shortcodeComment(ErrActionUnavailable.Error())
	// UntrustedSource: a Description or note text block under -auth. A button
	// planted there by a user-role author would run with the rights of whoever
	// clicks it, behind a confirmation text that author chose.
	if ctx.ForceReadOnly || ctx.UntrustedSource {
		return unavailable
	}
	resolve := actionResolverFrom(reqCtx)
	if resolve == nil {
		return unavailable
	}

	button, err := resolve(plugin, name, collectShortcodeParams(sc.Attrs), ctx)
	if err != nil {
		if errors.Is(err, ErrActionUnavailable) {
			return unavailable
		}
		return shortcodeErrorMarker("action", "[action] "+err.Error())
	}

	label := strings.TrimSpace(sc.Attrs["label"])
	if label == "" {
		label = strings.TrimSpace(button.Label)
	}
	if label == "" {
		// The button's visible text is its accessible name, so it is never empty.
		label = name
	}
	// confirm="" counts as unset: a template can add or reword a confirmation,
	// never remove one the plugin declared.
	confirm := button.Confirm
	if c := strings.TrimSpace(sc.Attrs["confirm"]); c != "" {
		confirm = c
	}

	params := button.Params
	if params == nil {
		params = map[string]any{}
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return shortcodeErrorMarker("action", "[action] params cannot be encoded")
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<action-shortcode data-plugin="%s" data-action="%s" data-entity-type="%s" data-entity-id="%d" data-async="%s" data-label="%s"`,
		html.EscapeString(plugin), html.EscapeString(name), html.EscapeString(ctx.EntityType), ctx.EntityID,
		strconv.FormatBool(button.Async), html.EscapeString(label))
	if confirm != "" {
		fmt.Fprintf(&b, ` data-confirm="%s"`, html.EscapeString(confirm))
	}
	fmt.Fprintf(&b, ` data-params="%s">`, html.EscapeString(string(encoded)))
	fmt.Fprintf(&b, `<button type="button" class="action-shortcode-button">%s</button><span class="action-shortcode-status"></span></action-shortcode>`,
		html.EscapeString(label))
	return b.String()
}

// lintAction reports what RenderActionShortcode would refuse. Missing plugin=
// and name= are already errors through the documented required attributes.
func lintAction(tk token, reloadSpans [][2]int, actionExists func(plugin, name string) bool, add func(start, end int, severity, msg string)) {
	if insideSpans(tk.start, reloadSpans) {
		add(tk.start, tk.end, SeverityError, "[action] cannot be used inside a [reload] button face")
	}
	plugin := strings.TrimSpace(tk.attrs["plugin"])
	name := strings.TrimSpace(tk.attrs["name"])
	if actionExists != nil && plugin != "" && name != "" && !actionExists(plugin, name) {
		add(tk.start, tk.end, SeverityWarning,
			fmt.Sprintf("no action %q of plugin %q is available; this renders nothing", name, plugin))
	}
}

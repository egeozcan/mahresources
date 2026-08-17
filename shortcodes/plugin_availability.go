package shortcodes

import "errors"

// ErrPluginUnavailable is a PluginRenderer's way of saying "not for this
// caller" rather than "this failed".
//
// The distinction is visible on the page. A rendering failure prints an
// author-facing error marker naming the shortcode; this prints the same comment
// a context with no plugin renderer at all prints. That is deliberate: with
// per-plugin access, one reader may reach a plugin and another may not, and the
// page must not become a way to enumerate which plugins are installed or which
// ones a given account is allowed.
var ErrPluginUnavailable = errors.New("plugin unavailable in this context")

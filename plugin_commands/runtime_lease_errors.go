package plugin_commands

import "errors"

// ErrRuntimeLeaseBusy reports that another command runtime owns the staging
// root lease.
var ErrRuntimeLeaseBusy = errors.New("plugin command runtime lease is busy")

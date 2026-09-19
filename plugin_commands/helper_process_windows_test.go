//go:build windows

package plugin_commands

import "os/exec"

func configureDetachedProcess(*exec.Cmd) {}

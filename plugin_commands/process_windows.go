//go:build windows

package plugin_commands

import "errors"

func (nativeProcessInspector) InspectGroup(int, string) (GroupIdentity, error) {
	return GroupIdentity{State: GroupAliveUnverified}, nil
}

func (nativeProcessInspector) KillGroup(int) error {
	return errors.New("plugin command process groups are unsupported on Windows")
}

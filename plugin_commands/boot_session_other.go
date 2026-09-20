//go:build aix || dragonfly || freebsd || netbsd || openbsd || solaris || windows

package plugin_commands

func CurrentBootSessionID() (string, error) {
	return "", ErrBootSessionIDUnavailable
}

//go:build windows

package plugin_commands

import "os"

func openExchangeRunDir(string) (*os.File, error) {
	return nil, ErrExchangeUnsupported
}

func openExchangeRegularAt(*os.File, string, func(string)) (*os.File, error) {
	return nil, ErrExchangeUnsupported
}

func unlinkExchangeRegularAt(*os.File, string, func(string)) error {
	return ErrExchangeUnsupported
}

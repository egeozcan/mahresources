//go:build windows

package plugin_commands

import "os"

func openExchangeRunDir(string, string, string) (*os.File, error) {
	return nil, ErrExchangeUnsupported
}

func openExchangeRegularAt(*os.File, string, func(string)) (*os.File, error) {
	return nil, ErrExchangeUnsupported
}

func statExchangeRegularAt(*os.File, string) (os.FileInfo, error) {
	return nil, ErrExchangeUnsupported
}

func unlinkExchangeRegularAt(*os.File, string, func(string)) error {
	return ErrExchangeUnsupported
}

func unlinkExchangeOpenedRegularAt(*os.File, string, *os.File) error {
	return ErrExchangeUnsupported
}

func removeExchangeRunDir(string, string, string, func()) error {
	return ErrExchangeUnsupported
}

//go:build windows

package plugin_commands

import "errors"

func makeTestFIFO(string) error   { return errors.New("unsupported") }
func makeTestDevice(string) error { return errors.New("unsupported") }

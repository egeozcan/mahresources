//go:build darwin

package plugin_commands

import (
	"encoding/binary"
	"testing"
)

func TestDarwinProcessEnvironmentSkipsExecutableAndArgv(t *testing.T) {
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint32(raw, 2)
	raw = append(raw, []byte("/bin/tool\x00\x00\x00")...)
	raw = append(raw, []byte("tool\x00MAHR_COMMAND_RUN_ID=forged-argv\x00")...)
	raw = append(raw, []byte("PATH=/bin\x00MAHR_COMMAND_RUN_ID=owned\x00")...)
	environment, err := darwinProcessEnvironment(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(environment) != 2 || environment[0] != "PATH=/bin" || environment[1] != "MAHR_COMMAND_RUN_ID=owned" {
		t.Fatalf("environment = %#v", environment)
	}
}

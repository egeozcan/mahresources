//go:build race

package plugin_system

// raceEnabled reports a -race build. Tests that measure heap growth over
// large imports skip under it: the detector slows them several times over
// and inflates what they measure.
const raceEnabled = true

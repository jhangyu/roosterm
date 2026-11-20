package api

import "slices"

// scrubPrefix runs the program with the stale Claude/tmux env of the hc core removed: TMUX and TMUX_PANE make a
// Claude launched in the tab think it is inside a dead tmux, and CLAUDE* leaks the session that started the core.
// The filter runs inside the terminal, so it removes exactly what hc passed and keeps HC_*, TERM*, TMUX_TMPDIR.
var scrubPrefix = []string{"/bin/sh", "-c",
	`unset TMUX TMUX_PANE $(env | sed -n "s/^\(CLAUDE[A-Za-z0-9_]*\)=.*/\1/p"); exec "$@"`, "sh"}

// ScrubArgv wraps argv so the program starts without the stale Claude/tmux env.
func ScrubArgv(argv []string) []string {
	return append(append([]string{}, scrubPrefix...), argv...)
}

// unscrubbed returns the original argv of a ScrubArgv result; other argv pass through.
func unscrubbed(argv []string) []string {
	if len(argv) >= len(scrubPrefix) && slices.Equal(argv[:len(scrubPrefix)], scrubPrefix) {
		return argv[len(scrubPrefix):]
	}
	return argv
}

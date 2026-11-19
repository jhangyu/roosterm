package hc

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestSupervisorIgnoresOtherFiles(t *testing.T) {
	t.Parallel()
	const sock = "/run/user/501/herdr-core/default.sock"
	cases := []struct {
		name string
		want bool
	}{
		{sock, true},
		{"/run/user/501/herdr-core/./default.sock", true},
		{"/run/user/501/herdr-core/default.sock.tmp", false},
		{"/run/user/501/herdr-core/core.json", false},
		{"/run/user/501/herdr-core", false},
	}
	for _, c := range cases {
		if got := matchesSocket(fsnotify.Event{Name: c.name, Op: fsnotify.Create}, sock); got != c.want {
			t.Errorf("matchesSocket(%q): expected %v, got %v", c.name, c.want, got)
		}
	}
}

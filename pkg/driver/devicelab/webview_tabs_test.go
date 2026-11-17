package devicelab

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeDevTools is Chrome's DevTools HTTP endpoint on a Unix socket, as the
// ADB forward exposes it; it records the tabs it is asked to close.
func fakeDevTools(t *testing.T) (socketPath string, closed func() []string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cdp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath = filepath.Join(dir, "s")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var ids []string
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := strings.CutPrefix(r.URL.Path, "/json/close/"); ok {
			mu.Lock()
			ids = append(ids, id)
			mu.Unlock()
			_, _ = w.Write([]byte("Target is closing"))
			return
		}
		http.NotFound(w, r)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return socketPath, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), ids...)
	}
}

func pages(ids ...string) []cdpTarget {
	targets := []cdpTarget{{ID: "worker", Type: "service_worker"}}
	for _, id := range ids {
		targets = append(targets, cdpTarget{ID: id, Type: "page"})
	}
	return targets
}

// The tabs Chrome had open when the runner first read its targets are the
// device user's, not the runner's: connecting never closes them. A tab an
// openLink opened after that is closed at a later connect, as before, so the
// runner's own tabs do not pile up, and openLink's disconnect in between
// does not make the runner forget which tabs were there first. The page kept
// at the first look is one of those, so it stays open too.
func TestCloseOtherTabsLeavesTabsOpenBeforeTheFirstLook(t *testing.T) {
	socket, closed := fakeDevTools(t)
	m := newWebViewManager(nil)

	// First look: the user's three tabs and the one the flow's openLink opened.
	m.closeOtherTabs(socket, pages("11", "12", "13", "20"), "20")
	if got := closed(); len(got) != 0 {
		t.Fatalf("the first connect closed %v; the tabs open before the runner looked must stay open", got)
	}

	// Two more openLinks, each followed by openLink's disconnect.
	m.disconnect()
	m.closeOtherTabs(socket, pages("11", "12", "13", "20", "21"), "21")
	m.cleanup()
	m.closeOtherTabs(socket, pages("11", "12", "13", "20", "21", "22"), "22")

	if got, want := closed(), []string{"21"}; !reflect.DeepEqual(got, want) {
		t.Errorf("closed %v; want only %v, the tab the runner opened after its first look", got, want)
	}
}

package devicelab

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
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

// fakeChrome is Chrome on a device as the driver reaches it: a CDPForwarder
// whose forward serves Chrome's DevTools HTTP endpoint on the local socket
// (its tab list, /json/close and the page WebSocket, which it refuses, so a
// connect stops after it picked a tab), and that records what it is asked.
type fakeChrome struct {
	path string

	mu       sync.Mutex
	tabs     []string
	closed   []string
	attached []string
	srv      *http.Server
}

func newFakeChrome(t *testing.T, tabs ...string) *fakeChrome {
	t.Helper()
	dir, err := os.MkdirTemp("", "cdp")
	if err != nil {
		t.Fatal(err)
	}
	c := &fakeChrome{path: filepath.Join(dir, "s"), tabs: tabs}
	t.Cleanup(func() {
		_ = c.RemoveSocketForward(c.path)
		_ = os.RemoveAll(dir)
	})
	return c
}

func (c *fakeChrome) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.URL.Path == "/json" {
		_ = json.NewEncoder(w).Encode(pages(c.tabs...))
		return
	}
	if id, ok := strings.CutPrefix(r.URL.Path, "/json/close/"); ok {
		c.closed = append(c.closed, id)
		for i, tab := range c.tabs {
			if tab == id {
				c.tabs = append(c.tabs[:i:i], c.tabs[i+1:]...)
				break
			}
		}
		_, _ = w.Write([]byte("Target is closing"))
		return
	}
	if id, ok := strings.CutPrefix(r.URL.Path, "/devtools/page/"); ok {
		c.attached = append(c.attached, id)
	}
	http.NotFound(w, r)
}

// openTab is a tab opening on the device (an openLink's).
func (c *fakeChrome) openTab(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tabs = append(c.tabs, id)
}

func (c *fakeChrome) record() (tabs, closed, attached []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.tabs...), append([]string(nil), c.closed...), append([]string(nil), c.attached...)
}

func (c *fakeChrome) ForwardToAbstractSocket(local, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.srv != nil {
		return nil
	}
	_ = os.Remove(local)
	ln, err := net.Listen("unix", local)
	if err != nil {
		return err
	}
	c.srv = &http.Server{Handler: c}
	go func(srv *http.Server) { _ = srv.Serve(ln) }(c.srv)
	return nil
}

func (c *fakeChrome) RemoveSocketForward(string) error {
	c.mu.Lock()
	srv := c.srv
	c.srv = nil
	c.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
	return nil
}

func (c *fakeChrome) ForwardTCPToAbstractSocket(int, string) error { return nil }
func (c *fakeChrome) RemoveTCPForward(int) error                   { return nil }
func (c *fakeChrome) CDPSocketPath() string                        { return c.path }

var chromeSocket = &core.CDPInfo{Available: true, Socket: "chrome_devtools_remote"}

// When the driver closes, it closes the tabs the runner opened, the one it
// was attached to last included: left open, the next run would take them for
// tabs that were there before and never close them, so on a device nobody
// resets they would pile up run after run. The tabs that were open before
// the run stay.
func TestCloseClosesTheTabsTheRunnerOpened(t *testing.T) {
	chrome := newFakeChrome(t, "11", "12", "13")
	d := New(newTrackingClient(), &core.PlatformInfo{}, nil)
	d.SetWebViewForwarder(chrome)

	_ = d.webView.connect(chromeSocket, "browser") // the first look
	for _, tab := range []string{"20", "21"} {     // two openLinks
		chrome.openTab(tab)
		d.webView.disconnect()
		_ = d.webView.connect(chromeSocket, "browser")
	}
	d.Close()

	tabs, closed, _ := chrome.record()
	if want := []string{"11", "12", "13"}; !reflect.DeepEqual(tabs, want) {
		t.Errorf("tabs open after Close: %v (closed %v); want only the ones open before the run, %v", tabs, closed, want)
	}
}

// An openLink before the driver's first connect to Chrome opens a tab of the
// runner's, not one of the device user's: the driver looks at Chrome's tabs
// before it opens the link, so it closes that tab when it closes.
func TestOpenLinkBeforeTheFirstConnectOpensATabOfTheRunners(t *testing.T) {
	chrome := newFakeChrome(t, "11", "12", "13")
	shell := &chromeShell{chrome: chrome, tab: "20"}
	d := New(newTrackingClient(), &core.PlatformInfo{}, shell)
	d.SetWebViewForwarder(chrome)

	if res := d.openLink(&flow.OpenLinkStep{Link: "https://example.com"}); !res.Success {
		t.Fatalf("openLink failed: %v", res.Error)
	}
	_ = d.webView.connect(chromeSocket, "browser")
	d.Close()

	tabs, closed, _ := chrome.record()
	if want := []string{"11", "12", "13"}; !reflect.DeepEqual(tabs, want) {
		t.Errorf("tabs open after Close: %v (closed %v); want only the ones open before the run, %v", tabs, closed, want)
	}
}

// chromeShell is a device shell whose Chrome has its DevTools socket open,
// and on which an `am start` opens a tab.
type chromeShell struct {
	chrome *fakeChrome
	tab    string
}

func (s *chromeShell) Shell(cmd string) (string, error) {
	switch {
	case cmd == "cat /proc/net/unix":
		return "0000000000000000: 00000002 00000000 00010000 0001 01 123456 @chrome_devtools_remote\n", nil
	case strings.HasPrefix(cmd, "am start"):
		s.chrome.openTab(s.tab)
	}
	return "", nil
}

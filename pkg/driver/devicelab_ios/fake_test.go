package devicelab_ios

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// call is one command the fake agent received.
type call struct {
	cmd  string
	args Args
}

// fakeAgent answers commands from a handler and records them.
type fakeAgent struct {
	mu      sync.Mutex
	calls   []call
	handler func(cmd string, args Args) (*Response, error)
}

func (f *fakeAgent) Call(_ context.Context, cmd string, args *Args) (*Response, error) {
	a := Args{}
	if args != nil {
		a = *args
	}
	f.mu.Lock()
	f.calls = append(f.calls, call{cmd, a})
	f.mu.Unlock()
	if f.handler == nil {
		return ok(nil), nil
	}
	return f.handler(cmd, a)
}

// sent returns the recorded calls of one command.
func (f *fakeAgent) sent(cmd string) []Args {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Args
	for _, c := range f.calls {
		if c.cmd == cmd {
			out = append(out, c.args)
		}
	}
	return out
}

func ok(p *Payload) *Response { return &Response{OK: true, Data: p} }

// tree is a screen of nodes, 400x800.
func tree(nodes ...Node) *Response {
	return ok(&Payload{Nodes: nodes, ScreenW: 400, ScreenH: 800})
}

// node is an on-screen node with parent -1 unless set.
func node(i int, typ, label string, x, y, w, h float64) Node {
	return Node{I: i, P: -1, Type: typ, Label: label, X: x, Y: y, W: w, H: h, Vis: 1, Enabled: true}
}

// screenOf answers find/snapshot with nodes and everything else with ok.
func screenOf(nodes ...Node) func(string, Args) (*Response, error) {
	return func(cmd string, _ Args) (*Response, error) {
		if cmd == "find" || cmd == "snapshot" {
			return tree(nodes...), nil
		}
		return ok(&Payload{}), nil
	}
}

// simctlLog records runSimctl calls and answers from a table of prefixes.
type simctlLog struct {
	mu      sync.Mutex
	calls   []string
	answers map[string]string
	fail    map[string]error
}

func (s *simctlLog) run(args ...string) (string, error) {
	line := strings.Join(args, " ")
	s.mu.Lock()
	s.calls = append(s.calls, line)
	s.mu.Unlock()
	for prefix, err := range s.fail {
		if strings.HasPrefix(line, prefix) {
			return "", err
		}
	}
	for prefix, out := range s.answers {
		if strings.HasPrefix(line, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func (s *simctlLog) has(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// newTestDriver returns a driver on a fake agent and a fake simctl, with
// short budgets.
func newTestDriver(t *testing.T, handler func(string, Args) (*Response, error)) (*Driver, *fakeAgent, *simctlLog) {
	t.Helper()
	fa := &fakeAgent{handler: handler}
	d := NewDriver(fa, &core.PlatformInfo{Platform: "ios", IsSimulator: true, ScreenWidth: 400, ScreenHeight: 800}, "SIM-1")
	sl := &simctlLog{answers: map[string]string{}, fail: map[string]error{}}
	d.runSimctl = sl.run
	d.runSimctlWithin = func(_ time.Duration, args ...string) (string, error) { return sl.run(args...) }
	d.openWeb = func() (webPages, error) { return nil, errNoInspector }
	d.SetFindTimeout(300)
	d.SetOptionalFindTimeout(200)
	return d, fa, sl
}

var errBoom = errors.New("boom")

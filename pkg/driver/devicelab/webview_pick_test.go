package devicelab

import (
	"net"
	"testing"
	"time"
)

func TestVisiblePageIndex(t *testing.T) {
	for name, tc := range map[string]struct {
		visible []bool
		want    int
		asked   int
	}{
		"one page is not asked":         {visible: []bool{false}, want: 0, asked: 0},
		"hidden first, visible second":  {visible: []bool{false, true, true}, want: 1, asked: 2},
		"visible first":                 {visible: []bool{true, false}, want: 0, asked: 1},
		"none visible falls back first": {visible: []bool{false, false}, want: 0, asked: 2},
	} {
		asked := 0
		got := visiblePageIndex(len(tc.visible), func(i int) bool { asked++; return tc.visible[i] })
		if got != tc.want || asked != tc.asked {
			t.Errorf("%s: picked %d after %d checks, want %d after %d", name, got, asked, tc.want, tc.asked)
		}
	}
}

// A DevTools socket nobody reads must not hold a write forever.
func TestWriteBoundConnTimesOut(t *testing.T) {
	old := cdpWriteTimeout
	cdpWriteTimeout = 100 * time.Millisecond
	defer func() { cdpWriteTimeout = old }()

	client, server := net.Pipe() // a pipe write blocks until the other end reads
	defer server.Close()
	conn := &writeBoundConn{Conn: client, d: &unixDialer{}}
	start := time.Now()
	_, err := conn.Write([]byte("Runtime.evaluate"))
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("write to an unread socket: err %v after %v; want a timeout near %v", err, time.Since(start), cdpWriteTimeout)
	}

	// A setup deadline earlier than the write bound wins.
	d := &unixDialer{}
	d.limit.Store(time.Now().Add(20 * time.Millisecond).UnixNano())
	client2, server2 := net.Pipe()
	defer server2.Close()
	start = time.Now()
	if _, err := (&writeBoundConn{Conn: client2, d: d}).Write([]byte("x")); err == nil || time.Since(start) > 80*time.Millisecond {
		t.Errorf("setup deadline: err %v after %v; want a timeout near 20ms", err, time.Since(start))
	}
}

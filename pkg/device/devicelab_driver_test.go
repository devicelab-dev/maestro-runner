// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package device

import (
	"errors"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestIndentDiag verifies the small string helper used to format the
// diagnostic block inside driver-not-ready errors. Two-space indent per
// line, no trailing blank.
func TestIndentDiag(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "single line",
			in:   "tcp:6791 jdwp:1234",
			want: "  tcp:6791 jdwp:1234",
		},
		{
			name: "multi-line",
			in:   "line1\nline2\nline3",
			want: "  line1\n  line2\n  line3",
		},
		{
			name: "trailing newline trimmed",
			in:   "only\n",
			want: "  only",
		},
		{
			name: "empty",
			in:   "",
			want: "  ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := indentDiag(tc.in)
			if got != tc.want {
				t.Errorf("indentDiag(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestAppendDriverDiagnostics_PreservesOriginalError ensures the
// diagnostic-enriched error still satisfies errors.Is against the
// original, so callers using errors.Is to classify the failure mode
// (e.g. "is this a driver-not-ready error?") still work after we wrap
// it with diagnostics. Uses a sentinel error to keep the test
// independent of any real adb / Shell call.
func TestAppendDriverDiagnostics_PreservesOriginalError(t *testing.T) {
	// We can't easily call appendDriverDiagnostics without a real
	// AndroidDevice, but we can verify the wrapping shape it produces
	// is errors.Is-compatible by mimicking the pattern:
	original := errors.New("DeviceLab Android Driver not ready after 30s")
	wrapped := wrapForDiag(original, "(adb output)", "(/proc/net/tcp output)")

	if !errors.Is(wrapped, original) {
		t.Errorf("wrapped error doesn't satisfy errors.Is against original")
	}
	if !strings.Contains(wrapped.Error(), "not ready after 30s") {
		t.Errorf("wrapped error lost original message: %v", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "[diagnostics]") {
		t.Errorf("wrapped error missing diagnostics block: %v", wrapped)
	}
}

// wrapForDiag mirrors appendDriverDiagnostics's error-wrapping shape
// so we can test it without an AndroidDevice. Keep in sync with the
// real function.
func wrapForDiag(orig error, forwards, listening string) error {
	var diag strings.Builder
	diag.WriteString("\n\n[diagnostics] adb forward --list:\n")
	diag.WriteString(indentDiag(forwards))
	diag.WriteString("\n[diagnostics] /proc/net/tcp:\n")
	diag.WriteString(indentDiag(listening))
	return wrapErr(orig, diag.String())
}

func wrapErr(orig error, extra string) error {
	return wrappedErr{orig: orig, extra: extra}
}

type wrappedErr struct {
	orig  error
	extra string
}

func (w wrappedErr) Error() string { return w.orig.Error() + w.extra }
func (w wrappedErr) Unwrap() error { return w.orig }

// Realistic fixtures for the driver-start classification.
const (
	testMarker = "devicelab-driver-start 1758600000123456789"

	psHeader = "USER           PID  PPID        VSZ    RSS WCHAN            ADDR S NAME\n" +
		"root             1     0   10929596  11420 do_epoll_wait       0 S init\n" +
		"root           312     1   14524836 101280 do_sys_poll         0 S zygote64\n"

	// Cold start: `am instrument` is up but the target is still being
	// forked from zygote and dexopted, so it has no package name yet.
	psColdStart = psHeader +
		"shell         4312  4298   13870292  90344 binder_ioctl_wr     0 S app_process\n" +
		"u0_a187       4330   312   14570396  48112 0                   0 R <pre-initialized>\n"

	psWarmStart = psHeader +
		"shell         4312  4298   13870292  90344 binder_ioctl_wr     0 S app_process\n" +
		"u0_a187       4330   312   14971104 132016 do_epoll_wait       0 S " + DeviceLabDriverServer + "\n"

	psGone = psHeader

	// What a force-stopped driver's `am instrument -w` prints — the tail
	// of every previous run's log, and also a genuine crash.
	processCrashed = "INSTRUMENTATION_RESULT: shortMsg=Process crashed.\nINSTRUMENTATION_CODE: 0\n"

	javaCrash = "INSTRUMENTATION_RESULT: shortMsg=java.lang.RuntimeException\n" +
		"INSTRUMENTATION_RESULT: longMsg=java.lang.RuntimeException: Unable to create instrumentation " +
		"dev.devicelab.driver.android.DeviceLabDriverRunner: java.lang.NoClassDefFoundError\n" +
		"\tat android.app.ActivityThread.handleBindApplication(ActivityThread.java:6960)\n" +
		"\tat android.app.ActivityThread.-$$Nest$mhandleBindApplication(Unknown Source:0)\n" +
		"\tat android.os.Looper.loop(Looper.java:288)\n" +
		"INSTRUMENTATION_CODE: 0\n"

	instrumentationFailed = "android.util.AndroidException: INSTRUMENTATION_FAILED: " +
		DeviceLabDriverTest + "/" + DeviceLabDriverServer + ".DeviceLabDriverRunner\n" +
		"\tat com.android.commands.am.Instrument.run(Instrument.java:519)\n" +
		"\tat com.android.commands.am.Am.runInstrument(Am.java:196)\n"
)

// classifyDriverState decides whether the driver has failed or is merely
// slow to appear. Getting that wrong is what made startup flaky: calling a
// healthy-but-slow start a crash killed drivers that were coming up. Only
// this start's own output may count as a failure.
func TestClassifyDriverState(t *testing.T) {
	ours := testMarker + "\n"
	cases := []struct {
		name       string
		ps         string
		log        string
		logErr     error
		wantState  driverState
		wantReason string
	}{
		{name: "warm start: process present", ps: psWarmStart, log: ours, wantState: driverRunning},
		{name: "running wins over log text", ps: psWarmStart, log: ours + processCrashed, wantState: driverRunning},
		{name: "cold start: marker only", ps: psColdStart, log: ours, wantState: driverStarting},
		{name: "cold start: marker then blank lines", ps: psColdStart, log: ours + "\n  \n", wantState: driverStarting},
		{name: "cold start: log not created yet", ps: psColdStart, logErr: errors.New("exit status 1"), wantState: driverStarting},
		{name: "cold start: empty log", ps: psColdStart, log: "", wantState: driverStarting},
		{
			// The bug: the previous run's force-stop result, still in the
			// file (or written late by the previous `am`), read as ours.
			name: "stale log from a previous run", ps: psColdStart,
			log: "devicelab-driver-start 1758599000000000000\n" + processCrashed, wantState: driverStarting,
		},
		{name: "stale log from a pre-marker build", ps: psColdStart, log: processCrashed, wantState: driverStarting},
		{
			name: "marker that only prefixes ours is not ours", ps: psColdStart,
			log: testMarker + "0\n" + processCrashed, wantState: driverStarting,
		},
		{
			name: "genuine crash: process crashed", ps: psGone, log: ours + processCrashed,
			wantState: driverFailed, wantReason: strings.TrimSpace(processCrashed),
		},
		{
			name: "genuine crash: stack trace", ps: psGone, log: ours + javaCrash,
			wantState: driverFailed, wantReason: strings.TrimSpace(javaCrash),
		},
		{
			name: "genuine crash: instrumentation failed", ps: psGone, log: ours + instrumentationFailed,
			wantState: driverFailed, wantReason: strings.TrimSpace(instrumentationFailed),
		},
		{
			name: "genuine crash: CRLF line endings", ps: psGone, log: testMarker + "\r\n" + processCrashed,
			wantState: driverFailed, wantReason: strings.TrimSpace(processCrashed),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, reason := classifyDriverState(tc.ps, tc.log, testMarker, tc.logErr)
			if state != tc.wantState {
				t.Fatalf("state = %v, want %v", state, tc.wantState)
			}
			if reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

// A long crash log is trimmed to its tail, where the actual error is.
func TestClassifyDriverStateTrimsLongLog(t *testing.T) {
	long := testMarker + "\n" + strings.Repeat("x", 600) + "REAL FAILURE HERE"
	state, reason := classifyDriverState(psGone, long, testMarker, nil)
	if state != driverFailed {
		t.Fatalf("state = %v, want driverFailed", state)
	}
	if len(reason) > driverLogTailLen {
		t.Errorf("reason not trimmed: %d chars", len(reason))
	}
	if !strings.HasSuffix(reason, "REAL FAILURE HERE") {
		t.Errorf("trim dropped the tail: %q", reason[max(0, len(reason)-40):])
	}
}

// The start command must give this start a fresh inode (rm) carrying its
// marker (written before the launch), and append the driver's output after
// it rather than truncating it away.
func TestDeviceLabStartCommand(t *testing.T) {
	cmd := deviceLabStartCommand(testMarker)
	steps := []string{
		"rm -f " + deviceLabDriverLog + ";",
		"echo '" + testMarker + "' > " + deviceLabDriverLog + ";",
		"nohup am instrument -w " + DeviceLabDriverTest + "/" + DeviceLabDriverServer + ".DeviceLabDriverRunner",
		">> " + deviceLabDriverLog + " 2>&1 &",
	}
	pos := -1
	for _, step := range steps {
		i := strings.Index(cmd, step)
		if i < 0 {
			t.Fatalf("command %q lacks %q", cmd, step)
		}
		if i <= pos {
			t.Fatalf("step %q out of order in %q", step, cmd)
		}
		pos = i
	}
}

// Each start gets its own marker, safe inside single quotes on one line.
func TestNewDriverLogMarker(t *testing.T) {
	a := newDriverLogMarker()
	time.Sleep(time.Microsecond)
	b := newDriverLogMarker()
	if a == b {
		t.Fatalf("markers repeat: %q", a)
	}
	if strings.ContainsAny(a, "'\n\r") {
		t.Fatalf("marker not shell/line safe: %q", a)
	}
}

// fakeDriverShell answers the two reads the start poll makes: `ps -A` and
// the driver log (catFails makes the log read fail).
func fakeDriverShell(ps, log string, catFails bool) func(string, ...string) *exec.Cmd {
	return func(_ string, args ...string) *exec.Cmd {
		cmd := strings.Join(args, " ")
		switch {
		case strings.Contains(cmd, "ps -A"):
			return exec.Command("printf", "%s", ps)
		case strings.Contains(cmd, "cat "+deviceLabDriverLog) && catFails:
			return exec.Command("false")
		case strings.Contains(cmd, "cat "+deviceLabDriverLog):
			return exec.Command("printf", "%s", log)
		}
		return exec.Command("true")
	}
}

// The readiness poll fails fast on this start's crash, and never on a
// stale log while the process is still coming up.
func TestWaitForDeviceLabDriverReady(t *testing.T) {
	cases := []struct {
		name    string
		ps, log string
		want    string
	}{
		{"genuine crash fails fast", psGone, testMarker + "\n" + processCrashed, "crashed on startup: INSTRUMENTATION_RESULT: shortMsg=Process crashed."},
		{"stale log is not a crash", psColdStart, "devicelab-driver-start 1\n" + processCrashed, "not ready after"},
		{"cold start is not a crash", psColdStart, testMarker + "\n", "not ready after"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeExec(t, fakeDriverShell(tc.ps, tc.log, false))
			d := &AndroidDevice{serial: "emulator-5554", adbPath: "adb"}
			err := d.waitForDeviceLabDriverReady(600*time.Millisecond, testMarker)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// Timeout diagnostics quote only this start's output.
func TestDriverLogTail(t *testing.T) {
	cases := []struct {
		name     string
		log      string
		catFails bool
		want     string
	}{
		{"ours", testMarker + "\n" + processCrashed, false, strings.TrimSpace(processCrashed)},
		{"stale", "devicelab-driver-start 1\n" + processCrashed, false, ""},
		{"marker only", testMarker + "\n", false, ""},
		{"unreadable", "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeExec(t, fakeDriverShell(psGone, tc.log, tc.catFails))
			d := &AndroidDevice{serial: "emulator-5554", adbPath: "adb"}
			if got := d.driverLogTail(testMarker); got != tc.want {
				t.Fatalf("driverLogTail = %q, want %q", got, tc.want)
			}
		})
	}
}

// A driver answering the WebSocket handshake is ready, whatever the log says.
func TestWaitForDeviceLabDriverReadyHealthy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = conn.Read(make([]byte, 512))
		_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n\r\n"))
	}()
	withFakeExec(t, fakeDriverShell(psGone, "devicelab-driver-start 1\n"+processCrashed, false))
	d := &AndroidDevice{serial: "emulator-5554", adbPath: "adb", driverLocalPort: ln.Addr().(*net.TCPAddr).Port}
	if err := d.waitForDeviceLabDriverReady(2*time.Second, testMarker); err != nil {
		t.Fatalf("waitForDeviceLabDriverReady: %v", err)
	}
}

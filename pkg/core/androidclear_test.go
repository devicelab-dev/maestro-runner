package core

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// scriptedShell answers each command with the next of its replies, or with
// the last one once they run out, and records the commands.
type scriptedShell struct {
	replies  []string
	err      error
	commands []string
}

func (s *scriptedShell) Shell(cmd string) (string, error) {
	s.commands = append(s.commands, cmd)
	if s.err != nil {
		return "", s.err
	}
	reply := s.replies[len(s.replies)-1]
	if len(s.commands) <= len(s.replies) {
		reply = s.replies[len(s.commands)-1]
	}
	return reply, nil
}

func TestAndroidStopBeforeClear_WaitsForScreens(t *testing.T) {
	// The force-stop's reply, then activities and windows as `grep -c` counts.
	sh := &scriptedShell{replies: []string{"", "1\n1\n", "0\n1\n", "0\n0\n"}}
	if err := AndroidStopBeforeClear(sh, "com.example.app", time.Second, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(sh.commands) != 4 {
		t.Fatalf("ran %d commands, want the force-stop and 3 checks: %q", len(sh.commands), sh.commands)
	}
	if sh.commands[0] != "am force-stop 'com.example.app'" {
		t.Errorf("first command = %q, want the force-stop", sh.commands[0])
	}
}

func TestAndroidStopBeforeClear_TimesOut(t *testing.T) {
	sh := &scriptedShell{replies: []string{"", "1\n0\n"}}
	err := AndroidStopBeforeClear(sh, "com.example.app", 0, time.Millisecond)
	if !errors.Is(err, ErrClearWaitTimedOut) {
		t.Fatalf("err = %v, want ErrClearWaitTimedOut", err)
	}
	if !strings.Contains(err.Error(), "1 activities and 0 windows") {
		t.Errorf("error should say what was still there: %v", err)
	}
}

func TestAndroidClearWaits_UnreadableDevice(t *testing.T) {
	for name, sh := range map[string]*scriptedShell{
		"shell error":     {err: errors.New("adb gone")},
		"empty output":    {replies: []string{""}},
		"no count":        {replies: []string{"sh: dumpsys: not found\n"}},
		"too many counts": {replies: []string{"0\n0\n0\n"}},
	} {
		t.Run(name, func(t *testing.T) {
			err := AndroidWaitForTaskRemoved(sh, "com.example", time.Second, time.Millisecond)
			if err == nil || errors.Is(err, ErrClearWaitTimedOut) {
				t.Errorf("err = %v, want a read error", err)
			}
			if len(sh.commands) != 1 {
				t.Errorf("ran %d commands, want 1: a device that can't be read isn't polled", len(sh.commands))
			}
		})
	}
}

func TestAndroidWaitForTaskRemoved(t *testing.T) {
	sh := &scriptedShell{replies: []string{"2\n", "0\n"}}
	if err := AndroidWaitForTaskRemoved(sh, "com.example", time.Second, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(sh.commands) != 2 {
		t.Errorf("ran %d checks, want 2", len(sh.commands))
	}
	timedOut := AndroidWaitForTaskRemoved(&scriptedShell{replies: []string{"1\n"}}, "com.example", 0, time.Millisecond)
	if !errors.Is(timedOut, ErrClearWaitTimedOut) {
		t.Errorf("err = %v, want ErrClearWaitTimedOut", timedOut)
	}
}

// Lines in the shape API 34's dumpsys prints them, for the app com.example,
// another app whose name starts the same, and windows that aren't the app's.
const (
	activitiesWithApp = `ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)
Display #0 (activities from top to bottom):
  * Task{bd528c4 #177 type=standard A=10192:com.example U=0 visible=true visibleRequested=true mode=fullscreen translucent=false sz=2}
    * Hist  #1: ActivityRecord{80ee08b u0 com.google.android.permissioncontroller/com.android.permissioncontroller.permission.ui.GrantPermissionsActivity t177}
    * Hist  #0: ActivityRecord{7742ed2 u0 com.example/.MainActivity t177}
  * Task{bb85db7 #1 type=home U=0 visible=false visibleRequested=false mode=fullscreen translucent=true sz=1}
  mLastFocusedRootTask=Task{bd528c4 #177 type=standard A=10192:com.example}
`
	activitiesTaskOnly = `ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)
  * Task{bd528c4 #177 type=standard A=10192:com.example U=0 visible=true visibleRequested=true mode=fullscreen translucent=false sz=1}
    * Hist  #0: ActivityRecord{80ee08b u0 com.google.android.permissioncontroller/com.android.permissioncontroller.permission.ui.GrantPermissionsActivity t177}
`
	activitiesOtherApp = `ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)
  * Task{c1 #180 type=standard A=10193:com.example2 U=10 visible=true visibleRequested=true mode=fullscreen translucent=false sz=1}
    * Hist  #0: ActivityRecord{c2 u10 com.example2/.MainActivity t180}
  mLastFocusedRootTask=Task{bd528c4 #177 type=standard A=10192:com.example}
`
	windowsWithApp = `WINDOW MANAGER WINDOWS (dumpsys window windows)
  Window #9 Window{b77abb9 u0 com.example/com.example.MainActivity}:
  Window #10 Window{3f7fa8f u0 Splash Screen com.example}:
  imeLayeringTarget in display# 0 Window{b77abb9 u0 com.example/com.example.MainActivity}
`
	windowsWithoutApp = `WINDOW MANAGER WINDOWS (dumpsys window windows)
  Window #9 Window{c3 u10 com.example2/com.example2.MainActivity}:
  Window #10 Window{3f7fa8f u0 Splash Screen com.example}:
  Window #11 Window{4a u0 NavigationBar0}:
`
)

// stubShell runs commands under /bin/sh with `am` and `dumpsys` stubbed: dumpsys
// prints the fixture for its arguments, so the real grep patterns are run.
type stubShell struct {
	t   *testing.T
	dir string
}

func newStubShell(t *testing.T, activities, windows string) *stubShell {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil || runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sh")
	}
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("activities.txt", activities, 0o644)
	write("windows.txt", windows, 0o644)
	write("am", "#!/bin/sh\nexit 0\n", 0o755)
	write("dumpsys", "#!/bin/sh\ncase \"$1 $2\" in\n\"activity activities\") cat \""+dir+"/activities.txt\" ;;\n\"window windows\") cat \""+dir+"/windows.txt\" ;;\nesac\n", 0o755)
	return &stubShell{t: t, dir: dir}
}

func (s *stubShell) Shell(cmd string) (string, error) {
	c := exec.Command("sh", "-c", cmd)
	c.Env = append(os.Environ(), "PATH="+s.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// As AndroidDevice.Shell does, a non-zero exit is an error and its output
	// is dropped, so a check that leaks grep's exit status fails here too.
	out, err := c.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func TestAndroidClearWaits_Patterns(t *testing.T) {
	cases := []struct {
		name, activities, windows string
		screensGone, taskGone     bool
	}{
		{"app showing", activitiesWithApp, windowsWithApp, false, false},
		{"only another app's activity left in the app's task", activitiesTaskOnly, windowsWithoutApp, true, false},
		{"only com.example2, a splash window and a stale focus line", activitiesOtherApp, windowsWithoutApp, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := newStubShell(t, c.activities, c.windows)
			err := AndroidStopBeforeClear(sh, "com.example", 0, time.Millisecond)
			if gone := err == nil; gone != c.screensGone {
				t.Errorf("screens gone = %v (err %v), want %v", gone, err, c.screensGone)
			}
			err = AndroidWaitForTaskRemoved(sh, "com.example", 0, time.Millisecond)
			if gone := err == nil; gone != c.taskGone {
				t.Errorf("task gone = %v (err %v), want %v", gone, err, c.taskGone)
			}
		})
	}
}

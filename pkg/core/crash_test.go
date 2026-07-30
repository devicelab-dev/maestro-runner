package core

import "testing"

func TestAndroidCrashSummary(t *testing.T) {
	const pkg = "com.testhiveapp"

	jvm := `07-18 10:00:01.100  1234  1234 E AndroidRuntime: FATAL EXCEPTION: main
07-18 10:00:01.100  1234  1234 E AndroidRuntime: Process: com.testhiveapp, PID: 1234
07-18 10:00:01.100  1234  1234 E AndroidRuntime: java.lang.NullPointerException: boom`

	native := `07-18 10:00:02.000  2000  2000 F libc    : Fatal signal 11 (SIGSEGV), code 1 in tid 2000 (com.testhiveapp)
07-18 10:00:02.010  2100  2100 F DEBUG   : pid: 2000, tid: 2000, name: com.testhiveapp  >>> com.testhiveapp <<<`

	anr := `07-18 10:00:03.000  3000  3000 E ActivityManager: ANR in com.testhiveapp (com.testhiveapp/.MainActivity)`

	cases := []struct {
		name, logcat, pkg string
		wantFound         bool
		wantContains      string
	}{
		{"jvm crash", jvm, pkg, true, "NullPointerException"},
		{"native crash", native, pkg, true, "native"},
		{"anr", anr, pkg, true, "ANR"},
		{"crash for a different package", jvm, "com.other.app", false, ""},
		{"no crash", "07-18 10:00:00.000 1 1 I ActivityManager: Displayed com.testhiveapp", pkg, false, ""},
		{"empty", "", pkg, false, ""},
		{"empty pkg", jvm, "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := AndroidCrashSummary(c.logcat, c.pkg)
			if found != c.wantFound {
				t.Fatalf("found = %v, want %v (summary=%q)", found, c.wantFound, got)
			}
			if c.wantContains != "" && !contains(got, c.wantContains) {
				t.Errorf("summary %q does not contain %q", got, c.wantContains)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

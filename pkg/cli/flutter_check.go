package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// detectFlutterDebugBuild inspects an iOS .app bundle (or .ipa extracted in
// place) to decide whether it's a Flutter app built in debug mode. Flutter
// debug builds embed a Dart kernel blob at
// Frameworks/App.framework/flutter_assets/kernel_blob.bin; release and profile
// builds compile to AOT and omit it.
//
// Returns:
//   - isFlutterDebug = true when we see the kernel-blob marker
//   - reason         = human-readable explanation when isFlutterDebug=true,
//     empty string otherwise (so callers can `if reason != ""` without nil
//     handling).
//
// Errors (missing path, unreadable bundle, etc.) are swallowed and treated as
// "can't tell" — this check is advisory; it must never block a run that
// would otherwise succeed.
func detectFlutterDebugBuild(appFilePath string) (isFlutterDebug bool, reason string) {
	if appFilePath == "" {
		return false, ""
	}

	info, err := os.Stat(appFilePath)
	if err != nil || !info.IsDir() {
		// .ipa files would need unzipping; we only look at .app directories.
		// .apk handling is for future work.
		return false, ""
	}
	if !strings.HasSuffix(strings.ToLower(appFilePath), ".app") {
		return false, ""
	}

	// Path inside an iOS Flutter .app bundle that exists only for debug builds.
	// AOT builds (release / profile) compile the kernel into the App binary
	// and omit the blob.
	kernelBlob := filepath.Join(appFilePath, "Frameworks", "App.framework", "flutter_assets", "kernel_blob.bin")
	if _, err := os.Stat(kernelBlob); err == nil {
		return true, "Flutter debug build detected — Frameworks/App.framework/flutter_assets/kernel_blob.bin is present"
	}

	// Some Flutter versions ship the framework one level shallower.
	altKernelBlob := filepath.Join(appFilePath, "flutter_assets", "kernel_blob.bin")
	if _, err := os.Stat(altKernelBlob); err == nil {
		return true, "Flutter debug build detected — flutter_assets/kernel_blob.bin is present"
	}

	return false, ""
}

// warnIfFlutterDebugBuild prints a one-time warning at startup when the
// configured --app-file looks like a Flutter debug build. The warning
// explains why the run is likely to fail and points at the fix.
//
// Advisory only — never returns an error. Standalone debug builds may still
// happen to work in unusual setups (live `flutter run` daemon reachable from
// the test host), so we don't block.
func warnIfFlutterDebugBuild(appFilePath string) {
	isDebug, reason := detectFlutterDebugBuild(appFilePath)
	if !isDebug {
		return
	}

	fmt.Fprintf(os.Stderr,
		"  %s⚠%s  %s\n"+
			"     Flutter debug builds need a live `flutter run` daemon to reach\n"+
			"     the Dart VM service. Launched standalone, the app starts, fails\n"+
			"     to connect, and terminates — typically producing a port-forward\n"+
			"     flood in WDA logs.\n\n"+
			"     Rebuild for testing:\n"+
			"       flutter build ios --release --no-codesign\n"+
			"       flutter build ios --profile --no-codesign   # release + VM service\n\n",
		color(colorYellow), color(colorReset), reason)
}

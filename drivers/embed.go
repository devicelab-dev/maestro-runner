// Package drivers embeds the device-driver source trees shipped inside
// the maestro-runner Go module. Library consumers (DeviceDeck and
// friends) build the runner from these sources, so the runner they run
// is version-locked to the Go client they compiled against — instead of
// depending on whatever maestro-runner installation happens to exist on
// the machine, where protocol skew between an old installed runner and
// a newer pinned client would fail in confusing ways at runtime.
package drivers

import "embed"

// IOSRunnerSource is the DevicelabIOSRunner Xcode project source tree.
// Extracted and compiled on demand by devicelab_ios.EnsureBuiltEmbedded;
// ~316K of Swift/Obj-C/project text, so embedding costs little.
//
//go:embed all:ios/DevicelabIOSRunner
var IOSRunnerSource embed.FS

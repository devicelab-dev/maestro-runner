package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/device"
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
	"github.com/urfave/cli/v2"
)

var devicesCommand = &cli.Command{
	Name:  "devices",
	Usage: "List the devices, emulators and simulators this machine can see",
	Description: `List everything maestro-runner could run on right now.

Android devices and emulators come from adb; iOS simulators from simctl and
physical iOS devices over usbmux. Only booted simulators are listed by default,
since a Mac typically has dozens that are shut down — pass --all for those too.

The --json form is stable and suited to scripts and CI. Restrict the listing to
one platform with the global --platform flag.

Examples:
  maestro-runner devices
  maestro-runner devices --all
  maestro-runner devices --json
  maestro-runner -p android devices`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "Include simulators that are shut down",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "Emit JSON instead of a table",
		},
	},
	Action: runDevices,
}

// DeviceEntry is one row of `maestro-runner devices`. The JSON tags are a
// public contract — scripts and CI read this shape.
type DeviceEntry struct {
	Platform  string `json:"platform"` // android | ios
	Kind      string `json:"kind"`     // device | emulator | simulator
	ID        string `json:"id"`       // adb serial or iOS UDID — what --device takes
	Name      string `json:"name,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
	State     string `json:"state"` // device | offline | unauthorized | Booted | Shutdown
	// Ready is true when a run could target this entry as-is. An offline or
	// unauthorized Android device and a shut-down simulator are all listed —
	// knowing they exist is the point — but none of them is runnable yet.
	Ready bool `json:"ready"`
}

func runDevices(c *cli.Context) error {
	platform := strings.ToLower(globalString(c, "platform"))
	entries := collectDevices(platform, c.Bool("all"))

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		// Never emit `null` for an empty listing — an empty array keeps
		// `| jq '.[]'` and `length` working without a special case.
		if entries == nil {
			entries = []DeviceEntry{}
		}
		return enc.Encode(entries)
	}

	fmt.Print(formatDeviceTable(entries, platform))
	return nil
}

// collectDevices gathers every device the host can see. Discovery failures are
// deliberately silent: a machine without adb, or without Xcode, is a normal
// state for a listing command, not an error — the empty result plus the hint in
// formatDeviceTable says more than a stack of tool-not-found errors would.
func collectDevices(platform string, includeShutdown bool) []DeviceEntry {
	var entries []DeviceEntry

	if platform == "" || platform == "android" {
		entries = append(entries, androidEntries()...)
	}
	if (platform == "" || platform == "ios") && runtime.GOOS == "darwin" {
		entries = append(entries, simulatorEntries(includeShutdown)...)
		entries = append(entries, physicalIOSEntries()...)
	}
	return entries
}

func androidEntries() []DeviceEntry {
	found, err := device.ListDevices()
	if err != nil {
		return nil
	}
	entries := make([]DeviceEntry, 0, len(found))
	for _, d := range found {
		entries = append(entries, DeviceEntry{
			Platform: "android",
			Kind:     d.Type,
			ID:       d.Serial,
			State:    d.State,
			Ready:    d.State == "device",
		})
	}
	return entries
}

func simulatorEntries(includeShutdown bool) []DeviceEntry {
	sims, err := simulator.ListIOSSimulators()
	if err != nil {
		return nil
	}
	var entries []DeviceEntry
	for _, s := range sims {
		booted := s.State == "Booted"
		if !booted && !includeShutdown {
			continue
		}
		entries = append(entries, DeviceEntry{
			Platform:  "ios",
			Kind:      "simulator",
			ID:        s.UDID,
			Name:      s.Name,
			OSVersion: s.OSVersion,
			State:     s.State,
			Ready:     booted,
		})
	}
	// Booted first, then by name, so the runnable ones are at the top of a
	// long --all listing.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Ready != entries[j].Ready {
			return entries[i].Ready
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

func physicalIOSEntries() []DeviceEntry {
	udids, err := listPhysicalIOSUDIDs()
	if err != nil {
		return nil
	}
	entries := make([]DeviceEntry, 0, len(udids))
	for _, udid := range udids {
		entry := DeviceEntry{
			Platform: "ios",
			Kind:     "device",
			ID:       udid,
			State:    "connected",
			Ready:    true,
		}
		// Name and OS version need a lockdown handshake, which fails on a
		// locked or untrusted phone. That is worth reporting rather than
		// hiding: the UDID alone is enough to run, but "trust this computer"
		// is the most common reason a connected phone won't work.
		if info, infoErr := getPhysicalDeviceInfo(udid); infoErr == nil {
			entry.Name = info.Name
			entry.OSVersion = info.OSVersion
		} else {
			entry.State = "connected (not paired — unlock the device and trust this computer)"
			entry.Ready = false
		}
		entries = append(entries, entry)
	}
	return entries
}

// formatDeviceTable renders entries grouped by platform. Kept free of I/O so
// the layout can be tested directly.
func formatDeviceTable(entries []DeviceEntry, platform string) string {
	if len(entries) == 0 {
		return noDevicesHint(platform)
	}

	var b strings.Builder
	for _, group := range []struct {
		title    string
		platform string
	}{
		{"Android", "android"},
		{"iOS", "ios"},
	} {
		rows := filterByPlatform(entries, group.platform)
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s\n", group.title)
		idWidth, nameWidth := 0, 0
		for _, r := range rows {
			idWidth = max(idWidth, len(r.ID))
			nameWidth = max(nameWidth, len(r.Name))
		}
		for _, r := range rows {
			marker := " "
			if r.Ready {
				marker = "●"
			}
			fmt.Fprintf(&b, "  %s %-*s  %-*s  %-9s  %s\n",
				marker, idWidth, r.ID, nameWidth, r.Name, r.Kind, describeState(r))
		}
		b.WriteString("\n")
	}
	b.WriteString("● = ready to run. Target one with --device <id>.\n")
	return b.String()
}

func describeState(r DeviceEntry) string {
	if r.OSVersion == "" {
		return r.State
	}
	label := "Android"
	if r.Platform == "ios" {
		label = "iOS"
	}
	return fmt.Sprintf("%s (%s %s)", r.State, label, r.OSVersion)
}

func filterByPlatform(entries []DeviceEntry, platform string) []DeviceEntry {
	var out []DeviceEntry
	for _, e := range entries {
		if e.Platform == platform {
			out = append(out, e)
		}
	}
	return out
}

// noDevicesHint explains what to do next rather than just reporting nothing,
// since "no devices" is the most common first-run state and the fix differs
// per platform.
func noDevicesHint(platform string) string {
	var b strings.Builder
	b.WriteString("No devices found.\n\n")
	if platform == "" || platform == "android" {
		b.WriteString("Android:\n")
		b.WriteString("  - Connect a device with USB debugging on, then check `adb devices`\n")
		b.WriteString("  - Or start an emulator, or pass --auto-start-emulator to a run\n")
	}
	if (platform == "" || platform == "ios") && runtime.GOOS == "darwin" {
		b.WriteString("iOS:\n")
		b.WriteString("  - Boot a simulator, or run `maestro-runner devices --all` to see the shut-down ones\n")
		b.WriteString("  - Or connect an iPhone, unlock it, and trust this computer\n")
	}
	b.WriteString("\nRun `maestro-runner doctor` to check the toolchain itself.\n")
	return b.String()
}

// globalString reads a global flag that may have been given before the
// subcommand (`maestro-runner -p ios devices`) or after it. urfave/cli keeps
// those in the parent context, so both spellings have to be checked.
func globalString(c *cli.Context, name string) string {
	if c.IsSet(name) {
		return c.String(name)
	}
	if lineage := c.Lineage(); len(lineage) > 1 && lineage[1] != nil {
		return lineage[1].String(name)
	}
	return c.String(name)
}

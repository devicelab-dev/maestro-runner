package devicelab_ios

import "testing"

func TestParseSimulatorProcesses(t *testing.T) {
	ps := []byte(`  101 /Applications/Xcode.app/Contents/MacOS/Xcode
  202 /Users/r/Library/Developer/CoreSimulator/Devices/U1/data/Containers/Bundle/Application/AA/DevicelabIOSAgentUITests-Runner.app/DevicelabIOSAgentUITests-Runner
  303 /Users/r/Library/Developer/CoreSimulator/Devices/U1/data/Containers/Bundle/Application/BB/My App.app/My App -flag
  404 /Users/r/Library/Developer/CoreSimulator/Devices/U2/data/Containers/Bundle/Application/CC/Other.app/Other
  505 /Users/r/Library/Developer/CoreSimulator/Devices/U1/data/Library/launchd_sim
  606 /bin/zsh -c grep /Users/r/Library/Developer/CoreSimulator/Devices/U1/data/Containers/Bundle/Application/
`)
	got := parseSimulatorProcesses(ps, "U1")
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].pid != 202 || got[0].name != "DevicelabIOSAgentUITests-Runner" {
		t.Errorf("agent: %+v", got[0])
	}
	if got[1].pid != 303 || got[1].name != "My" {
		t.Errorf("app: %+v", got[1])
	}
}

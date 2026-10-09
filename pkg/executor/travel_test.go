package executor

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// recordLocations returns a driver that records every setLocation it is asked
// for, failing from call number failAt (1-based; 0 never fails).
func recordLocations(calls *[]string, failAt int) *mockDriver {
	return &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		s, ok := step.(*flow.SetLocationStep)
		if !ok {
			return &core.CommandResult{Success: false, Message: "unexpected step " + step.Describe()}
		}
		*calls = append(*calls, s.Latitude+","+s.Longitude)
		if failAt > 0 && len(*calls) == failAt {
			return &core.CommandResult{Success: false, Error: errors.New("denied"), Message: "mock location denied"}
		}
		return &core.CommandResult{Success: true}
	}}
}

func TestExecuteTravel_WalksEachLegInFiftySteps(t *testing.T) {
	var calls []string
	fr := &FlowRunner{ctx: context.Background(), driver: recordLocations(&calls, 0)}

	res := fr.executeTravel(&flow.TravelStep{Points: []string{"0, 0", "1, 0", "1, 2"}, Speed: 1e9})
	if !res.Success {
		t.Fatalf("travel failed: %s", res.Message)
	}
	if len(calls) != 1+2*travelLegSteps {
		t.Fatalf("setLocation called %d times, want %d", len(calls), 1+2*travelLegSteps)
	}
	for i, want := range map[int]string{0: "0,0", 25: "0.5,0", 50: "1,0", 75: "1,1", 100: "1,2"} {
		if calls[i] != want {
			t.Errorf("call %d = %s, want %s", i, calls[i], want)
		}
	}
}

func TestExecuteTravel_BadPointFailsBeforeMoving(t *testing.T) {
	for _, pts := range [][]string{nil, {"0,0", "abc"}, {"0,0", "1,2,3"}, {"0,0", "91,0"}} {
		var calls []string
		fr := &FlowRunner{ctx: context.Background(), driver: recordLocations(&calls, 0)}
		res := fr.executeTravel(&flow.TravelStep{Points: pts})
		if res.Success || len(calls) != 0 {
			t.Errorf("points %q: success=%v after %d setLocation calls; want a failure before any", pts, res.Success, len(calls))
		}
	}
}

func TestExecuteTravel_SetLocationFailureStopsTravel(t *testing.T) {
	var calls []string
	fr := &FlowRunner{ctx: context.Background(), driver: recordLocations(&calls, 3)}
	res := fr.executeTravel(&flow.TravelStep{Points: []string{"0,0", "1,1"}, Speed: 1e9})
	if res.Success || len(calls) != 3 {
		t.Fatalf("success=%v after %d calls; want a failure at call 3", res.Success, len(calls))
	}
	if !strings.Contains(res.Message, "mock location denied") {
		t.Errorf("message %q does not carry the driver's reason", res.Message)
	}
}

func TestExecuteTravel_CancelStopsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls []string
	fr := &FlowRunner{ctx: ctx, driver: recordLocations(&calls, 0)}
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	res := fr.executeTravel(&flow.TravelStep{Points: []string{"0,0", "1,0"}}) // default speed: minutes
	if res.Success || time.Since(start) > time.Second {
		t.Errorf("success=%v after %v; want a prompt failure on cancel", res.Success, time.Since(start))
	}
}

// Maestro's leg length, which its pauses are timed by: a 0.1° leg comes out
// at about 194m, so the default 4 m/s takes about 48s.
func TestMaestroLegMeters(t *testing.T) {
	got := maestroLegMeters(geoPoint{0, 0}, geoPoint{0.1, 0})
	if math.Abs(got-194.06) > 0.1 {
		t.Errorf("maestroLegMeters(0.1° of latitude) = %.2f, want ~194.06", got)
	}
}

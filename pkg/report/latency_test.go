package report

import "testing"

func ms(v int64) *int64 { return &v }

func TestComputeStepLatency(t *testing.T) {
	t.Run("summarises a flat command list", func(t *testing.T) {
		cmds := []Command{
			{Type: "tapOn", Duration: ms(10)},
			{Type: "inputText", Duration: ms(20)},
			{Type: "assertVisible", Duration: ms(30)},
			{Type: "launchApp", Duration: ms(40)},
		}
		got := ComputeStepLatency(cmds)

		if got.Count != 4 {
			t.Errorf("Count = %d, want 4", got.Count)
		}
		if got.Mean != 25 {
			t.Errorf("Mean = %d, want 25", got.Mean)
		}
		if got.Max != 40 {
			t.Errorf("Max = %d, want 40", got.Max)
		}
		if got.SlowestType != "launchApp" {
			t.Errorf("SlowestType = %q, want launchApp", got.SlowestType)
		}
		// Nearest rank: p50 of 4 samples is the 2nd, p95 the 4th.
		if got.P50 != 20 {
			t.Errorf("P50 = %d, want 20", got.P50)
		}
		if got.P95 != 40 {
			t.Errorf("P95 = %d, want 40", got.P95)
		}
	})

	// A repeat/runFlow container's duration is the sum of its children, so
	// counting it too would double-count the work and skew the percentiles
	// toward the containers.
	t.Run("counts leaves, not containers", func(t *testing.T) {
		cmds := []Command{
			{Type: "repeat", Duration: ms(1000), SubCommands: []Command{
				{Type: "tapOn", Duration: ms(10)},
				{Type: "tapOn", Duration: ms(30)},
			}},
			{Type: "assertVisible", Duration: ms(20)},
		}
		got := ComputeStepLatency(cmds)

		if got.Count != 3 {
			t.Errorf("Count = %d, want 3 — the repeat container must not be counted", got.Count)
		}
		if got.Max != 30 {
			t.Errorf("Max = %d, want 30 — the 1000ms container is not a step", got.Max)
		}
		if got.SlowestType == "repeat" {
			t.Error("SlowestType must name a leaf command, not a container")
		}
	})

	// A skipped command has no duration; treating it as 0 would silently pull
	// every percentile down and mask a real slowdown.
	t.Run("skips commands with no duration", func(t *testing.T) {
		cmds := []Command{
			{Type: "tapOn", Duration: ms(50)},
			{Type: "tapOn"},
			{Type: "tapOn", Duration: ms(70)},
		}
		got := ComputeStepLatency(cmds)

		if got.Count != 2 {
			t.Errorf("Count = %d, want 2", got.Count)
		}
		if got.Mean != 60 {
			t.Errorf("Mean = %d, want 60 — a missing duration is not a zero", got.Mean)
		}
	})

	t.Run("empty input yields a zero summary", func(t *testing.T) {
		if got := ComputeStepLatency(nil); got.Count != 0 || got.P95 != 0 {
			t.Errorf("got %+v, want the zero value", got)
		}
	})

	t.Run("single sample reports itself at every percentile", func(t *testing.T) {
		got := ComputeStepLatency([]Command{{Type: "tapOn", Duration: ms(42)}})
		if got.P50 != 42 || got.P95 != 42 || got.Max != 42 || got.Mean != 42 {
			t.Errorf("got %+v, want 42 throughout", got)
		}
	})
}

func TestPercentileNearestRank(t *testing.T) {
	sorted := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	tests := []struct {
		p    int
		want int64
	}{
		{50, 5},
		{95, 10},
		{100, 10},
		{1, 1},
		// Guard the bounds: a rank below 1 or above n must clamp, not panic.
		{0, 1},
	}
	for _, tt := range tests {
		if got := percentile(sorted, tt.p); got != tt.want {
			t.Errorf("percentile(p%d) = %d, want %d", tt.p, got, tt.want)
		}
	}

	// Every reported percentile must be a value that was actually observed.
	for _, p := range []int{10, 25, 50, 75, 90, 95, 99} {
		got := percentile(sorted, p)
		var found bool
		for _, v := range sorted {
			if v == got {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("percentile(p%d) = %d, which is not an observed sample", p, got)
		}
	}
}

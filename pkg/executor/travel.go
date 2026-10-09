package executor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// travelLegSteps is how many locations each leg between two points is walked
// through, as Maestro does.
const travelLegSteps = 50

// defaultTravelSpeed is Maestro's speed when the step gives none, in metres
// per second.
const defaultTravelSpeed = 4.0

type geoPoint struct {
	lat, lon float64
}

// executeTravel moves the device along the step's points with setLocation,
// so it works on every driver that can set the location. Like Maestro, it sets
// the first point, then walks each leg in 50 even steps, pausing between them
// for the leg's length divided by speed (metres per second).
//
// The drivers used to do this themselves: two Android drivers read speed as
// km/h, paused 3600/speed whole seconds at each point whatever the distance
// (72s a point by default, none at all above 3600), and skipped points they
// could not read; the iOS drivers had no travel at all.
func (fr *FlowRunner) executeTravel(step *flow.TravelStep) *core.CommandResult {
	points, err := parseTravelPoints(step.Points)
	if err != nil {
		return &core.CommandResult{Success: false, Error: err, Message: err.Error()}
	}
	speed := step.Speed
	if speed <= 0 {
		speed = defaultTravelSpeed
	}

	if res := fr.travelSetLocation(points[0]); res != nil {
		return res
	}
	for i := 1; i < len(points); i++ {
		start, end := points[i-1], points[i]
		pause := time.Duration(maestroLegMeters(start, end)/speed*1000) * time.Millisecond / travelLegSteps
		for s := 1; s <= travelLegSteps; s++ {
			f := float64(s) / travelLegSteps
			p := geoPoint{start.lat + (end.lat-start.lat)*f, start.lon + (end.lon-start.lon)*f}
			if res := fr.travelSetLocation(p); res != nil {
				return res
			}
			if res := fr.travelPause(pause); res != nil {
				return res
			}
		}
	}
	return &core.CommandResult{Success: true, Message: fmt.Sprintf("Traveled through %d points", len(points))}
}

func (fr *FlowRunner) travelSetLocation(p geoPoint) *core.CommandResult {
	res := fr.driver.Execute(&flow.SetLocationStep{
		BaseStep:  flow.BaseStep{StepType: flow.StepSetLocation},
		Latitude:  strconv.FormatFloat(p.lat, 'f', -1, 64),
		Longitude: strconv.FormatFloat(p.lon, 'f', -1, 64),
	})
	if res == nil || res.Success {
		return nil
	}
	msg := fmt.Sprintf("travel: setting the location to %v,%v failed: %s", p.lat, p.lon, res.Message)
	return &core.CommandResult{Success: false, Error: res.Error, Message: msg}
}

func (fr *FlowRunner) travelPause(d time.Duration) *core.CommandResult {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-fr.ctx.Done():
		return &core.CommandResult{Success: false, Error: fr.ctx.Err(), Message: "travel interrupted"}
	}
}

// parseTravelPoints reads "latitude, longitude" points. A point that cannot be
// read fails the step: skipping it would travel a different route.
func parseTravelPoints(raw []string) ([]geoPoint, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("travel: no points given")
	}
	points := make([]geoPoint, 0, len(raw))
	for _, r := range raw {
		parts := strings.Split(r, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("travel: point %q is not \"latitude, longitude\"", r)
		}
		lat, latErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lon, lonErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if latErr != nil || lonErr != nil || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
			return nil, fmt.Errorf("travel: point %q is not \"latitude, longitude\" in range", r)
		}
		points = append(points, geoPoint{lat, lon})
	}
	return points, nil
}

// maestroLegMeters is the leg length Maestro times its pauses by. Its
// haversine converts degrees to radians twice, so the result is about 1/57 of
// the real distance and travel runs about 57 times faster than speed says.
// Flows tuned on Maestro expect that timing (a 0.1° leg at the default speed
// takes about 48s there, where the true distance would take 46 minutes), so
// it is reproduced here rather than corrected.
func maestroLegMeters(a, b geoPoint) float64 {
	rad := func(deg float64) float64 { return deg * math.Pi / 180 }
	oLat, oLon := rad(a.lat), rad(a.lon)
	aLat, aLon := rad(b.lat), rad(b.lon)
	dLat, dLon := rad(aLat-oLat), rad(aLon-oLon)
	x := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(oLat))*math.Cos(rad(aLat))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(x), math.Sqrt(1-x))
	return 6371 * c * 1000
}

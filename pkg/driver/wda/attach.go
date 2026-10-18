package wda

import (
	"fmt"
	"net/http"
	"time"
)

// AttachTimeout is how long --wda-attach waits for the running WebDriverAgent
// to answer. A variable so tests can shorten it.
var AttachTimeout = 30 * time.Second

// attachPollInterval is how often WaitForRunning asks; a variable so tests can
// shorten it.
var attachPollInterval = time.Second

// attachProbeTimeout bounds one /status probe, so a WDA that takes the
// connection and never answers cannot hold the wait past its timeout (the
// client's own timeout is a minute). A variable so tests can shorten it.
var attachProbeTimeout = 5 * time.Second

// WaitForRunning waits until the WebDriverAgent behind c answers /status, or
// timeout passes. A WDA that was just started takes a few seconds to listen,
// so one failed probe is not an answer.
func WaitForRunning(c *Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		probeTimeout := attachProbeTimeout
		if left := time.Until(deadline); left > 0 && left < probeTimeout {
			probeTimeout = left
		}
		probe := &Client{baseURL: c.baseURL, httpClient: &http.Client{Timeout: probeTimeout}}
		_, err := probe.Status()
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("no WDA answers at %s after %v: %w", c.baseURL, timeout, err)
		}
		time.Sleep(attachPollInterval)
	}
}

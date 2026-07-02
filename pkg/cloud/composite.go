// Package cloud — composite provider.
//
// Composite fans every Provider hook out to several providers at once, so a
// run can drive a real cloud provider (Sauce Labs, …) AND auxiliary providers
// (e.g. the session exporter) simultaneously, instead of Detect()'s single
// first-match winner. One member's failure never blocks the others — errors are
// aggregated and returned together.
package cloud

import (
	"errors"
	"strings"
)

type composite struct{ members []Provider }

// Composite returns a Provider that fans out to all non-nil members.
// Returns nil if no members, or the sole member if only one (no wrapper cost).
func Composite(providers ...Provider) Provider {
	var m []Provider
	for _, p := range providers {
		if p != nil {
			m = append(m, p)
		}
	}
	switch len(m) {
	case 0:
		return nil
	case 1:
		return m[0]
	default:
		return &composite{members: m}
	}
}

func (c *composite) Name() string {
	names := make([]string, len(c.members))
	for i, p := range c.members {
		names[i] = p.Name()
	}
	return strings.Join(names, "+")
}

func (c *composite) ExtractMeta(sessionID string, caps map[string]interface{}, meta map[string]string) {
	for _, p := range c.members {
		p.ExtractMeta(sessionID, caps, meta)
	}
}

func (c *composite) OnRunStart(meta map[string]string, totalFlows int) error {
	var errs []error
	for _, p := range c.members {
		errs = append(errs, p.OnRunStart(meta, totalFlows))
	}
	return errors.Join(errs...)
}

func (c *composite) OnFlowStart(meta map[string]string, flowIdx, totalFlows int, name, file string) error {
	var errs []error
	for _, p := range c.members {
		errs = append(errs, p.OnFlowStart(meta, flowIdx, totalFlows, name, file))
	}
	return errors.Join(errs...)
}

func (c *composite) OnFlowEnd(meta map[string]string, result *FlowResult) error {
	var errs []error
	for _, p := range c.members {
		errs = append(errs, p.OnFlowEnd(meta, result))
	}
	return errors.Join(errs...)
}

func (c *composite) ReportResult(appiumURL string, meta map[string]string, result *TestResult) error {
	var errs []error
	for _, p := range c.members {
		errs = append(errs, p.ReportResult(appiumURL, meta, result))
	}
	return errors.Join(errs...)
}

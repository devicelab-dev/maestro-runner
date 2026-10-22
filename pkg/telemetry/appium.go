package telemetry

import (
	"net"
	"net/url"
	"strings"
)

// appiumVendors maps the domain of a cloud's Appium hub to the name reported
// for it. The hosts, from each vendor's docs (2026-10):
//
//	saucelabs     ondemand.<region>.saucelabs.com
//	browserstack  hub-cloud.browserstack.com, hub.browserstack.com
//	lambdatest    mobile-hub.lambdatest.com, hub(.eu|.ap).lambdatest.com; renamed TestMu AI in 2026
//	perfecto      <cloud>.perfectomobile.com/nexperience/perfectomobile/wd/hub
//	kobiton       api.kobiton.com (on-prem portals use their own domain)
//	headspin      <region>.headspin.io:<port>/v0/<token>/wd/hub
//	testingbot    hub.testingbot.com
//	digitalai     cloud.seetest.io, <tenant>.experitest.com
//	aws           devicefarm-interactive-global.<region>.api.aws (Device Farm remote access)
//
// The telemetry Worker accepts only these names, "local" and "other", and
// rejects the whole event on any other value, so a vendor is added there
// before it is added here. BitBar (bitbar.com), pCloudy (pcloudy.com) and
// TestGrid (testgrid.io) report "other" until then.
var appiumVendors = []struct {
	domain, vendor string
}{
	{"saucelabs.com", "saucelabs"},
	{"browserstack.com", "browserstack"},
	{"lambdatest.com", "lambdatest"},
	{"testmuai.com", "lambdatest"},
	{"perfectomobile.com", "perfecto"},
	{"kobiton.com", "kobiton"},
	{"headspin.io", "headspin"},
	{"testingbot.com", "testingbot"},
	{"seetest.io", "digitalai"},
	{"experitest.com", "digitalai"},
	{"digital.ai", "digitalai"},
}

// AppiumVendor names where the Appium server at rawURL runs: a cloud from
// appiumVendors, "aws" for Device Farm, "local" for localhost or any IP
// address, and "other" for any other server. It is "" when there is no URL.
// Only the host is read, and never sent: nor are credentials, port, path or
// query.
func AppiumVendor(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "other"
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil {
		return "local"
	}
	for _, v := range appiumVendors {
		if host == v.domain || strings.HasSuffix(host, "."+v.domain) {
			return v.vendor
		}
	}
	if strings.Contains(host, "devicefarm") && (strings.HasSuffix(host, ".api.aws") || strings.HasSuffix(host, ".amazonaws.com")) {
		return "aws"
	}
	return "other"
}

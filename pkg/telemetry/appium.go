package telemetry

import (
	"net"
	"net/url"
	"regexp"
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
//	bitbar        cloud.bitbar.com
//	pcloudy       device.pcloudy.com, <region>.pcloudy.com /appiumcloud/wd/hub
//	testgrid      <tenant>.testgrid.io
//	aws           devicefarm-interactive-global.<region>.api.aws (Device Farm remote access)
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
	{"bitbar.com", "bitbar"},
	{"pcloudy.com", "pcloudy"},
	{"testgrid.io", "testgrid"},
}

// hostnamePattern is a DNS host name: dot-separated labels of letters, digits
// and hyphens. Anything else is never sent.
var hostnamePattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// AppiumVendor names where the Appium server at rawURL runs: a cloud from
// appiumVendors, "aws" for Device Farm, "local" for localhost or any IP
// address, and otherwise the server's host name, lowercased ("other" when it
// is not a plain DNS name). It is "" when there is no URL. Only the host is
// read: credentials, port, path and query never are.
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
	if len(host) > 253 || !hostnamePattern.MatchString(host) {
		return "other"
	}
	for _, v := range appiumVendors {
		if host == v.domain || strings.HasSuffix(host, "."+v.domain) {
			return v.vendor
		}
	}
	if strings.Contains(host, "devicefarm") && (strings.HasSuffix(host, ".api.aws") || strings.HasSuffix(host, ".amazonaws.com")) {
		return "aws"
	}
	return host
}

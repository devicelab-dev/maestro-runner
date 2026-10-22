package telemetry

import "testing"

func TestAppiumVendor(t *testing.T) {
	tests := []struct{ url, want string }{
		{"", ""},
		{"https://user:key@ondemand.us-west-1.saucelabs.com/wd/hub", "saucelabs"},
		{"https://user:key@ondemand.eu-central-1.saucelabs.com:443/wd/hub", "saucelabs"},
		{"https://user:key@hub-cloud.browserstack.com/wd/hub", "browserstack"},
		{"HTTPS://HUB.BROWSERSTACK.COM./wd/hub", "browserstack"},
		{"https://user:key@mobile-hub.lambdatest.com/wd/hub", "lambdatest"},
		{"https://user:key@hub.eu.lambdatest.com/wd/hub", "lambdatest"},
		{"https://demo.perfectomobile.com/nexperience/perfectomobile/wd/hub", "perfecto"},
		{"https://user:key@api.kobiton.com/wd/hub", "kobiton"},
		{"https://us-nyc.headspin.io:7016/v0/token/wd/hub", "headspin"},
		{"https://key:secret@hub.testingbot.com/wd/hub", "testingbot"},
		{"https://cloud.seetest.io/wd/hub", "digitalai"},
		{"https://acme.experitest.com/wd/hub", "digitalai"},
		{"https://cloud.bitbar.com/wd/hub", "other"},
		{"https://device.pcloudy.com/appiumcloud/wd/hub", "other"},
		{"https://ind-west.pcloudy.com/appiumcloud/wd/hub", "other"},
		{"https://acme.testgrid.io/quantumgrid/wd/hub", "other"},
		{"https://devicefarm-interactive-global.us-west-2.api.aws/remote-endpoint/ABCD", "aws"},
		{"http://localhost:4723", "local"},
		{"http://127.0.0.1:4723/wd/hub", "local"},
		{"http://[::1]:4723", "local"},
		{"http://10.0.0.5:4723", "local"},
		{"http://52.12.34.56:4723", "local"},
		{"http://ec2-1-2-3-4.compute-1.amazonaws.com:4723", "other"},
		{"https://user:key@appium.acme-corp.internal:4723/wd/hub?x=1", "other"},
		{"https://saucelabs.com.evil.example/wd/hub", "other"},
		{"https://notsaucelabs.com/wd/hub", "other"},
		{"http://under_score.example.com", "other"},
		{"::not a url", "other"},
	}
	for _, tt := range tests {
		if got := AppiumVendor(tt.url); got != tt.want {
			t.Errorf("AppiumVendor(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestBuild_AppiumVendorOnlyForAppium(t *testing.T) {
	url := "https://user:key@hub-cloud.browserstack.com/wd/hub"
	if got := Build(Run{Command: "test", Driver: "appium", AppiumURL: url}, nil, "").AppiumVendor; got != "browserstack" {
		t.Errorf("appium run: AppiumVendor = %q, want browserstack", got)
	}
	if got := Build(Run{Command: "test", Driver: "devicelab", AppiumURL: url}, nil, "").AppiumVendor; got != "" {
		t.Errorf("devicelab run: AppiumVendor = %q, want empty", got)
	}
}

// The telemetry Worker rejects an event whose appium_vendor is not on its
// list (maestro-runner-telemetry src/event.js), so every name sent must be.
func TestAppiumVendor_OnlyWorkerValues(t *testing.T) {
	worker := map[string]bool{
		"saucelabs": true, "browserstack": true, "lambdatest": true, "perfecto": true, "kobiton": true,
		"headspin": true, "testingbot": true, "digitalai": true, "aws": true, "local": true, "other": true,
	}
	for _, v := range appiumVendors {
		if !worker[v.vendor] {
			t.Errorf("vendor %q (for %s) is not accepted by the Worker", v.vendor, v.domain)
		}
	}
}

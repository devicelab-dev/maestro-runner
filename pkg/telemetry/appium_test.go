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
		{"https://cloud.bitbar.com/wd/hub", "bitbar"},
		{"https://device.pcloudy.com/appiumcloud/wd/hub", "pcloudy"},
		{"https://ind-west.pcloudy.com/appiumcloud/wd/hub", "pcloudy"},
		{"https://acme.testgrid.io/quantumgrid/wd/hub", "testgrid"},
		{"https://devicefarm-interactive-global.us-west-2.api.aws/remote-endpoint/ABCD", "aws"},
		{"http://localhost:4723", "local"},
		{"http://127.0.0.1:4723/wd/hub", "local"},
		{"http://[::1]:4723", "local"},
		{"http://10.0.0.5:4723", "local"},
		{"http://52.12.34.56:4723", "local"},
		{"http://ec2-1-2-3-4.compute-1.amazonaws.com:4723", "ec2-1-2-3-4.compute-1.amazonaws.com"},
		{"https://user:key@appium.acme-corp.internal:4723/wd/hub?x=1", "appium.acme-corp.internal"},
		{"https://saucelabs.com.evil.example/wd/hub", "saucelabs.com.evil.example"},
		{"https://notsaucelabs.com/wd/hub", "notsaucelabs.com"},
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

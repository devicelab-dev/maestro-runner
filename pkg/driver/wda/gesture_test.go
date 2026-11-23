package wda

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Every coordinate gesture goes out with snapshotMaxDepth at 1 and the
// session's depth set again after it. A refused depth leaves the gesture as it
// was, and a refused restore is tried once more.
func TestGesturesRunAtDepthOne(t *testing.T) {
	gestures := map[string]func(c *Client) error{
		"/wda/tap":                   func(c *Client) error { return c.Tap(1, 2) },
		"/wda/doubleTap":             func(c *Client) error { return c.DoubleTap(1, 2) },
		"/wda/touchAndHold":          func(c *Client) error { return c.LongPress(1, 2, 1) },
		"/actions":                   func(c *Client) error { return c.Swipe(1, 2, 3, 4, 0) },
		"/wda/dragfromtoforduration": func(c *Client) error { return c.DragFromTo(1, 2, 3, 4, 1) },
	}
	cases := []struct {
		name   string
		refuse func(depth string, tries int) bool // which settings calls WebDriverAgent refuses
		want   string
	}{
		{"accepted", func(string, int) bool { return false }, "depth=1 GESTURE depth=62"},
		{"depth refused", func(d string, _ int) bool { return d == "1" }, "depth=1! GESTURE"},
		{"restore refused once", func(d string, tries int) bool { return d == "62" && tries == 1 }, "depth=1 GESTURE depth=62! depth=62"},
	}
	for path, gesture := range gestures {
		for _, tc := range cases {
			t.Run(tc.name+" "+path, func(t *testing.T) {
				var got []string
				restores := 0
				server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
					if !strings.HasSuffix(r.URL.Path, "/appium/settings") {
						got = append(got, strings.TrimPrefix(r.URL.Path, "/session/s"))
						jsonResponse(w, map[string]interface{}{"value": nil})
						return
					}
					var body struct {
						Settings map[string]json.Number `json:"settings"`
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					depth := body.Settings["snapshotMaxDepth"].String()
					if depth == "62" {
						restores++
					}
					if tc.refuse(depth, restores) {
						got = append(got, "depth="+depth+"!")
						w.WriteHeader(http.StatusInternalServerError)
						jsonResponse(w, map[string]interface{}{"value": map[string]string{"error": "unknown error", "message": "no"}})
						return
					}
					got = append(got, "depth="+depth)
					jsonResponse(w, map[string]interface{}{"value": nil})
				})
				defer server.Close()
				client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "s"}
				if err := gesture(client); err != nil {
					t.Fatal(err)
				}
				want := strings.ReplaceAll(tc.want, "GESTURE", path)
				if strings.Join(got, " ") != want {
					t.Errorf("requests %q, want %q", strings.Join(got, " "), want)
				}
			})
		}
	}
}

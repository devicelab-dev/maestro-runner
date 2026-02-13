package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const updateCheckURL = "https://open.devicelab.dev/api/maestro-runner/updates"

// updateNotice receives the update message from the background check.
var updateNotice = make(chan string, 1)

type updateResponse struct {
	LatestVersion string `json:"latest_version"`
}

// startUpdateCheck kicks off a background update check.
// Call printUpdateNotice() later to print the result.
func startUpdateCheck() {
	ch := updateNotice
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}

		req, err := http.NewRequest("GET", updateCheckURL, nil)
		if err != nil {
			ch <- ""
			return
		}

		req.Header.Set("User-Agent", "maestro-runner")

		resp, err := client.Do(req)
		if err != nil {
			ch <- ""
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			ch <- ""
			return
		}

		var result updateResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ch <- ""
			return
		}

		if result.LatestVersion != "" && result.LatestVersion != Version {
			ch <- fmt.Sprintf("\n  Update available: %s → %s\n  Run: curl -fsSL https://open.devicelab.dev/maestro-runner/install | bash\n", Version, result.LatestVersion)
		} else {
			ch <- ""
		}
	}()
}

// printUpdateNotice prints the update message if one is available.
func printUpdateNotice() {
	select {
	case msg := <-updateNotice:
		if msg != "" {
			fmt.Print(msg)
		}
	default:
		// Check not finished yet, don't block
	}
}

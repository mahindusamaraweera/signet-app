package handlers

import (
	"encoding/json"
	"testing"
)

func TestFlexString(t *testing.T) {
	for in, want := range map[string]string{
		`{"package_id": 812}`:   "812", // what the dashboards send
		`{"package_id": "812"}`: "812",
		`{"package_id": null}`:  "",
		`{}`:                    "",
	} {
		var body struct {
			PackageID flexString `json:"package_id"`
		}
		if err := json.Unmarshal([]byte(in), &body); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if string(body.PackageID) != want {
			t.Errorf("%s: got %q, want %q", in, body.PackageID, want)
		}
	}
}

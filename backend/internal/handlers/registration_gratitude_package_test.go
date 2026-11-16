package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"signet-backend/internal/models"
)

func TestMarkGratitudeBlockedPackageOnlyFlagsBlockedPackage(t *testing.T) {
	choices := []purchasePackageChoice{
		{Package: models.Package{ID: 1}},
		{Package: models.Package{ID: gratitudeBlockedPackageID}},
		{Package: models.Package{ID: 3}},
	}
	markGratitudeBlockedPackage(choices)
	for _, c := range choices {
		want := c.ID == gratitudeBlockedPackageID
		if c.Unavailable != want {
			t.Errorf("package %d: unavailable = %v, want %v", c.ID, c.Unavailable, want)
		}
		if want && c.UnavailableReason == "" {
			t.Errorf("package %d: missing unavailable reason", c.ID)
		}
	}
}

func TestPackageChoiceOmitsUnavailableFieldsWhenSelectable(t *testing.T) {
	raw, err := json.Marshal(purchasePackageChoice{Package: models.Package{ID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "unavailable") {
		t.Fatalf("selectable package leaked unavailable fields: %s", raw)
	}
}

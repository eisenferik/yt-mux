package urlguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type urlFixture struct {
	Allowed  []string `json:"allowed"`
	Rejected []struct {
		URL string `json:"url"`
		Why string `json:"why"`
	} `json:"rejected"`
}

func loadURLFixture(t *testing.T) urlFixture {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "youtube-urls.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture urlFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Allowed) == 0 || len(fixture.Rejected) == 0 {
		t.Fatalf("the shared URL fixture is empty: %#v", fixture)
	}
	return fixture
}

func TestNormalizeAllowedURLs(t *testing.T) {
	for _, raw := range loadURLFixture(t).Allowed {
		if _, err := Normalize(raw); err != nil {
			t.Errorf("expected %q to be allowed: %v", raw, err)
		}
	}
}

func TestNormalizeRejectedURLs(t *testing.T) {
	for _, testCase := range loadURLFixture(t).Rejected {
		if _, err := Normalize(testCase.URL); err == nil {
			t.Errorf("expected %q to be rejected: %s", testCase.URL, testCase.Why)
		}
	}
}

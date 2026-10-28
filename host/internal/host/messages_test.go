package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func requestTarget(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var head header
	if err := json.Unmarshal(raw, &head); err != nil {
		t.Fatal(err)
	}
	switch head.Type {
	case "settings.get":
		return &settingsGetRequest{}
	case "settings.set":
		return &settingsSetRequest{}
	case "cancel":
		return &cancelRequest{}
	case "start":
		return &startRequest{}
	case "destination.pick":
		return &destinationPickRequest{}
	}
	t.Fatalf("the fixture names a request the host does not handle: %q", head.Type)
	return nil
}

func TestSharedProtocolMessagesDecodeStrictly(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "protocol-messages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Accepted []struct {
			Name    string          `json:"name"`
			Message json.RawMessage `json:"message"`
		} `json:"accepted"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range fixture.Accepted {
		t.Run(testCase.Name, func(t *testing.T) {
			if err := decodeStrict(testCase.Message, requestTarget(t, testCase.Message)); err != nil {
				t.Fatalf("the host must decode a request the extension sends: %v", err)
			}
		})
	}
}

package host

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	return New(writeTestConfiguration(t), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
}

func TestSettingsSaveRepliesWithTheSavedSettings(t *testing.T) {
	app := newTestApp(t)
	destination := filepath.Join(t.TempDir(), "saved")
	request, err := json.Marshal(map[string]any{
		"v": 1, "type": "settings.set", "id": "save1",
		"settings": map[string]any{
			"destination": destination + `\.\`, "defaultPreset": "archive",
			"subtitleLanguages": []string{"en"}, "cookiesEnabled": false, "notifications": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	outcome := app.handleRequest(request, nil)
	message, ok := outcome.reply.(settingsSavedMessage)
	if outcome.fatal != nil || !ok {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if message.Settings.Destination != destination {
		t.Fatalf("reply carries %q instead of the saved destination %q", message.Settings.Destination, destination)
	}
}

func TestSettingsSaveReportsInvalidSettingsWithoutStoppingTheHost(t *testing.T) {
	app := newTestApp(t)
	outcome := app.handleRequest(json.RawMessage(`{"v":1,"type":"settings.set","id":"request1","settings":{"destination":"Videos","defaultPreset":"archive","subtitleLanguages":[],"cookiesEnabled":false,"notifications":true}}`), nil)

	message, ok := outcome.reply.(errorMessage)
	if outcome.fatal != nil || !ok || message.Kind != ErrorKindInvalidRequest || !strings.Contains(message.Detail, "destination") {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

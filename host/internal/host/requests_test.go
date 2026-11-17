package host

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func revealOutcome(t *testing.T, app *App, path string) (requestOutcome, []string) {
	t.Helper()
	var revealed []string
	app.launch = func(command *exec.Cmd) error {
		revealed = append(revealed, command.SysProcAttr.CmdLine)
		return nil
	}
	request, err := json.Marshal(map[string]any{"v": 1, "type": "reveal", "id": "reveal1", "path": path})
	if err != nil {
		t.Fatal(err)
	}
	return app.handleRequest(request, nil), revealed
}

func TestRevealOpensTheSavedFile(t *testing.T) {
	root := writeTestConfiguration(t)
	app := New(root, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	saved := filepath.Join(root, "clip.mkv")
	if err := os.WriteFile(saved, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	outcome, revealed := revealOutcome(t, app, saved)

	if _, ok := outcome.reply.(launchedMessage); !ok || outcome.fatal != nil {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if len(revealed) != 1 || !strings.HasSuffix(revealed[0], ` /select,"`+saved+`"`) {
		t.Fatalf("revealed %q instead of %q", revealed, saved)
	}
}

func TestRevealReportsAFileThatIsGone(t *testing.T) {
	root := writeTestConfiguration(t)
	app := New(root, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})

	outcome, _ := revealOutcome(t, app, filepath.Join(root, "gone.mkv"))

	message, ok := outcome.reply.(errorMessage)
	if outcome.fatal != nil || !ok || message.Kind != ErrorKindSavedFileMissing {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestUpdateStartsTheInstalledUpdateScriptInItsOwnWindow(t *testing.T) {
	root := writeTestConfiguration(t)
	app := New(root, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	var started []*exec.Cmd
	app.launch = func(command *exec.Cmd) error {
		started = append(started, command)
		return nil
	}

	outcome := app.handleRequest(json.RawMessage(`{"v":1,"type":"update","id":"update1"}`), nil)

	if _, ok := outcome.reply.(launchedMessage); !ok || outcome.fatal != nil {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if len(started) != 1 || !slices.Equal(started[0].Args[len(started[0].Args)-2:], []string{filepath.Join(root, "update.ps1"), "-FromNotification"}) {
		t.Fatalf("unexpected command: %#v", started)
	}
}

func TestSettingsReplyCarriesTheInstalledExtensionVersion(t *testing.T) {
	root := writeTestConfiguration(t)
	if err := os.MkdirAll(filepath.Join(root, "extension"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "extension", "manifest.json"), []byte(`{"version":"1.2.3"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := New(root, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})

	outcome := app.handleRequest(json.RawMessage(`{"v":1,"type":"settings.get","id":"request1"}`), nil)

	if message, ok := outcome.reply.(settingsMessage); !ok || message.Version != "1.2.3" {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

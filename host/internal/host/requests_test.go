package host

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eisenferik/yt-mux/host/internal/config"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	return New(writeTestConfiguration(t), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
}

func savedSettings(t *testing.T, app *App) config.Settings {
	t.Helper()
	settings, err := config.LoadSettings(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	return settings
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

func TestStartRejectsCookiesWhileCookiesAreDisabled(t *testing.T) {
	app := newTestApp(t)
	if savedSettings(t, app).CookiesEnabled {
		t.Fatal("this test requires cookies to be disabled in settings")
	}
	raw := json.RawMessage(`{"v":1,"type":"start","id":"request1",` +
		`"url":"https://www.youtube.com/watch?v=abc","preset":"archive",` +
		`"cookies":[{"domain":".youtube.com","name":"SID","value":"secret","path":"/","secure":true}]}`)

	outcome := app.handleRequest(raw, nil)
	if outcome.operation != nil {
		t.Fatal("expected no operation to start")
	}
	message, ok := outcome.reply.(errorMessage)
	if !ok || message.Kind != ErrorKindInvalidRequest || !strings.Contains(message.Detail, "cookies are disabled") {
		t.Fatalf("unexpected rejection: %#v", outcome.reply)
	}
	if outcome.fatal == nil || !strings.Contains(outcome.fatal.Error(), "cookies are disabled") {
		t.Fatal("expected the rejection to terminate the host")
	}
}

func TestStartReturnsAcceptedReplyAndDeferredOperation(t *testing.T) {
	app := newTestApp(t)
	outcome := app.handleRequest(json.RawMessage(
		`{"v":1,"type":"start","id":"request1",`+
			`"url":"https://www.youtube.com/watch?v=abc","preset":"archive"}`,
	), nil)

	if outcome.fatal != nil {
		t.Fatal(outcome.fatal)
	}
	message, ok := outcome.reply.(acceptedMessage)
	if !ok || message.ID != "request1" {
		t.Fatalf("unexpected acceptance: %#v", outcome.reply)
	}
	if outcome.operation == nil || outcome.operation.id != "request1" {
		t.Fatalf("expected a deferred operation plan: %#v", outcome.operation)
	}
}

func TestHandleRequestRejections(t *testing.T) {
	const validURL = "https://www.youtube.com/watch?v=abc"
	cases := []struct {
		name     string
		raw      string
		active   bool
		contains string
		fatal    bool
		reply    bool
	}{
		{
			name:     "unsupported preset",
			raw:      `{"v":1,"type":"start","id":"request1","url":"` + validURL + `","preset":"arbitrary"}`,
			contains: "unsupported preset",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "unsupported protocol version",
			raw:      `{"v":2,"type":"start","id":"request1","url":"` + validURL + `","preset":"archive"}`,
			contains: "unsupported protocol version",
			fatal:    true,
		},
		{
			name:     "unsupported request type",
			raw:      `{"v":1,"type":"exec","id":"request1"}`,
			contains: "unsupported request type",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "invalid request ID",
			raw:      `{"v":1,"type":"settings.get","id":"../escape"}`,
			contains: "request ID is invalid",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "second start while an operation runs",
			raw:      `{"v":1,"type":"start","id":"request2","url":"` + validURL + `","preset":"archive"}`,
			active:   true,
			contains: "already running",
			reply:    true,
		},
		{
			name:     "settings.set while an operation runs",
			raw:      `{"v":1,"type":"settings.set","id":"request2","settings":{}}`,
			active:   true,
			contains: "already running",
			reply:    true,
		},
		{
			name:     "malformed settings.get payload",
			raw:      `{"v":1,"type":"settings.get","id":"request1","extra":true}`,
			contains: "required schema",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "malformed settings.set payload",
			raw:      `{"v":1,"type":"settings.set","id":"request1","settings":"none"}`,
			contains: "required schema",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "malformed destination.pick payload",
			raw:      `{"v":1,"type":"destination.pick","id":"request1","start":42}`,
			contains: "required schema",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "malformed start payload",
			raw:      `{"v":1,"type":"start","id":"request1","url":"` + validURL + `","preset":"archive","extra":1}`,
			contains: "required schema",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "malformed cancel payload",
			raw:      `{"v":1,"type":"cancel","id":"request1","extra":null}`,
			active:   true,
			contains: "required schema",
			fatal:    true,
			reply:    true,
		},
		{
			name:     "trailing data after a request",
			raw:      `{"v":1,"type":"cancel","id":"request1"}{"v":1}`,
			active:   true,
			contains: "invalid request envelope",
			fatal:    true,
		},
		{
			name:     "invalid settings",
			raw:      `{"v":1,"type":"settings.set","id":"request1","settings":{}}`,
			contains: "destination",
			reply:    true,
		},
		{
			name:     "cancel with a mismatched ID",
			raw:      `{"v":1,"type":"cancel","id":"request2"}`,
			active:   true,
			contains: "no matching operation",
			reply:    true,
		},
		{
			name:     "cancel without an operation",
			raw:      `{"v":1,"type":"cancel","id":"request1"}`,
			contains: "no matching operation",
			reply:    true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			app := newTestApp(t)
			var active *operation
			if testCase.active {
				active = &operation{id: "request1", cancel: func() {}}
			}

			outcome := app.handleRequest(json.RawMessage(testCase.raw), active)
			if outcome.operation != nil {
				t.Fatal("expected no operation to start")
			}
			if (outcome.fatal != nil) != testCase.fatal {
				t.Fatalf("fatal: got %t, want %t", outcome.fatal != nil, testCase.fatal)
			}
			if testCase.fatal && !strings.Contains(outcome.fatal.Error(), testCase.contains) {
				t.Fatalf("expected fatal error containing %q: %v", testCase.contains, outcome.fatal)
			}
			message, hasReply := outcome.reply.(errorMessage)
			if hasReply != testCase.reply {
				t.Fatalf("error reply: got %t, want %t", hasReply, testCase.reply)
			}
			if hasReply {
				if message.Kind != ErrorKindInvalidRequest || !strings.Contains(message.Detail, testCase.contains) {
					t.Fatalf("unexpected rejection: %#v", message)
				}
			} else if outcome.fatal == nil || !strings.Contains(outcome.fatal.Error(), testCase.contains) {
				t.Fatalf("expected fatal error containing %q: %v", testCase.contains, outcome.fatal)
			}
		})
	}
}

func TestCancelStopsTheMatchingOperation(t *testing.T) {
	app := newTestApp(t)
	cancelled := false
	active := &operation{id: "request1", cancel: func() { cancelled = true }}

	outcome := app.handleRequest(
		json.RawMessage(`{"v":1,"type":"cancel","id":"request1"}`),
		active,
	)
	if outcome.fatal != nil {
		t.Fatal(outcome.fatal)
	}
	if outcome.reply != nil || outcome.operation != nil {
		t.Fatal("cancel must not replace the running operation")
	}
	if !cancelled {
		t.Fatal("expected the running operation to be cancelled")
	}
}

package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eisenferik/yt-mux/host/internal/config"
	"github.com/eisenferik/yt-mux/host/internal/cookies"
	"github.com/eisenferik/yt-mux/host/internal/protocol"
	"github.com/eisenferik/yt-mux/host/internal/tail"
	"github.com/eisenferik/yt-mux/host/internal/ytdlp"
)

const (
	fakeYTDLPEnv    = "YT_MUX_FAKE_YTDLP"
	fakeStartedEnv  = "YT_MUX_FAKE_YTDLP_STARTED"
	fakeJobTotal    = 364511204
	fakeStreamTotal = 362000000
	fakeDownloaded  = 45381721
	floatEpsilon    = 1e-9
)

func TestMain(m *testing.M) {
	switch os.Getenv(fakeYTDLPEnv) {
	case "done":
		fmt.Printf("@TOTAL@%d\n", fakeJobTotal)
		fmt.Printf("@P@%d|%d|8412000|38|299\n", fakeDownloaded, fakeStreamTotal)
		fmt.Println(`@DONE@C:\Videos\clip.mkv`)
		os.Exit(0)
	case "postprocess":
		fmt.Fprintln(os.Stderr, "@PP@started")
		fmt.Fprintln(os.Stderr, "@PP@finished")
		fmt.Println(`@DONE@C:\Videos\clip.mkv`)
		os.Exit(0)
	case "missing-path":
		os.Exit(0)
	case "long-line":
		fmt.Println(strings.Repeat("x", childLineBufferLimit+1))
		fmt.Println(`@DONE@C:\Videos\clip.mkv`)
		os.Exit(0)
	case "hang":
		if path := os.Getenv(fakeStartedEnv); path != "" {
			if err := os.WriteFile(path+".tmp", []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
				os.Exit(1)
			}
			if err := os.Rename(path+".tmp", path); err != nil {
				os.Exit(1)
			}
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestAppReturnsSettingsOverNativeFraming(t *testing.T) {
	root := writeTestConfiguration(t)
	var input bytes.Buffer
	if err := protocol.NewWriter(&input).Send(settingsGetRequest{Version: 1, Type: "settings.get", ID: "request1"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := New(root, &input, &output, &bytes.Buffer{})
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	var message settingsMessage
	readMessage(t, protocol.NewReader(&output), &message)
	if message.Type != "settings" || message.ID != "request1" || message.Settings.DefaultPreset != "archive" {
		t.Fatalf("unexpected settings response: %#v", message)
	}
}

func TestAppReportsBrokenSettingsAsHostFailure(t *testing.T) {
	root := writeTestConfiguration(t)
	var input bytes.Buffer
	if err := protocol.NewWriter(&input).Send(settingsGetRequest{Version: 1, Type: "settings.get", ID: "request1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "settings.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := New(root, &input, &output, &bytes.Buffer{})

	if err := app.Run(); err == nil {
		t.Fatal("expected broken settings to terminate the host")
	}
	var message errorMessage
	readMessage(t, protocol.NewReader(&output), &message)
	if message.Kind != ErrorKindHostFailure || message.ID != "request1" || message.Detail != "" {
		t.Fatalf("unexpected host failure response: %#v", message)
	}
}

func TestAppDoesNotRetryADeadResponseWriter(t *testing.T) {
	root := writeTestConfiguration(t)
	var input bytes.Buffer
	if err := protocol.NewWriter(&input).Send(settingsGetRequest{Version: 1, Type: "settings.get", ID: "request1"}); err != nil {
		t.Fatal(err)
	}
	output := &rejectingWriter{}
	app := New(root, &input, output, &bytes.Buffer{})

	if err := app.Run(); err == nil {
		t.Fatal("expected the response write to fail")
	}
	if output.writes != 1 {
		t.Fatalf("response writes: got %d, want 1", output.writes)
	}
}

func TestAppRejectsDisallowedURLBeforeStartingAProcess(t *testing.T) {
	root := writeTestConfiguration(t)
	var input bytes.Buffer
	request := startRequest{
		Version: 1,
		Type:    "start",
		ID:      "request1",
		URL:     "https://evil.example/video",
		Preset:  "archive",
	}
	if err := protocol.NewWriter(&input).Send(request); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := New(root, &input, &output, &bytes.Buffer{})
	if err := app.Run(); err == nil {
		t.Fatal("expected invalid request to terminate the host")
	}
	var message errorMessage
	readMessage(t, protocol.NewReader(&output), &message)
	if message.Kind != ErrorKindInvalidRequest || message.ID != "request1" {
		t.Fatalf("unexpected error response: %#v", message)
	}
}

func TestAppReportsAMissingDownloaderAfterAccepting(t *testing.T) {
	reader := runStartedDownload(t, writeTestConfiguration(t))
	expectAcceptedThenError(t, reader, ErrorKindProcessStartFailed)
}

func TestAppReportsProgressAndDoneFromChildOutput(t *testing.T) {
	t.Setenv(fakeYTDLPEnv, "done")
	reader := runStartedDownload(t, writeTestConfigurationWithFakeYTDLP(t))

	var accepted acceptedMessage
	readMessage(t, reader, &accepted)
	if accepted.Type != "accepted" || accepted.ID != "request1" || accepted.Version != ProtocolVersion {
		t.Fatalf("unexpected acceptance: %#v", accepted)
	}
	var progress progressMessage
	readMessage(t, reader, &progress)
	if progress.Type != "progress" || progress.ID != "request1" || progress.Version != ProtocolVersion {
		t.Fatalf("unexpected progress: %#v", progress)
	}
	if progress.Downloaded == nil || *progress.Downloaded != fakeDownloaded {
		t.Fatalf("unexpected downloaded bytes: %#v", progress.Downloaded)
	}
	wantPercent := float64(fakeDownloaded) * 100 / float64(fakeJobTotal)
	if progress.Percent == nil || math.Abs(*progress.Percent-wantPercent) > floatEpsilon {
		t.Fatalf("the percentage must come from the job total: %#v, want %v", progress.Percent, wantPercent)
	}
	var done doneMessage
	readMessage(t, reader, &done)
	if done.Type != "done" || done.ID != "request1" || done.Path != `C:\Videos\clip.mkv` {
		t.Fatalf("unexpected done response: %#v", done)
	}
}

func TestAppReportsPostprocessingAnnouncedOnTheChildErrorStream(t *testing.T) {
	t.Setenv(fakeYTDLPEnv, "postprocess")
	reader := runStartedDownload(t, writeTestConfigurationWithFakeYTDLP(t))

	var accepted acceptedMessage
	readMessage(t, reader, &accepted)
	var postprocess postprocessMessage
	readMessage(t, reader, &postprocess)
	if postprocess.Type != "postprocess" || postprocess.ID != "request1" {
		t.Fatalf("unexpected postprocess response: %#v", postprocess)
	}
	var done doneMessage
	readMessage(t, reader, &done)
	if done.Type != "done" || done.Path != `C:\Videos\clip.mkv` {
		t.Fatalf("unexpected done response: %#v", done)
	}
}

func TestAppReportsMissingOutputPathWhenChildOmitsDone(t *testing.T) {
	t.Setenv(fakeYTDLPEnv, "missing-path")
	reader := runStartedDownload(t, writeTestConfigurationWithFakeYTDLP(t))

	expectAcceptedThenError(t, reader, ErrorKindMissingOutputPath)
}

func TestFailedOutcomeIgnoresLoginHintsWhenCookiesWereUsed(t *testing.T) {
	loginLine := "ERROR: [youtube] abc: Sign in to confirm you're not a bot."
	failed := processResult{waitErr: errors.New("exit status 1")}

	message := failedOutcome(t, failed, false, loginLine)
	if message.Kind != ErrorKindYTDLPLoginRequired || message.Detail != "" {
		t.Fatalf("without cookies: %#v", message)
	}
	message = failedOutcome(t, failed, true, loginLine)
	if message.Kind != ErrorKindYTDLPFailed || message.Detail != loginLine {
		t.Fatalf("with cookies: %#v", message)
	}
	message = failedOutcome(t, failed, false, "ERROR: Requested format is not available")
	if message.Kind != ErrorKindYTDLPFailed || message.Detail == "" {
		t.Fatalf("unrelated failure: %#v", message)
	}
}

func failedOutcome(t *testing.T, result processResult, cookiesUsed bool, lines ...string) errorMessage {
	t.Helper()
	outputTail := tail.New(tailLineLimit, "")
	for _, line := range lines {
		outputTail.Add(line)
	}
	message, ok := result.outcome("request1", cookiesUsed, outputTail).(errorMessage)
	if !ok {
		t.Fatalf("expected an error message for %+v", result)
	}
	return message
}

func writeTestConfiguration(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	settings := config.Settings{
		Destination: filepath.Join(root, "Videos"), DefaultPreset: "archive", SubtitleLanguages: []string{"ja", "en"}, Notifications: true,
	}
	if _, err := config.SaveSettings(filepath.Join(root, "config", "settings.json"), settings); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeTestConfigurationWithFakeYTDLP(t *testing.T) string {
	t.Helper()
	root := writeTestConfiguration(t)
	source, err := os.Open(testExecutable(t))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	binDirectory := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	target, err := os.Create(ytdlp.Executable(binDirectory))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

func runStartedDownload(t *testing.T, root string, cookieItems ...cookies.Cookie) *protocol.Reader {
	t.Helper()
	inputReader, inputWriter := io.Pipe()
	t.Cleanup(func() { inputWriter.Close() })
	go func() {
		_ = protocol.NewWriter(inputWriter).Send(startRequest{
			Version: 1,
			Type:    "start",
			ID:      "request1",
			URL:     "https://www.youtube.com/watch?v=abc",
			Preset:  "archive",
			Cookies: cookieItems,
		})
	}()
	var output bytes.Buffer
	app := New(root, inputReader, &output, &bytes.Buffer{})
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	return protocol.NewReader(&output)
}

func testExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return executable
}

func readMessage(t *testing.T, reader *protocol.Reader, target any) {
	t.Helper()
	raw, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func expectAcceptedThenError(t *testing.T, reader *protocol.Reader, kind ErrorKind) errorMessage {
	t.Helper()
	var accepted acceptedMessage
	readMessage(t, reader, &accepted)
	if accepted.Type != "accepted" || accepted.ID != "request1" || accepted.Version != ProtocolVersion {
		t.Fatalf("unexpected acceptance: %#v", accepted)
	}
	var failure errorMessage
	readMessage(t, reader, &failure)
	if failure.Type != "error" || failure.ID != "request1" || failure.Version != ProtocolVersion || failure.Kind != kind {
		t.Fatalf("unexpected failure: %#v", failure)
	}
	return failure
}

type rejectingWriter struct {
	writes int
}

func (w *rejectingWriter) Write(_ []byte) (int, error) {
	w.writes++
	return 0, errors.New("writer rejected output")
}

func TestAppReportsAnUnreadableChildStream(t *testing.T) {
	t.Setenv(fakeYTDLPEnv, "long-line")
	reader := runStartedDownload(t, writeTestConfigurationWithFakeYTDLP(t))

	failure := expectAcceptedThenError(t, reader, ErrorKindProcessOutputFailed)
	if failure.Detail != "" {
		t.Fatalf("unexpected failure: %#v", failure)
	}
}

func TestAppReportsACookieFileItCannotCreate(t *testing.T) {
	root := writeTestConfiguration(t)
	if err := os.WriteFile(filepath.Join(root, "tmp"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(root, "config", "settings.json")
	settings, err := config.LoadSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	settings.CookiesEnabled = true
	if _, err := config.SaveSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}

	reader := runStartedDownload(t, root, cookies.Cookie{Domain: ".youtube.com", Name: "SID", Value: "secret", Path: "/", Secure: true})
	failure := expectAcceptedThenError(t, reader, ErrorKindCookieFileFailed)
	if failure.Detail != "" {
		t.Fatalf("unexpected failure: %#v", failure)
	}
}

func TestAppReportsACancelledDownload(t *testing.T) {
	t.Setenv(fakeYTDLPEnv, "hang")
	root := writeTestConfigurationWithFakeYTDLP(t)

	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	app := New(root, inputReader, outputWriter, &bytes.Buffer{})
	finished := make(chan error, 1)
	go func() { finished <- app.Run() }()

	writer := protocol.NewWriter(inputWriter)
	if err := writer.Send(startRequest{
		Version: 1,
		Type:    "start",
		ID:      "request1",
		URL:     "https://www.youtube.com/watch?v=abc",
		Preset:  "archive",
	}); err != nil {
		t.Fatal(err)
	}
	reader := protocol.NewReader(outputReader)
	var accepted acceptedMessage
	readMessage(t, reader, &accepted)
	if accepted.Type != "accepted" {
		t.Fatalf("unexpected acceptance: %#v", accepted)
	}
	if err := writer.Send(cancelRequest{Version: 1, Type: "cancel", ID: "request1"}); err != nil {
		t.Fatal(err)
	}
	var failure errorMessage
	readMessage(t, reader, &failure)
	if failure.Kind != ErrorKindCancelled || failure.ID != "request1" || failure.Detail != "" {
		t.Fatalf("unexpected failure: %#v", failure)
	}

	inputWriter.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	outputWriter.Close()
}

func TestAppStopsTheDownloadWhenInputCloses(t *testing.T) {
	t.Setenv(fakeYTDLPEnv, "hang")
	startedPath := filepath.Join(t.TempDir(), "started")
	t.Setenv(fakeStartedEnv, startedPath)
	root := writeTestConfigurationWithFakeYTDLP(t)

	inputReader, inputWriter := io.Pipe()
	app := New(root, inputReader, io.Discard, &bytes.Buffer{})
	finished := make(chan error, 1)
	go func() { finished <- app.Run() }()
	if err := protocol.NewWriter(inputWriter).Send(startRequest{
		Version: 1,
		Type:    "start",
		ID:      "request1",
		URL:     "https://www.youtube.com/watch?v=abc",
		Preset:  "archive",
	}); err != nil {
		t.Fatal(err)
	}
	child := waitForFakeChild(t, startedPath)
	t.Cleanup(func() { _ = child.Kill() })

	inputWriter.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		_, _ = child.Wait()
		close(exited)
	}()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("the download process outlived the host input")
	}
}

func waitForFakeChild(t *testing.T, startedPath string) *os.Process {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		payload, err := os.ReadFile(startedPath)
		if err == nil {
			pid, err := strconv.Atoi(string(payload))
			if err != nil {
				t.Fatal(err)
			}
			process, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			return process
		}
		if time.Now().After(deadline) {
			t.Fatal("the fake downloader never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

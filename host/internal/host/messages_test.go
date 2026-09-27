package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDecodeStrictRejectsTrailingData(t *testing.T) {
	raw := json.RawMessage(`{"v":1,"type":"start","id":"abc","url":"https://youtu.be/abc","preset":"archive"} {}`)
	var request startRequest
	if err := decodeStrict(raw, &request); err == nil {
		t.Fatal("expected trailing data to be rejected")
	}
}

func TestRequestIDValidation(t *testing.T) {
	for _, id := range []string{"a1b2", "request_123", "A-B"} {
		if err := validateRequestID(id); err != nil {
			t.Errorf("expected %q to be valid: %v", id, err)
		}
	}
	for _, id := range []string{"", "contains space", "../escape"} {
		if err := validateRequestID(id); err == nil {
			t.Errorf("expected %q to be invalid", id)
		}
	}
}

var errorKinds = []ErrorKind{
	ErrorKindCancelled,
	ErrorKindInvalidRequest,
	ErrorKindHostFailure,
	ErrorKindDestinationPickFailed,
	ErrorKindCookieFileFailed,
	ErrorKindProcessStartFailed,
	ErrorKindProcessOutputFailed,
	ErrorKindMissingOutputPath,
	ErrorKindYTDLPFailed,
	ErrorKindYTDLPLoginRequired,
}

func TestErrorKindsMatchTheSharedVocabulary(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "error-kinds.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want []ErrorKind
	if err := json.Unmarshal(payload, &want); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(errorKinds, want) {
		t.Fatalf("error kinds diverged from the shared vocabulary: got %v, want %v", errorKinds, want)
	}
}

type protocolCase struct {
	Name    string          `json:"name"`
	Message json.RawMessage `json:"message"`
	Why     string          `json:"why"`
}

type protocolFixture struct {
	Accepted []protocolCase `json:"accepted"`
	Rejected []protocolCase `json:"rejected"`
}

func loadProtocolFixture(t *testing.T) protocolFixture {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "protocol-messages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture protocolFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Accepted) == 0 || len(fixture.Rejected) == 0 {
		t.Fatalf("the shared protocol fixture is empty: %#v", fixture)
	}
	return fixture
}

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
	fixture := loadProtocolFixture(t)
	for _, testCase := range fixture.Accepted {
		t.Run(testCase.Name, func(t *testing.T) {
			if err := decodeStrict(testCase.Message, requestTarget(t, testCase.Message)); err != nil {
				t.Fatalf("the host must decode a request the extension sends: %v", err)
			}
		})
	}
	for _, testCase := range fixture.Rejected {
		t.Run(testCase.Name, func(t *testing.T) {
			if err := decodeStrict(testCase.Message, requestTarget(t, testCase.Message)); err == nil {
				t.Fatalf("expected the request to be refused: %s", testCase.Why)
			}
		})
	}
}

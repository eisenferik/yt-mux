package host

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/eisenferik/yt-mux/host/internal/config"
	"github.com/eisenferik/yt-mux/host/internal/cookies"
)

const ProtocolVersion = 1

type ErrorKind string

const (
	ErrorKindCancelled             ErrorKind = "CANCELLED"
	ErrorKindInvalidRequest        ErrorKind = "INVALID_REQUEST"
	ErrorKindHostFailure           ErrorKind = "HOST_FAILURE"
	ErrorKindDestinationPickFailed ErrorKind = "DESTINATION_PICK_FAILED"
	ErrorKindCookieFileFailed      ErrorKind = "COOKIE_FILE_FAILED"
	ErrorKindProcessStartFailed    ErrorKind = "PROCESS_START_FAILED"
	ErrorKindMissingOutputPath     ErrorKind = "MISSING_OUTPUT_PATH"
	ErrorKindYTDLPFailed           ErrorKind = "YTDLP_FAILED"
	ErrorKindYTDLPLoginRequired    ErrorKind = "YTDLP_LOGIN_REQUIRED"
	ErrorKindYTDLPMembersOnly      ErrorKind = "YTDLP_MEMBERS_ONLY"
)

type header struct {
	Version int    `json:"v"`
	Type    string `json:"type"`
	ID      string `json:"id"`
}

func newHeader(messageType, id string) header {
	return header{Version: ProtocolVersion, Type: messageType, ID: id}
}

type startRequest struct {
	header
	URL     string           `json:"url"`
	Preset  string           `json:"preset"`
	Cookies []cookies.Cookie `json:"cookies,omitempty"`
}

type cancelRequest struct {
	header
}

type settingsGetRequest struct {
	header
}

type settingsSetRequest struct {
	header
	Settings config.Settings `json:"settings"`
}

type destinationPickRequest struct {
	header
	Start string `json:"start,omitempty"`
}

type nativeResponse interface {
	isNativeResponse()
}

type responseMarker struct{}

func (responseMarker) isNativeResponse() {}

type acceptedMessage struct {
	header
	responseMarker
}

type progressMessage struct {
	header
	responseMarker
	Percent *float64 `json:"pct"`
}

type postprocessMessage struct {
	header
	responseMarker
}

type doneMessage struct {
	header
	responseMarker
	Path string `json:"path"`
}

type errorMessage struct {
	header
	responseMarker
	Kind   ErrorKind `json:"kind"`
	Detail string    `json:"detail,omitempty"`
}

func newError(id string, kind ErrorKind, detail string) errorMessage {
	return errorMessage{header: newHeader("error", id), Kind: kind, Detail: detail}
}

type settingsMessage struct {
	header
	responseMarker
	Settings config.Settings `json:"settings"`
}

type settingsSavedMessage struct {
	header
	responseMarker
	Settings config.Settings `json:"settings"`
}

type destinationMessage struct {
	header
	responseMarker
	Path string `json:"path"`
}

func decodeEnvelope(raw json.RawMessage) (header, error) {
	var result header
	if err := json.Unmarshal(raw, &result); err != nil {
		return header{}, errors.New("invalid request envelope")
	}
	return result, nil
}

func decodeStrict(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("request does not match the required schema")
	}
	return nil
}

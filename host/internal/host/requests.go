package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/eisenferik/yt-mux/host/internal/config"
	"github.com/eisenferik/yt-mux/host/internal/folderpick"
)

type requestOutcome struct {
	reply     nativeResponse
	operation *operationPlan
	fatal     error
}

func (a *App) handleRequest(raw json.RawMessage, active *operation) requestOutcome {
	envelope, err := decodeEnvelope(raw)
	if err != nil {
		return requestOutcome{fatal: err}
	}

	switch envelope.Type {
	case "settings.get":
		var request settingsGetRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		settings, err := config.LoadSettings(a.settingsPath)
		if err != nil {
			return fatalHostFailureOutcome(request.ID, err)
		}
		return requestOutcome{reply: settingsMessage{
			header:   newHeader("settings", request.ID),
			Settings: settings,
			Version:  installedVersion(a.installRoot),
		}}

	case "settings.set":
		var request settingsSetRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		saved, err := config.SaveSettings(a.settingsPath, request.Settings)
		if err != nil {
			if config.IsSettingsValidationError(err) {
				return requestOutcome{reply: newError(request.ID, ErrorKindInvalidRequest, err.Error())}
			}
			return fatalHostFailureOutcome(request.ID, err)
		}
		return requestOutcome{reply: settingsSavedMessage{header: newHeader("settings.saved", request.ID), Settings: saved}}

	case "destination.pick":
		var request destinationPickRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		start := request.Start
		if start == "" {
			settings, err := config.LoadSettings(a.settingsPath)
			if err != nil {
				return fatalHostFailureOutcome(request.ID, err)
			}
			start = settings.Destination
		}
		path, cancelled, err := folderpick.Pick(start)
		if cancelled {
			return requestOutcome{reply: newError(request.ID, ErrorKindCancelled, "")}
		}
		if err != nil {
			return requestOutcome{reply: newError(request.ID, ErrorKindDestinationPickFailed, "")}
		}
		return requestOutcome{reply: destinationMessage{header: newHeader("destination", request.ID), Path: path}}

	case "reveal":
		var request revealRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		if _, err := os.Stat(request.Path); err != nil {
			return requestOutcome{reply: newError(request.ID, ErrorKindSavedFileMissing, "")}
		}
		if err := a.launch(explorerCommand(request.Path)); err != nil {
			return fatalHostFailureOutcome(request.ID, err)
		}
		return requestOutcome{reply: launchedMessage{header: newHeader("launched", request.ID)}}

	case "update":
		var request updateRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		if err := a.launch(updateCommand(a.installRoot)); err != nil {
			return fatalHostFailureOutcome(request.ID, err)
		}
		return requestOutcome{reply: launchedMessage{header: newHeader("launched", request.ID)}}

	case "start":
		var request startRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		settings, err := config.LoadSettings(a.settingsPath)
		if err != nil {
			return fatalHostFailureOutcome(request.ID, err)
		}
		preset := config.Presets[request.Preset]
		plan := &operationPlan{
			run: func(operationContext context.Context) (nativeResponse, error) {
				return a.download(
					operationContext,
					request.ID,
					request.URL,
					preset,
					settings,
					request.Cookies,
				)
			},
		}
		return requestOutcome{
			reply:     acceptedMessage{header: newHeader("accepted", request.ID)},
			operation: plan,
		}

	case "cancel":
		var request cancelRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		active.cancel()
	}
	return requestOutcome{}
}

func fatalInvalidRequestOutcome(id string, err error) requestOutcome {
	return requestOutcome{reply: newError(id, ErrorKindInvalidRequest, err.Error()), fatal: err}
}

func fatalHostFailureOutcome(id string, err error) requestOutcome {
	return requestOutcome{
		reply: newError(id, ErrorKindHostFailure, ""),
		fatal: err,
	}
}

func installedVersion(installRoot string) string {
	var manifest struct {
		Version string `json:"version"`
	}
	payload, err := os.ReadFile(filepath.Join(installRoot, "extension", "manifest.json"))
	if err != nil || json.Unmarshal(payload, &manifest) != nil {
		return ""
	}
	return manifest.Version
}

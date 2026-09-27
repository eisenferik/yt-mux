package host

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/eisenferik/yt-mux/host/internal/config"
	"github.com/eisenferik/yt-mux/host/internal/cookies"
	"github.com/eisenferik/yt-mux/host/internal/folderpick"
	"github.com/eisenferik/yt-mux/host/internal/urlguard"
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
	if err := validateRequestID(envelope.ID); err != nil {
		return fatalInvalidRequestOutcome(envelope.ID, err)
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
		return requestOutcome{reply: settingsMessage{header: newHeader("settings", request.ID), Settings: settings}}

	case "settings.set":
		if active != nil {
			return invalidRequestOutcome(envelope.ID, errors.New("an operation is already running"))
		}
		var request settingsSetRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		saved, err := config.SaveSettings(a.settingsPath, request.Settings)
		if err != nil {
			if config.IsSettingsValidationError(err) {
				return invalidRequestOutcome(request.ID, err)
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
			return requestOutcome{reply: newError(request.ID, ErrorKindDestinationPickFailed, err.Error())}
		}
		return requestOutcome{reply: destinationMessage{header: newHeader("destination", request.ID), Path: path}}

	case "start":
		if active != nil {
			return invalidRequestOutcome(envelope.ID, errors.New("an operation is already running"))
		}
		var request startRequest
		if err := decodeStrict(raw, &request); err != nil {
			return fatalInvalidRequestOutcome(envelope.ID, err)
		}
		normalizedURL, err := urlguard.Normalize(request.URL)
		if err != nil {
			return fatalInvalidRequestOutcome(request.ID, err)
		}
		preset, ok := config.Presets[request.Preset]
		if !ok {
			return fatalInvalidRequestOutcome(request.ID, errors.New("unsupported preset"))
		}
		settings, err := config.LoadSettings(a.settingsPath)
		if err != nil {
			return fatalHostFailureOutcome(request.ID, err)
		}
		if len(request.Cookies) > 0 && !settings.CookiesEnabled {
			return fatalInvalidRequestOutcome(request.ID, errors.New("cookies are disabled in settings"))
		}
		if err := cookies.Validate(request.Cookies); err != nil {
			return fatalInvalidRequestOutcome(request.ID, err)
		}
		plan := &operationPlan{
			id: request.ID,
			run: func(operationContext context.Context) (nativeResponse, error) {
				return a.download(
					operationContext,
					request.ID,
					normalizedURL,
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
		if active == nil || active.id != request.ID {
			return invalidRequestOutcome(request.ID, errors.New("no matching operation is running"))
		}
		active.cancel()
		return requestOutcome{}

	default:
		return fatalInvalidRequestOutcome(envelope.ID, errors.New("unsupported request type"))
	}
}

func invalidRequestOutcome(id string, err error) requestOutcome {
	return requestOutcome{reply: newError(id, ErrorKindInvalidRequest, err.Error())}
}

func fatalInvalidRequestOutcome(id string, err error) requestOutcome {
	outcome := invalidRequestOutcome(id, err)
	outcome.fatal = err
	return outcome
}

func fatalHostFailureOutcome(id string, err error) requestOutcome {
	return requestOutcome{
		reply: newError(id, ErrorKindHostFailure, ""),
		fatal: err,
	}
}

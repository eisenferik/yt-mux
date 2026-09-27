package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/eisenferik/yt-mux/host/internal/cookies"
	"github.com/eisenferik/yt-mux/host/internal/protocol"
)

const ExpectedOrigin = "chrome-extension://nkdpdpbaoogfjijkpbmfaplddhbjmfop/"

type App struct {
	binDirectory string
	settingsPath string
	temporaryDir string
	reader       *protocol.Reader
	writer       *protocol.Writer
	log          io.Writer
}

type readResult struct {
	raw json.RawMessage
	err error
}

type operation struct {
	id     string
	cancel context.CancelFunc
	done   <-chan operationResult
}

type operationResult struct {
	message nativeResponse
	err     error
}

type operationPlan struct {
	id  string
	run func(context.Context) (nativeResponse, error)
}

func (p *operationPlan) start() *operation {
	operationContext, cancel := context.WithCancel(context.Background())
	done := make(chan operationResult, 1)
	go func() {
		message, err := p.run(operationContext)
		done <- operationResult{message: message, err: err}
	}()
	return &operation{id: p.id, cancel: cancel, done: done}
}

func New(installRoot string, input io.Reader, output, logOutput io.Writer) *App {
	app := &App{
		binDirectory: filepath.Join(installRoot, "bin"),
		settingsPath: filepath.Join(installRoot, "config", "settings.json"),
		temporaryDir: filepath.Join(installRoot, "tmp"),
		reader:       protocol.NewReader(input),
		writer:       protocol.NewWriter(output),
		log:          logOutput,
	}
	if err := cookies.CleanupStale(app.temporaryDir, time.Now()); err != nil {
		fmt.Fprintf(app.log, "warning: stale cookie cleanup failed: %v\n", err)
	}
	return app
}

func (a *App) Run() error {
	readChannel := make(chan readResult, 1)
	go func() {
		for {
			raw, err := a.reader.Read()
			readChannel <- readResult{raw: raw, err: err}
			if err != nil {
				return
			}
		}
	}()

	var active *operation
	defer func() {
		if active != nil {
			a.cancelAndWait(active)
		}
	}()

	for {
		var operationDone <-chan operationResult
		if active != nil {
			operationDone = active.done
		}
		select {
		case result := <-readChannel:
			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					return nil
				}
				return fmt.Errorf("read native request: %w", result.err)
			}
			outcome := a.handleRequest(result.raw, active)
			if outcome.reply != nil {
				if err := a.writer.Send(outcome.reply); err != nil {
					return fmt.Errorf("write native response: %w", err)
				}
			}
			if outcome.fatal != nil {
				return outcome.fatal
			}
			if outcome.operation != nil {
				active = outcome.operation.start()
			}
		case result := <-operationDone:
			active.cancel()
			active = nil
			if result.err != nil {
				return result.err
			}
			if err := a.writer.Send(result.message); err != nil {
				return fmt.Errorf("write native response: %w", err)
			}
			return nil
		}
	}
}

func (a *App) cancelAndWait(active *operation) {
	active.cancel()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-active.done:
	case <-timer.C:
		// Process exit closes the Job Object as a final fail-safe.
	}
}

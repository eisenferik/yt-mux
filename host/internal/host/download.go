package host

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/eisenferik/yt-mux/host/internal/config"
	"github.com/eisenferik/yt-mux/host/internal/cookies"
	"github.com/eisenferik/yt-mux/host/internal/job"
	"github.com/eisenferik/yt-mux/host/internal/tail"
	"github.com/eisenferik/yt-mux/host/internal/ytdlp"
)

const (
	tailLineLimit        = 100
	childLineBufferLimit = 1024 * 1024
)

func (a *App) download(
	ctx context.Context,
	id, rawURL string,
	preset config.Preset,
	settings config.Settings,
	cookieItems []cookies.Cookie,
) (nativeResponse, error) {
	cookiePath := ""
	if len(cookieItems) > 0 {
		path, err := cookies.CreateSecure(a.temporaryDir, cookieItems)
		if err != nil {
			return newError(id, ErrorKindCookieFileFailed, ""), nil
		}
		cookiePath = path
		defer os.Remove(cookiePath)
	}
	args := ytdlp.DownloadArguments(a.binDirectory, settings, preset, rawURL, cookiePath)
	process, err := job.Start(ytdlp.Executable(a.binDirectory), args)
	if err != nil {
		return newError(id, ErrorKindProcessStartFailed, ""), nil
	}
	defer process.Close()

	outputTail := tail.New(tailLineLimit, cookiePath)
	result, streamErr := a.monitorProcess(ctx, id, process, outputTail)
	if streamErr != nil {
		return nil, fmt.Errorf("write native progress: %w", streamErr)
	}
	return result.outcome(id, cookiePath != "", outputTail), nil
}

type childLine struct {
	text   string
	stdout bool
	err    error
}

type processResult struct {
	finalPath    string
	waitErr      error
	cancelled    bool
	outputFailed bool
}

func (r processResult) outcome(id string, cookiesUsed bool, outputTail *tail.Buffer) nativeResponse {
	switch {
	case r.cancelled:
		return newError(id, ErrorKindCancelled, "")
	case r.outputFailed:
		return newError(id, ErrorKindProcessOutputFailed, "")
	case r.waitErr != nil:
		analysis := ytdlp.AnalyzeFailure(outputTail.Lines())
		if !cookiesUsed && analysis.LoginRequired {
			return newError(id, ErrorKindYTDLPLoginRequired, "")
		}
		return newError(id, ErrorKindYTDLPFailed, analysis.Diagnostic)
	case r.finalPath == "":
		return newError(id, ErrorKindMissingOutputPath, "")
	default:
		return doneMessage{header: newHeader("done", id), Path: r.finalPath}
	}
}

func (a *App) monitorProcess(ctx context.Context, id string, process *job.Process, outputTail *tail.Buffer) (processResult, error) {
	lines := make(chan childLine, 32)
	var scanners sync.WaitGroup
	scanners.Add(2)
	go scanLines(process.Stdout(), true, lines, &scanners)
	go scanLines(process.Stderr(), false, lines, &scanners)
	waitChannel := make(chan error, 1)
	go func() {
		// Wait closes the child pipes, so both scanners must reach EOF first
		// or the trailing @DONE@ line is lost.
		scanners.Wait()
		close(lines)
		waitChannel <- process.Wait()
	}()

	var result processResult
	var tracker ytdlp.Tracker
	postprocessing := false
	ctxDone := ctx.Done()
	for waitChannel != nil || lines != nil {
		select {
		case <-ctxDone:
			result.cancelled = true
			_ = process.Terminate()
			ctxDone = nil
		case err := <-waitChannel:
			result.waitErr = err
			waitChannel = nil
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			if line.err != nil {
				if !result.outputFailed {
					result.outputFailed = true
					_ = process.Terminate()
				}
				continue
			}
			if status, ok := ytdlp.ParsePostprocess(line.text); ok {
				if status != "started" || postprocessing {
					continue
				}
				postprocessing = true
				if err := a.writer.Send(postprocessMessage{header: newHeader("postprocess", id)}); err != nil {
					_ = process.Terminate()
					return processResult{}, err
				}
				continue
			}
			if line.stdout {
				if total, ok := ytdlp.ParseTotal(line.text); ok {
					tracker.SetTotal(total)
					continue
				}
				if progress, ok := ytdlp.ParseProgress(line.text); ok {
					if postprocessing {
						continue
					}
					message := progressMessage{
						header:     newHeader("progress", id),
						Downloaded: progress.Downloaded,
						Total:      progress.Total,
						Speed:      progress.Speed,
						ETA:        progress.ETA,
						Percent:    tracker.Update(progress),
					}
					if err := a.writer.Send(message); err != nil {
						_ = process.Terminate()
						return processResult{}, err
					}
					continue
				}
				if path, ok := ytdlp.ParseDone(line.text); ok {
					result.finalPath = path
					continue
				}
			}
			outputTail.Add(line.text)
		}
	}
	return result, nil
}

func scanLines(reader io.Reader, stdout bool, output chan<- childLine, group *sync.WaitGroup) {
	defer group.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), childLineBufferLimit)
	for scanner.Scan() {
		output <- childLine{text: strings.TrimSuffix(scanner.Text(), "\r"), stdout: stdout}
	}
	if err := scanner.Err(); err != nil {
		output <- childLine{stdout: stdout, err: err}
	}
}

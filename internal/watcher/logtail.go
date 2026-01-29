package watcher

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// LogTailer tails trunk-recorder log files with rotation support
type LogTailer struct {
	logDir   string
	parser   *LogParser
	logger   *zap.Logger
	watcher  *fsnotify.Watcher
	eventsCh chan LogEvent

	// Current state
	currentFile *os.File
	currentPath string

	// Log file pattern: MM-DD-YYYY_HHMM_NN.log
	reLogFile *regexp.Regexp
}

// NewLogTailer creates a new log tailer
func NewLogTailer(logDir string, logger *zap.Logger) (*LogTailer, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	return &LogTailer{
		logDir:    logDir,
		parser:    NewLogParser(),
		logger:    logger,
		watcher:   watcher,
		eventsCh:  make(chan LogEvent, 1000),
		reLogFile: regexp.MustCompile(`^\d{2}-\d{2}-\d{4}_\d{4}_\d{2}\.log$`),
	}, nil
}

// Events returns the channel of parsed log events
func (t *LogTailer) Events() <-chan LogEvent {
	return t.eventsCh
}

// Start begins tailing the log directory
func (t *LogTailer) Start(ctx context.Context) error {
	// Watch the log directory for new files
	if err := t.watcher.Add(t.logDir); err != nil {
		return fmt.Errorf("watch log directory: %w", err)
	}

	// Find and open the most recent log file
	if err := t.openLatestLog(); err != nil {
		t.logger.Warn("No log files found yet", zap.String("dir", t.logDir))
	}

	go t.run(ctx)
	return nil
}

// Stop closes the tailer
func (t *LogTailer) Stop() error {
	if t.currentFile != nil {
		t.currentFile.Close()
	}
	close(t.eventsCh)
	return t.watcher.Close()
}

// openLatestLog finds and opens the most recent log file
func (t *LogTailer) openLatestLog() error {
	entries, err := os.ReadDir(t.logDir)
	if err != nil {
		return err
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && t.reLogFile.MatchString(entry.Name()) {
			logFiles = append(logFiles, entry.Name())
		}
	}

	if len(logFiles) == 0 {
		return fmt.Errorf("no log files found")
	}

	// Sort by name (date format sorts correctly)
	sort.Strings(logFiles)

	// Open the latest file
	latest := logFiles[len(logFiles)-1]
	return t.switchToFile(filepath.Join(t.logDir, latest), true)
}

// switchToFile switches to tailing a new file
func (t *LogTailer) switchToFile(path string, seekEnd bool) error {
	// Close current file if open
	if t.currentFile != nil {
		t.currentFile.Close()
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}

	if seekEnd {
		// Seek to end - we only want new lines
		_, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			file.Close()
			return err
		}
	}

	t.currentFile = file
	t.currentPath = path
	t.parser.Reset()

	t.logger.Info("Tailing log file",
		zap.String("file", filepath.Base(path)),
		zap.Bool("from_end", seekEnd),
	)

	return nil
}

// run is the main event loop
func (t *LogTailer) run(ctx context.Context) {
	reader := bufio.NewReader(t.currentFile)
	pollTicker := time.NewTicker(100 * time.Millisecond)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-t.watcher.Events:
			if !ok {
				return
			}

			// Check for new log files
			if event.Op&fsnotify.Create != 0 {
				name := filepath.Base(event.Name)
				if t.reLogFile.MatchString(name) {
					// New log file created - check if it's newer
					if t.isNewerLog(event.Name) {
						t.logger.Info("Log rotation detected, switching to new file",
							zap.String("file", name),
						)
						// Read any remaining lines from current file first
						t.drainFile(reader)
						// Switch to new file
						if err := t.switchToFile(event.Name, false); err != nil {
							t.logger.Error("Failed to switch to new log", zap.Error(err))
						} else {
							reader = bufio.NewReader(t.currentFile)
						}
					}
				}
			}

			// Handle writes to current file
			if event.Op&fsnotify.Write != 0 && event.Name == t.currentPath {
				t.readLines(reader)
			}

		case <-pollTicker.C:
			// Poll for new content (some systems don't generate write events for appends)
			if t.currentFile != nil {
				t.readLines(reader)
			}

		case err, ok := <-t.watcher.Errors:
			if !ok {
				return
			}
			t.logger.Error("Log watcher error", zap.Error(err))
		}
	}
}

// readLines reads available lines from the file
func (t *LogTailer) readLines(reader *bufio.Reader) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				t.logger.Debug("Error reading log", zap.Error(err))
			}
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		events := t.parser.ParseLine(line)
		for _, event := range events {
			select {
			case t.eventsCh <- event:
			default:
				// Channel full, drop event
				t.logger.Debug("Log event channel full, dropping event")
			}
		}
	}
}

// drainFile reads any remaining content from the current file
func (t *LogTailer) drainFile(reader *bufio.Reader) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		events := t.parser.ParseLine(line)
		for _, event := range events {
			select {
			case t.eventsCh <- event:
			default:
			}
		}
	}
}

// isNewerLog checks if the given log file is newer than the current one
func (t *LogTailer) isNewerLog(path string) bool {
	if t.currentPath == "" {
		return true
	}

	// Compare by filename (date format)
	currentName := filepath.Base(t.currentPath)
	newName := filepath.Base(path)

	return newName > currentName
}

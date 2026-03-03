package audio

import (
	"os"
	"path/filepath"
	"strings"
)

// containedIn reports whether path is under dir after cleaning and resolving to absolute paths.
func containedIn(path, dir string) bool {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return false
	}
	// Ensure the path is either exactly the dir or under dir + separator
	return absPath == absDir || strings.HasPrefix(absPath, absDir+string(filepath.Separator))
}

// ResolveFile finds an audio file on disk given the tr-engine managed path
// and/or trunk-recorder's call_filename.
// Priority: 1) absolute path within allowed dir  2) audioDir/audioPath  3) trAudioDir + call_filename
// All resolved paths are validated to be within their respective allowed directories.
func ResolveFile(audioDir, trAudioDir, audioPath, callFilename string) string {
	// 1) Absolute audioPath within an allowed directory (e.g. from file watcher)
	if audioPath != "" && filepath.IsAbs(audioPath) {
		if (audioDir != "" && containedIn(audioPath, audioDir)) ||
			(trAudioDir != "" && containedIn(audioPath, trAudioDir)) {
			if _, err := os.Stat(audioPath); err == nil {
				return audioPath
			}
		}
	}

	// 2) tr-engine managed audio file (relative path under audioDir)
	if audioPath != "" && audioDir != "" {
		full := filepath.Join(audioDir, audioPath)
		if containedIn(full, audioDir) {
			if _, err := os.Stat(full); err == nil {
				return full
			}
		}
	}

	if callFilename == "" {
		return ""
	}

	// 2) TR_AUDIO_DIR configured — resolve call_filename relative to it
	if trAudioDir != "" {
		// call_filename is TR's absolute path (e.g. /app/tr_audio/warco/2026/2/17/file.m4a)
		// Try it directly under TR_AUDIO_DIR by extracting the basename
		full := filepath.Join(trAudioDir, filepath.Base(callFilename))
		if containedIn(full, trAudioDir) {
			if _, err := os.Stat(full); err == nil {
				return full
			}
		}

		// Try matching: find the short_name directory in call_filename
		// and use everything from there as a relative path under TR_AUDIO_DIR.
		// e.g. /app/tr_audio/warco/2026/2/17/file.m4a → warco/2026/2/17/file.m4a
		parts := strings.Split(filepath.ToSlash(callFilename), "/")
		for i := range parts {
			if i == 0 {
				continue
			}
			candidate := filepath.Join(trAudioDir, filepath.Join(parts[i:]...))
			if containedIn(candidate, trAudioDir) {
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}

	// Absolute path fallback removed — serving arbitrary MQTT-provided paths
	// is a directory traversal risk. Set TR_AUDIO_DIR or AUDIO_DIR instead.
	return ""
}

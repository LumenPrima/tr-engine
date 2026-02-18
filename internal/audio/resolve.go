package audio

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveFile finds an audio file on disk given the tr-engine managed path
// and/or trunk-recorder's call_filename.
// Priority: 1) audioDir/audioPath  2) trAudioDir + call_filename  3) absolute call_filename
func ResolveFile(audioDir, trAudioDir, audioPath, callFilename string) string {
	// 1) tr-engine managed audio file
	if audioPath != "" && audioDir != "" {
		full := filepath.Join(audioDir, audioPath)
		if _, err := os.Stat(full); err == nil {
			return full
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
		if _, err := os.Stat(full); err == nil {
			return full
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
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	// 3) Try call_filename as an absolute path (same machine, same filesystem)
	if filepath.IsAbs(callFilename) {
		if _, err := os.Stat(callFilename); err == nil {
			return callFilename
		}
	}

	return ""
}

package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/snarg/tr-engine/internal/database"
)

// CompositeID represents a parsed composite ID (system_id:entity_id) or plain entity ID.
type CompositeID struct {
	SystemID int
	EntityID int
	IsPlain  bool // true if no system_id prefix was given
}

// ParseCompositeID parses a path parameter as either "system_id:entity_id" or plain "entity_id".
func ParseCompositeID(r *http.Request, param string) (CompositeID, error) {
	raw, _ := url.PathUnescape(chi.URLParam(r, param))
	if raw == "" {
		return CompositeID{}, fmt.Errorf("missing path parameter: %s", param)
	}

	// Accept both ":" and "-" as separators for composite IDs.
	// Dash is Cloudflare WAF-safe; colon is the canonical format.
	sep := -1
	if idx := strings.IndexByte(raw, ':'); idx > 0 {
		sep = idx
	} else if idx := strings.IndexByte(raw, '-'); idx > 0 {
		// Only treat "-" as separator if both sides are numeric
		left, right := raw[:idx], raw[idx+1:]
		if _, err := strconv.Atoi(left); err == nil {
			if _, err := strconv.Atoi(right); err == nil {
				sep = idx
			}
		}
	}

	if sep > 0 {
		sysID, err := strconv.Atoi(raw[:sep])
		if err != nil {
			return CompositeID{}, fmt.Errorf("invalid system_id in composite ID: %s", raw)
		}
		entID, err := strconv.Atoi(raw[sep+1:])
		if err != nil {
			return CompositeID{}, fmt.Errorf("invalid entity_id in composite ID: %s", raw)
		}
		return CompositeID{SystemID: sysID, EntityID: entID}, nil
	}

	entID, err := strconv.Atoi(raw)
	if err != nil {
		return CompositeID{}, fmt.Errorf("invalid ID: %s", raw)
	}
	return CompositeID{EntityID: entID, IsPlain: true}, nil
}

// AmbiguousErrorResponse is returned when a plain ID matches multiple systems.
type AmbiguousErrorResponse struct {
	Error   string                    `json:"error"`
	Matches []database.AmbiguousMatch `json:"matches"`
}

// WriteAmbiguous writes a 409 response listing systems where the entity was found.
func WriteAmbiguous(w http.ResponseWriter, entityID int, matches []database.AmbiguousMatch) {
	WriteJSON(w, http.StatusConflict, AmbiguousErrorResponse{
		Error:   fmt.Sprintf("Ambiguous ID %d: exists in multiple systems", entityID),
		Matches: matches,
	})
}

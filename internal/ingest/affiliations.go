package ingest

import (
	"sync"
	"time"
)

// affiliationEntry tracks a unit's current talkgroup affiliation.
type affiliationEntry struct {
	SystemID        int
	SystemName      string
	Sysid           string
	UnitID          int
	UnitAlphaTag    string
	Tgid            int
	TgAlphaTag      string
	TgDescription   string
	TgTag           string
	TgGroup         string
	PreviousTgid    *int
	AffiliatedSince time.Time
	LastEventTime   time.Time
	Status          string // "affiliated" or "off"
}

type affiliationKey struct {
	SystemID int
	UnitID   int
}

type affiliationMap struct {
	mu    sync.Mutex
	items map[affiliationKey]*affiliationEntry
}

func newAffiliationMap() *affiliationMap {
	return &affiliationMap{items: make(map[affiliationKey]*affiliationEntry)}
}

func (m *affiliationMap) Update(key affiliationKey, entry *affiliationEntry) {
	m.mu.Lock()
	m.items[key] = entry
	m.mu.Unlock()
}

func (m *affiliationMap) MarkOff(key affiliationKey, t time.Time) {
	m.mu.Lock()
	if e, ok := m.items[key]; ok {
		e.Status = "off"
		e.LastEventTime = t
	}
	m.mu.Unlock()
}

func (m *affiliationMap) UpdateActivity(key affiliationKey, t time.Time) {
	m.mu.Lock()
	if e, ok := m.items[key]; ok {
		e.LastEventTime = t
	}
	m.mu.Unlock()
}

func (m *affiliationMap) Get(key affiliationKey) (*affiliationEntry, bool) {
	m.mu.Lock()
	e, ok := m.items[key]
	if ok {
		copy := *e
		m.mu.Unlock()
		return &copy, true
	}
	m.mu.Unlock()
	return nil, false
}

func (m *affiliationMap) All() []affiliationEntry {
	m.mu.Lock()
	result := make([]affiliationEntry, 0, len(m.items))
	for _, e := range m.items {
		result = append(result, *e)
	}
	m.mu.Unlock()
	return result
}

func (m *affiliationMap) EvictStale(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	m.mu.Lock()
	evicted := 0
	for k, e := range m.items {
		if e.LastEventTime.Before(cutoff) {
			delete(m.items, k)
			evicted++
		}
	}
	m.mu.Unlock()
	return evicted
}

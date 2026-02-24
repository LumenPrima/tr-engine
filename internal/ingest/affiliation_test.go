package ingest

import (
	"testing"
	"time"
)

func TestAffiliationMap_UpdateAndGet(t *testing.T) {
	m := newAffiliationMap()
	key := affiliationKey{SystemID: 1, UnitID: 100}
	now := time.Now()

	m.Update(key, &affiliationEntry{
		SystemID:        1,
		UnitID:          100,
		Tgid:            200,
		TgAlphaTag:      "Fire Dispatch",
		Status:          "affiliated",
		AffiliatedSince: now,
		LastEventTime:   now,
	})

	entry, ok := m.Get(key)
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if entry.SystemID != 1 {
		t.Errorf("SystemID = %d, want 1", entry.SystemID)
	}
	if entry.UnitID != 100 {
		t.Errorf("UnitID = %d, want 100", entry.UnitID)
	}
	if entry.Tgid != 200 {
		t.Errorf("Tgid = %d, want 200", entry.Tgid)
	}
	if entry.TgAlphaTag != "Fire Dispatch" {
		t.Errorf("TgAlphaTag = %q, want %q", entry.TgAlphaTag, "Fire Dispatch")
	}
	if entry.Status != "affiliated" {
		t.Errorf("Status = %q, want %q", entry.Status, "affiliated")
	}
}

func TestAffiliationMap_GetReturnsCopy(t *testing.T) {
	m := newAffiliationMap()
	key := affiliationKey{SystemID: 1, UnitID: 100}
	m.Update(key, &affiliationEntry{
		SystemID: 1,
		UnitID:   100,
		Tgid:     200,
		Status:   "affiliated",
	})

	// Mutate the returned copy
	entry, _ := m.Get(key)
	entry.Tgid = 999

	// Original should be unchanged
	original, _ := m.Get(key)
	if original.Tgid != 200 {
		t.Errorf("mutation leaked: Tgid = %d, want 200", original.Tgid)
	}
}

func TestAffiliationMap_GetMissing(t *testing.T) {
	m := newAffiliationMap()
	entry, ok := m.Get(affiliationKey{SystemID: 1, UnitID: 999})
	if ok {
		t.Error("expected ok=false for missing key")
	}
	if entry != nil {
		t.Error("expected nil entry for missing key")
	}
}

func TestAffiliationMap_MarkOff(t *testing.T) {
	m := newAffiliationMap()
	key := affiliationKey{SystemID: 1, UnitID: 100}
	now := time.Now()

	m.Update(key, &affiliationEntry{
		SystemID:      1,
		UnitID:        100,
		Tgid:          200,
		Status:        "affiliated",
		LastEventTime: now,
	})

	offTime := now.Add(5 * time.Second)
	m.MarkOff(key, offTime)

	entry, ok := m.Get(key)
	if !ok {
		t.Fatal("expected entry to still exist after MarkOff")
	}
	if entry.Status != "off" {
		t.Errorf("Status = %q, want %q", entry.Status, "off")
	}
	if !entry.LastEventTime.Equal(offTime) {
		t.Errorf("LastEventTime = %v, want %v", entry.LastEventTime, offTime)
	}
	// Tgid should be preserved
	if entry.Tgid != 200 {
		t.Errorf("Tgid = %d, want 200 (should not change on MarkOff)", entry.Tgid)
	}
}

func TestAffiliationMap_MarkOffMissing(t *testing.T) {
	m := newAffiliationMap()
	// Should not panic
	m.MarkOff(affiliationKey{SystemID: 1, UnitID: 999}, time.Now())
}

func TestAffiliationMap_UpdateActivity(t *testing.T) {
	m := newAffiliationMap()
	key := affiliationKey{SystemID: 1, UnitID: 100}
	now := time.Now()

	m.Update(key, &affiliationEntry{
		SystemID:      1,
		UnitID:        100,
		Tgid:          200,
		Status:        "affiliated",
		LastEventTime: now,
	})

	later := now.Add(10 * time.Second)
	m.UpdateActivity(key, later)

	entry, _ := m.Get(key)
	if !entry.LastEventTime.Equal(later) {
		t.Errorf("LastEventTime = %v, want %v", entry.LastEventTime, later)
	}
	// Other fields unchanged
	if entry.Tgid != 200 {
		t.Errorf("Tgid = %d, want 200", entry.Tgid)
	}
	if entry.Status != "affiliated" {
		t.Errorf("Status = %q, want %q", entry.Status, "affiliated")
	}
}

func TestAffiliationMap_UpdateActivityMissing(t *testing.T) {
	m := newAffiliationMap()
	// Should not panic
	m.UpdateActivity(affiliationKey{SystemID: 1, UnitID: 999}, time.Now())
}

func TestAffiliationMap_All(t *testing.T) {
	m := newAffiliationMap()
	now := time.Now()

	m.Update(affiliationKey{SystemID: 1, UnitID: 100}, &affiliationEntry{
		SystemID: 1, UnitID: 100, Tgid: 200, Status: "affiliated", LastEventTime: now,
	})
	m.Update(affiliationKey{SystemID: 1, UnitID: 101}, &affiliationEntry{
		SystemID: 1, UnitID: 101, Tgid: 300, Status: "affiliated", LastEventTime: now,
	})

	all := m.All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d entries, want 2", len(all))
	}

	// Verify it's a snapshot — mutating doesn't affect original
	all[0].Tgid = 999
	entry, _ := m.Get(affiliationKey{SystemID: 1, UnitID: 100})
	if entry.Tgid == 999 {
		t.Error("mutation of All() snapshot leaked to original map")
	}
}

func TestAffiliationMap_UpdateOverwrites(t *testing.T) {
	m := newAffiliationMap()
	key := affiliationKey{SystemID: 1, UnitID: 100}
	now := time.Now()

	m.Update(key, &affiliationEntry{
		SystemID: 1, UnitID: 100, Tgid: 200, Status: "affiliated", LastEventTime: now,
	})

	// Re-affiliate to a different talkgroup
	later := now.Add(5 * time.Second)
	m.Update(key, &affiliationEntry{
		SystemID: 1, UnitID: 100, Tgid: 300, Status: "affiliated", LastEventTime: later,
	})

	entry, _ := m.Get(key)
	if entry.Tgid != 300 {
		t.Errorf("Tgid = %d, want 300 after overwrite", entry.Tgid)
	}
}

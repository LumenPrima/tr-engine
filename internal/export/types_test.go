package export

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSystemRecordP25_RoundTrip(t *testing.T) {
	rec := SystemRecord{V: 1, Type: "p25", Name: "Butler/Warren", Sysid: "348", Wacn: "BEE00"}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["_v"] != float64(1) {
		t.Errorf("expected _v=1, got %v", raw["_v"])
	}

	var decoded SystemRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Sysid != "348" || decoded.Wacn != "BEE00" {
		t.Errorf("round-trip failed: got sysid=%q wacn=%q", decoded.Sysid, decoded.Wacn)
	}
}

func TestSystemRecordConventional_HasSites(t *testing.T) {
	rec := SystemRecord{
		V: 1, Type: "conventional", Name: "Local Fire",
		Sites: []SiteRef{{InstanceID: "trunk-recorder", ShortName: "local_fire"}},
	}
	data, _ := json.Marshal(rec)
	var decoded SystemRecord
	json.Unmarshal(data, &decoded)
	if len(decoded.Sites) != 1 || decoded.Sites[0].ShortName != "local_fire" {
		t.Errorf("conventional system sites not preserved")
	}
}

func TestTalkgroupRecord_RoundTrip(t *testing.T) {
	prio := 1
	now := time.Now().UTC().Truncate(time.Second)
	rec := TalkgroupRecord{
		V: 1, SystemRef: SystemRef{Sysid: "348", Wacn: "BEE00"},
		Tgid: 24513, AlphaTag: "BC Fire", AlphaTagSource: "manual",
		Tag: "Fire Dispatch", Group: "Butler County", Description: "Fire/EMS",
		Mode: "D", Priority: &prio, FirstSeen: &now, LastSeen: &now,
	}
	data, _ := json.Marshal(rec)
	var decoded TalkgroupRecord
	json.Unmarshal(data, &decoded)
	if decoded.Tgid != 24513 || decoded.AlphaTagSource != "manual" {
		t.Errorf("talkgroup round-trip failed")
	}
	if decoded.Priority == nil || *decoded.Priority != 1 {
		t.Errorf("priority not preserved")
	}
}

func TestUnitRecord_RoundTrip(t *testing.T) {
	rec := UnitRecord{
		V: 1, SystemRef: SystemRef{Sysid: "348", Wacn: "BEE00"},
		UnitID: 12345, AlphaTag: "Engine 1", AlphaTagSource: "csv",
	}
	data, _ := json.Marshal(rec)
	var decoded UnitRecord
	json.Unmarshal(data, &decoded)
	if decoded.UnitID != 12345 || decoded.AlphaTagSource != "csv" {
		t.Errorf("unit round-trip failed")
	}
}

func TestSystemRecordP25_OmitsEmptyFields(t *testing.T) {
	rec := SystemRecord{V: 1, Type: "p25", Name: "Test", Sysid: "348", Wacn: "BEE00"}
	data, _ := json.Marshal(rec)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["sites"]; ok {
		t.Error("P25 system should not have sites field")
	}
}

func TestTalkgroupDirectoryRecord_RoundTrip(t *testing.T) {
	prio := 5
	rec := TalkgroupDirectoryRecord{
		V: 1, SystemRef: SystemRef{Sysid: "348", Wacn: "BEE00"},
		Tgid: 100, AlphaTag: "Test TG", Mode: "D",
		Description: "Test desc", Tag: "Fire", Category: "Public Safety",
		Priority: &prio,
	}
	data, _ := json.Marshal(rec)
	var decoded TalkgroupDirectoryRecord
	json.Unmarshal(data, &decoded)
	if decoded.Category != "Public Safety" || decoded.Priority == nil || *decoded.Priority != 5 {
		t.Errorf("talkgroup directory round-trip failed: %+v", decoded)
	}
}

func TestCallRecord_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dur := float32(45.2)
	freq := int64(851250000)
	rec := CallRecord{
		V:         1,
		SystemRef: SystemRef{Sysid: "348", Wacn: "BEE00"},
		SiteRef:   &SiteRef{InstanceID: "tr-1", ShortName: "butco"},
		Tgid:      24513,
		StartTime: now,
		Duration:  &dur,
		Freq:      &freq,
		Emergency: true,
		SrcList:   json.RawMessage(`[{"src":12345}]`),
		FreqList:  json.RawMessage(`[{"freq":851250000}]`),
		UnitIDs:   []int{12345, 67890},
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CallRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Tgid != 24513 || !decoded.Emergency || decoded.SiteRef == nil {
		t.Errorf("call round-trip failed: %+v", decoded)
	}
	if decoded.SiteRef.ShortName != "butco" {
		t.Errorf("site_ref not preserved")
	}
	if len(decoded.UnitIDs) != 2 {
		t.Errorf("unit_ids not preserved")
	}
}

func TestTranscriptionRecord_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	conf := float32(0.92)
	rec := TranscriptionRecord{
		V:             1,
		SystemRef:     SystemRef{Sysid: "348", Wacn: "BEE00"},
		Tgid:          24513,
		CallStartTime: now,
		Text:          "Engine 5 responding",
		Source:        "auto",
		IsPrimary:     true,
		Confidence:    &conf,
		Language:      "en",
		Model:         "whisper-large-v3-turbo",
		WordCount:     3,
	}
	data, _ := json.Marshal(rec)
	var decoded TranscriptionRecord
	json.Unmarshal(data, &decoded)
	if decoded.Source != "auto" || decoded.Text != "Engine 5 responding" {
		t.Errorf("transcription round-trip failed")
	}
	if decoded.Confidence == nil || *decoded.Confidence != 0.92 {
		t.Errorf("confidence not preserved")
	}
}

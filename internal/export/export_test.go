package export

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

func TestWriteJSONL_CreatesValidTarEntry(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	err := writeJSONL(tw, "test.jsonl", func(enc *json.Encoder) error {
		return enc.Encode(map[string]string{"key": "value"})
	})
	if err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gz.Close()

	gr, _ := gzip.NewReader(&buf)
	tr := tar.NewReader(gr)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != "test.jsonl" {
		t.Errorf("expected name test.jsonl, got %s", hdr.Name)
	}
	data, _ := io.ReadAll(tr)
	if !bytes.Contains(data, []byte(`"key":"value"`)) {
		t.Errorf("expected JSON content, got %s", data)
	}
}

func TestWriteJSON_CreatesValidTarEntry(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	m := Manifest{Version: 1, Format: "tr-engine-export", Counts: ManifestCounts{Systems: 2}}
	if err := writeJSON(tw, "manifest.json", m); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gz.Close()

	gr, _ := gzip.NewReader(&buf)
	tr := tar.NewReader(gr)
	tr.Next()
	data, _ := io.ReadAll(tr)

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Version != 1 || decoded.Counts.Systems != 2 {
		t.Errorf("manifest round-trip failed: %+v", decoded)
	}
}

func TestWriteJSONL_EmptyCallback(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	err := writeJSONL(tw, "empty.jsonl", func(enc *json.Encoder) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gz.Close()

	gr, _ := gzip.NewReader(&buf)
	tr := tar.NewReader(gr)
	hdr, _ := tr.Next()
	if hdr.Size != 0 {
		t.Errorf("expected empty file, got size %d", hdr.Size)
	}
}

func TestBuildSystemRef_P25(t *testing.T) {
	ref := buildSystemRef(database.SystemAPI{Sysid: "348", Wacn: "BEE00", SystemType: "p25"})
	if ref.Sysid != "348" || ref.Wacn != "BEE00" {
		t.Errorf("expected P25 ref, got %+v", ref)
	}
}

func TestBuildSystemRef_Conventional(t *testing.T) {
	ref := buildSystemRef(database.SystemAPI{Sysid: "0", SystemType: "conventional"})
	if ref.Sysid != "" || ref.Wacn != "" {
		t.Errorf("expected empty ref for conventional, got %+v", ref)
	}
}

func TestBuildSystemRecord_ConventionalHasSites(t *testing.T) {
	sys := database.SystemAPI{
		SystemType: "conventional", Name: "Test",
		Sites: []database.SiteAPI{
			{InstanceID: "tr-1", ShortName: "test"},
		},
	}
	rec := buildSystemRecord(sys)
	if len(rec.Sites) != 1 || rec.Sites[0].ShortName != "test" {
		t.Errorf("conventional system should have inline sites")
	}
}

func TestBuildSystemRecord_P25NoSites(t *testing.T) {
	sys := database.SystemAPI{
		SystemType: "p25", Sysid: "348", Wacn: "BEE00",
		Sites: []database.SiteAPI{
			{InstanceID: "tr-1", ShortName: "test"},
		},
	}
	rec := buildSystemRecord(sys)
	if len(rec.Sites) != 0 {
		t.Errorf("P25 system should not have inline sites")
	}
}

func TestBuildCallRecord(t *testing.T) {
	siteID := 42
	stopTime := time.Date(2025, 6, 15, 12, 0, 30, 0, time.UTC)
	dur := float32(12.5)

	ce := database.CallExport{
		SystemID:      1,
		SiteID:        &siteID,
		Tgid:          101,
		StartTime:     time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
		StopTime:      &stopTime,
		Duration:      &dur,
		AudioType:     "m4a",
		AudioFilePath: "2025/06/call.m4a",
		Emergency:     true,
		Phase2TDMA:    true,
		PatchedTgids:  []int32{200, 201},
		UnitIDs:       []int32{5001, 5002, 5003},
		InstanceID:    "tr-1",
	}

	sysRef := SystemRef{Sysid: "348", Wacn: "BEE00"}
	siteIDMap := map[int]database.Site{
		42: {SiteID: 42, SystemID: 1, InstanceID: "tr-1", ShortName: "butco"},
	}

	rec := buildCallRecord(ce, sysRef, siteIDMap)

	// Basic field mapping
	if rec.V != 1 {
		t.Errorf("expected V=1, got %d", rec.V)
	}
	if rec.Tgid != 101 {
		t.Errorf("expected Tgid=101, got %d", rec.Tgid)
	}
	if !rec.StartTime.Equal(ce.StartTime) {
		t.Errorf("StartTime mismatch")
	}
	if rec.StopTime == nil || !rec.StopTime.Equal(stopTime) {
		t.Errorf("StopTime mismatch")
	}
	if rec.Duration == nil || *rec.Duration != 12.5 {
		t.Errorf("Duration mismatch")
	}
	if rec.AudioType != "m4a" {
		t.Errorf("expected AudioType=m4a, got %s", rec.AudioType)
	}
	if rec.AudioFilePath != "2025/06/call.m4a" {
		t.Errorf("expected AudioFilePath=2025/06/call.m4a, got %s", rec.AudioFilePath)
	}
	if !rec.Emergency {
		t.Error("expected Emergency=true")
	}
	if !rec.Phase2TDMA {
		t.Error("expected Phase2TDMA=true")
	}
	if rec.InstanceID != "tr-1" {
		t.Errorf("expected InstanceID=tr-1, got %s", rec.InstanceID)
	}

	// SystemRef
	if rec.SystemRef.Sysid != "348" || rec.SystemRef.Wacn != "BEE00" {
		t.Errorf("SystemRef mismatch: %+v", rec.SystemRef)
	}

	// SiteRef resolved from siteIDMap
	if rec.SiteRef == nil {
		t.Fatal("expected SiteRef to be populated")
	}
	if rec.SiteRef.InstanceID != "tr-1" {
		t.Errorf("expected SiteRef.InstanceID=tr-1, got %s", rec.SiteRef.InstanceID)
	}
	if rec.SiteRef.ShortName != "butco" {
		t.Errorf("expected SiteRef.ShortName=butco, got %s", rec.SiteRef.ShortName)
	}

	// int32 → []int conversion for PatchedTgids
	if len(rec.PatchedTgids) != 2 || rec.PatchedTgids[0] != 200 || rec.PatchedTgids[1] != 201 {
		t.Errorf("PatchedTgids mismatch: %v", rec.PatchedTgids)
	}

	// int32 → []int conversion for UnitIDs
	if len(rec.UnitIDs) != 3 || rec.UnitIDs[0] != 5001 || rec.UnitIDs[1] != 5002 || rec.UnitIDs[2] != 5003 {
		t.Errorf("UnitIDs mismatch: %v", rec.UnitIDs)
	}
}

func TestBuildCallRecord_NoSite(t *testing.T) {
	ce := database.CallExport{
		SystemID:  1,
		SiteID:    nil,
		Tgid:      101,
		StartTime: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
		AudioType: "m4a",
	}

	sysRef := SystemRef{Sysid: "348", Wacn: "BEE00"}
	siteIDMap := map[int]database.Site{
		42: {SiteID: 42, SystemID: 1, InstanceID: "tr-1", ShortName: "butco"},
	}

	rec := buildCallRecord(ce, sysRef, siteIDMap)

	if rec.SiteRef != nil {
		t.Errorf("expected SiteRef to be nil when SiteID is nil, got %+v", rec.SiteRef)
	}
	if rec.Tgid != 101 {
		t.Errorf("expected Tgid=101, got %d", rec.Tgid)
	}
}

package export

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"

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

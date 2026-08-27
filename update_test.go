package mewsync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestAssetNameFor(t *testing.T) {
	if got := assetNameFor("1.2.3", "linux", "amd64"); got != "mewsync_1.2.3_linux_amd64.tar.gz" {
		t.Fatalf("got %q", got)
	}
	if got := assetNameFor("1.2.3", "darwin", "arm64"); got != "mewsync_1.2.3_darwin_arm64.tar.gz" {
		t.Fatalf("got %q", got)
	}
	if got := assetNameFor("1.2.3", "windows", "amd64"); got != "mewsync_1.2.3_windows_amd64.zip" {
		t.Fatalf("got %q", got)
	}
}

func TestFindAsset(t *testing.T) {
	assets := []releaseAsset{
		{Name: "mewsync_1.0.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/a"},
		{Name: "mewsync_1.0.0_checksums.txt", BrowserDownloadURL: "https://example.com/b"},
	}
	a, ok := findAsset(assets, "mewsync_1.0.0_linux_amd64.tar.gz")
	if !ok || a.BrowserDownloadURL != "https://example.com/a" {
		t.Fatalf("expected match, got %+v ok=%v", a, ok)
	}
	if _, ok := findAsset(assets, "nope"); ok {
		t.Fatal("expected no match")
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello world")
	sum := sha256.Sum256(data)
	sumHex := hex.EncodeToString(sum[:])
	checksums := sumHex + "  mewsync_1.0.0_linux_amd64.tar.gz\n" +
		"deadbeef  some_other_file.tar.gz\n"

	if err := verifyChecksum(data, checksums, "mewsync_1.0.0_linux_amd64.tar.gz"); err != nil {
		t.Fatalf("expected valid checksum, got %v", err)
	}
	if err := verifyChecksum([]byte("tampered"), checksums, "mewsync_1.0.0_linux_amd64.tar.gz"); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if err := verifyChecksum(data, checksums, "missing_file.tar.gz"); err == nil {
		t.Fatal("expected missing entry error")
	}
}

func buildTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o755}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("write content: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestExtractTarGzBinary(t *testing.T) {
	archive := buildTarGz(t, map[string][]byte{
		"mewsync_1.0.0_linux_amd64/README.md": []byte("readme"),
		"mewsync_1.0.0_linux_amd64/mewsync":   []byte("fake binary contents"),
	})

	out, err := extractTarGzBinary(archive, "mewsync")
	if err != nil {
		t.Fatalf("extractTarGzBinary: %v", err)
	}
	if string(out) != "fake binary contents" {
		t.Fatalf("got %q", out)
	}

	if _, err := extractTarGzBinary(archive, "mewsync.exe"); err == nil {
		t.Fatal("expected not-found error for missing file")
	}
}

func TestBinaryName(t *testing.T) {
	if binaryName("windows") != "mewsync.exe" {
		t.Fatal("expected mewsync.exe on windows")
	}
	if binaryName("linux") != "mewsync" {
		t.Fatal("expected mewsync on linux")
	}
}

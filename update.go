package mewsync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Version is the running binary's version, set at build time via
// -ldflags "-X mewsync.Version=x.y.z" (defaults to "dev").
var Version = "dev"

const (
	updateOwner = "arshnah"
	updateRepo  = "mewsync"
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

func fetchLatestRelease() (releaseInfo, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", updateOwner, updateRepo)
	resp, err := doRequest("GET", u, map[string]string{"Accept": "application/vnd.github+json"}, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("update check responded %d", resp.StatusCode)
	}
	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return releaseInfo{}, err
	}
	return rel, nil
}

// CheckForUpdate compares Version against the latest GitHub release and
// returns a human-readable status line.
func CheckForUpdate() (string, error) {
	rel, err := fetchLatestRelease()
	if err != nil {
		return "", err
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(Version, "v")

	cmp, err := compareSemver(latest, current)
	if err != nil {
		return fmt.Sprintf("could not compare versions (latest %s, running %s)", latest, current), nil
	}
	if cmp > 0 {
		return fmt.Sprintf("update available: v%s (running v%s). Download: %s", latest, current, rel.HTMLURL), nil
	}
	return fmt.Sprintf("up to date (v%s)", current), nil
}

func compareSemver(a, b string) (int, error) {
	pa, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1, nil
			}
			return -1, nil
		}
	}
	return 0, nil
}

func parseSemver(v string) ([3]int, error) {
	var out [3]int
	v = strings.SplitN(v, "-", 2)[0]
	parts := strings.SplitN(v, ".", 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, fmt.Errorf("bad version segment %q: %w", parts[i], err)
		}
		out[i] = n
	}
	return out, nil
}

// assetNameFor returns the release asset filename mewsync's release workflow
// is expected to publish for the given version/OS/arch, following the same
// goreleaser-style naming the packaging/ manifests already assume:
// mewsync_<version>_<os>_<arch>.tar.gz, or .zip on Windows.
func assetNameFor(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("mewsync_%s_%s_%s.%s", version, goos, goarch, ext)
}

// checksumsAssetName is the goreleaser-default checksums file name.
func checksumsAssetName(version string) string {
	return fmt.Sprintf("mewsync_%s_checksums.txt", version)
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return releaseAsset{}, false
}

// verifyChecksum checks data's sha256 against the line for filename inside a
// goreleaser-style checksums.txt (lines shaped "<hex sha256>  <filename>").
func verifyChecksum(data []byte, checksumsText, filename string) error {
	want := ""
	for _, line := range strings.Split(checksumsText, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == filename {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s", filename)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: want %s, got %s", filename, want, got)
	}
	return nil
}

// binaryName is the executable's expected name inside the release archive.
func binaryName(goos string) string {
	if goos == "windows" {
		return "mewsync.exe"
	}
	return "mewsync"
}

// extractTarGzBinary pulls a single named file out of a gzip-compressed tar
// archive, as produced by goreleaser's default archive format.
func extractTarGzBinary(data []byte, wantName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if filepath.Base(hdr.Name) != wantName {
			continue
		}
		out, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %s from archive: %w", wantName, err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s not found in archive", wantName)
}

func downloadAsset(url string) ([]byte, error) {
	resp, err := doRequest("GET", url, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download responded %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ApplyUpdate downloads and installs the latest release in place.
//
// On Linux and macOS this replaces the running executable: the new binary is
// written to a temp file next to it and atomically renamed over it, which
// works even while the old binary is still running because the process keeps
// its open file handle on the old inode under the old name until it exits.
// The caller has to restart the process to pick up the new binary.
//
// On Windows, replacing a running executable's file isn't reliably possible
// (the OS keeps the file locked while it's mapped into the running process),
// so this just reports where to download the installer instead of attempting
// an in-place swap.
func ApplyUpdate() (string, error) {
	rel, err := fetchLatestRelease()
	if err != nil {
		return "", err
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(Version, "v")
	cmp, err := compareSemver(latest, current)
	if err == nil && cmp <= 0 {
		return fmt.Sprintf("already up to date (v%s)", current), nil
	}

	if runtime.GOOS == "windows" {
		return fmt.Sprintf("update available: v%s (running v%s). mewsync can't safely replace"+
			" its own running .exe on Windows, download and run the installer: %s",
			latest, current, rel.HTMLURL), nil
	}

	assetName := assetNameFor(latest, runtime.GOOS, runtime.GOARCH)
	asset, ok := findAsset(rel.Assets, assetName)
	if !ok {
		return "", fmt.Errorf("no release asset named %s found for v%s (release: %s)", assetName, latest, rel.HTMLURL)
	}

	data, err := downloadAsset(asset.BrowserDownloadURL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", assetName, err)
	}

	if sums, ok := findAsset(rel.Assets, checksumsAssetName(latest)); ok {
		sumsData, err := downloadAsset(sums.BrowserDownloadURL)
		if err != nil {
			return "", fmt.Errorf("download checksums: %w", err)
		}
		if err := verifyChecksum(data, string(sumsData), assetName); err != nil {
			return "", fmt.Errorf("update aborted: %w", err)
		}
	}

	binData, err := extractTarGzBinary(data, binaryName(runtime.GOOS))
	if err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}

	if err := replaceRunningExecutable(binData); err != nil {
		return "", err
	}

	return fmt.Sprintf("updated to v%s, restart mewsync to use it", latest), nil
}

// replaceRunningExecutable writes data to a temp file next to the current
// executable and renames it over the original. Rename is atomic on the same
// filesystem, and both Linux and macOS allow replacing a file that's
// currently executing (the running process keeps the old inode open under
// its old, now-unlinked name).
func replaceRunningExecutable(data []byte) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolve running executable path: %w", err)
	}

	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".mewsync-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		return fmt.Errorf("replace %s: %w", exePath, err)
	}
	return nil
}

package app

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ScanResults struct {
	HardenedFeatures   []string         `json:"hardened_features,omitempty"`
	AntiEnvironment    []string         `json:"anti_environment,omitempty"`
	ThirdPartySDKs     []string         `json:"third_party_sdks,omitempty"`
	AntiProxy          []string         `json:"anti_proxy_detection,omitempty"`
	EmbeddedAPKs       []string         `json:"embedded_apks,omitempty"`
	Certificates       []CertificateRef `json:"certificates,omitempty"`
	KeyLeaks           []KeyLeakFinding `json:"key_leaks,omitempty"`
	Signature          SignatureSummary `json:"signature"`
	InnerAPKCount      int              `json:"inner_apk_count"`
	DetectionNotes     []string         `json:"detection_notes,omitempty"`
	SkippedLargeFiles  []string         `json:"skipped_large_files,omitempty"`
	ScanningExceptions []string         `json:"exceptions,omitempty"`
}

type CertificateRef struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type KeyLeakFinding struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
	Snippet string `json:"snippet"`
}

type SignatureSummary struct {
	HasV2Signature bool   `json:"has_v2_signature"`
	JanusRisk      bool   `json:"janus_risk"`
	Details        string `json:"details,omitempty"`
	Error          string `json:"error,omitempty"`
}

var (
	hardenerLibs = map[string]string{
		"libjiagu.so":         "360 加固",
		"libjgdaemon.so":      "360 加固守护",
		"libtosprotection.so": "腾讯乐固",
		"libshellx.so":        "爱加密",
		"libprotectClass.so":  "梆梆加固",
		"libnsdark.so":        "娜迦壳",
	}
	thirdPartySDKLibs = map[string]string{
		"libflutter.so":        "Flutter 引擎",
		"libreactnativejni.so": "ReactNative",
		"libweibosdkcore.so":   "微博 SDK",
		"libBugly.so":          "Bugly",
		"libumeng-spy.so":      "友盟 SDK",
	}
	antiEnvKeywords = []string{
		"frida", "xposed", "magisk", "rootcloak", "busybox", "supersu", "genymotion", "virtualbox",
		"ro.secure", "ro.debuggable", "ro.secureboot.lockstate", "fridaserver",
	}
	antiProxyKeywords = []string{
		"isNetworkProxy", "ProxyDetector", "getProxyHost", "http.proxyHost", "checkVpn", "isVpnUsed",
	}
	keyLeakPatterns = []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`(?i)(?:secret|access|private)[-_ ]?(?:key|token)\s*[:=]\s*[A-Za-z0-9+/=\-]{8,}`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	}
	certExtensions = []string{".cer", ".crt", ".pem", ".p12", ".pfx", ".der"}
	textExtensions = []string{".xml", ".json", ".txt", ".ini", ".properties", ".js", ".html", ".cfg", ".conf", ".yml", ".yaml"}
)

func scanAPK(apkPath string, cfg ScanningConfig) (*ScanResults, error) {
	reader, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	result := &ScanResults{
		HardenedFeatures: []string{},
		AntiEnvironment:  []string{},
		ThirdPartySDKs:   []string{},
		AntiProxy:        []string{},
		EmbeddedAPKs:     []string{},
		Certificates:     []CertificateRef{},
		KeyLeaks:         []KeyLeakFinding{},
	}

	hardenedSet := map[string]struct{}{}
	sdkSet := map[string]struct{}{}
	antiEnvSet := map[string]struct{}{}
	antiProxySet := map[string]struct{}{}
	keyLeakSet := map[string]struct{}{}

	maxBytes := int64(cfg.MaxScanSizeMB) * 1024 * 1024

	for _, file := range reader.File {
		lower := strings.ToLower(file.Name)

		// Hardened libs detection
		if strings.HasPrefix(lower, "lib/") && strings.HasSuffix(lower, ".so") {
			for lib, label := range hardenerLibs {
				if strings.Contains(lower, lib) {
					hardenedSet[label] = struct{}{}
				}
			}
			for lib, label := range thirdPartySDKLibs {
				if strings.Contains(lower, lib) {
					sdkSet[label] = struct{}{}
				}
			}
		}

		if strings.HasSuffix(lower, ".apk") && lower != filepath.ToSlash(filepath.Base(apkPath)) {
			result.EmbeddedAPKs = append(result.EmbeddedAPKs, file.Name)
		}

		for _, ext := range certExtensions {
			if strings.HasSuffix(lower, ext) {
				if certRef, err := extractCertInfo(file); err == nil {
					result.Certificates = append(result.Certificates, certRef)
				} else {
					result.ScanningExceptions = append(result.ScanningExceptions, fmt.Sprintf("cert %s: %v", file.Name, err))
				}
				break
			}
		}

		if strings.HasSuffix(lower, ".dex") {
			data, err := readZipLimited(file, maxBytes)
			if err != nil {
				result.ScanningExceptions = append(result.ScanningExceptions, fmt.Sprintf("dex %s: %v", file.Name, err))
				continue
			}
			content := bytes.ToLower(data)
			for _, kw := range antiEnvKeywords {
				if bytes.Contains(content, []byte(strings.ToLower(kw))) {
					antiEnvSet[kw] = struct{}{}
				}
			}
			for _, kw := range antiProxyKeywords {
				if bytes.Contains(content, []byte(strings.ToLower(kw))) {
					antiProxySet[kw] = struct{}{}
				}
			}
		} else if shouldScanKeyLeaks(lower, file.UncompressedSize64) {
			data, err := readZipLimited(file, maxBytes)
			if err != nil {
				result.SkippedLargeFiles = append(result.SkippedLargeFiles, file.Name)
				continue
			}
			content := string(data)
			for _, pattern := range keyLeakPatterns {
				loc := pattern.FindStringIndex(content)
				if loc != nil {
					snippet := extractSnippet(content, loc[0], loc[1])
					key := file.Name + pattern.String()
					if _, exists := keyLeakSet[key]; !exists {
						result.KeyLeaks = append(result.KeyLeaks, KeyLeakFinding{
							Path:    file.Name,
							Pattern: pattern.String(),
							Snippet: snippet,
						})
						keyLeakSet[key] = struct{}{}
					}
				}
			}
			for _, hint := range cfg.KeyLeakHints {
				if strings.Contains(strings.ToLower(content), strings.ToLower(hint)) {
					key := file.Name + hint
					if _, exists := keyLeakSet[key]; !exists {
						idx := strings.Index(strings.ToLower(content), strings.ToLower(hint))
						snippet := extractSnippet(content, idx, idx+len(hint))
						result.KeyLeaks = append(result.KeyLeaks, KeyLeakFinding{
							Path:    file.Name,
							Pattern: hint,
							Snippet: snippet,
						})
						keyLeakSet[key] = struct{}{}
					}
				}
			}
		}
	}

	result.HardenedFeatures = mapKeysSorted(hardenedSet)
	result.ThirdPartySDKs = mapKeysSorted(sdkSet)
	result.AntiEnvironment = mapKeysSorted(antiEnvSet)
	result.AntiProxy = mapKeysSorted(antiProxySet)
	result.InnerAPKCount = len(result.EmbeddedAPKs)

	result.Signature = verifySignature(apkPath, cfg.ApksignerPath)

	return result, nil
}

func readZipLimited(file *zip.File, limit int64) ([]byte, error) {
	if limit > 0 && file.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%s exceeds scan limit %d bytes", file.Name, limit)
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var reader io.Reader = rc
	if limit > 0 {
		reader = io.LimitReader(rc, limit)
	}
	return io.ReadAll(reader)
}

func extractCertInfo(file *zip.File) (CertificateRef, error) {
	rc, err := file.Open()
	if err != nil {
		return CertificateRef{}, err
	}
	defer rc.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, rc); err != nil {
		return CertificateRef{}, err
	}
	return CertificateRef{
		Path:   file.Name,
		Size:   int64(file.UncompressedSize64),
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func shouldScanKeyLeaks(path string, size uint64) bool {
	lower := strings.ToLower(path)
	for _, ext := range textExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	if size <= 512*1024 {
		return true
	}
	return false
}

func extractSnippet(content string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	left := start - 40
	if left < 0 {
		left = 0
	}
	right := end + 40
	if right > len(content) {
		right = len(content)
	}
	snippet := content[left:right]
	snippet = strings.ReplaceAll(snippet, "\n", "\\n")
	return snippet
}

func mapKeysSorted(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func verifySignature(apkPath, apksigner string) SignatureSummary {
	if apksigner == "" {
		return SignatureSummary{
			JanusRisk: true,
			Error:     "apksigner path not configured",
		}
	}
	cmd := exec.Command(apksigner, "verify", "--verbose", "--print-certs", apkPath)
	output, err := cmd.CombinedOutput()
	text := string(output)
	summary := SignatureSummary{
		Details: text,
	}
	if err != nil {
		summary.JanusRisk = true
		summary.Error = err.Error()
		return summary
	}
	lower := strings.ToLower(text)
	hasV2 := strings.Contains(lower, "apk signature scheme v2") && strings.Contains(lower, ": true")
	summary.HasV2Signature = hasV2
	summary.JanusRisk = !hasV2
	return summary
}

package app

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type protectionMetadata struct {
	RandomPackage string             `json:"random_package,omitempty"`
	PreviousPkg   string             `json:"previous_package,omitempty"`
	EncryptionKey string             `json:"encryption_key,omitempty"`
	DexEncrypted  []encryptionRecord `json:"dex_encrypted,omitempty"`
	Pseudo        *pseudoRecord      `json:"pseudo,omitempty"`
}

type encryptionRecord struct {
	File        string `json:"file"`
	Nonce       string `json:"nonce"`
	Artifact    string `json:"artifact,omitempty"`
	Compressed  bool   `json:"compressed,omitempty"`
	PlainBytes  int    `json:"plain_bytes,omitempty"`
	CipherBytes int    `json:"cipher_bytes,omitempty"`
}

type pseudoRecord struct {
	Source   string `json:"source"`
	Key      string `json:"key"`
	Artifact string `json:"artifact"`
	Bytes    int    `json:"bytes"`
}

func (t *Tool) runProtections(currentApk, baseName, runDir string, report *Report) (string, error) {
	cfg := t.cfg.Protections
	if !cfg.Enabled {
		return "", nil
	}
	step := ReportStep{Name: "protections"}
	defer func() {
		report.Steps = append(report.Steps, step)
	}()

	extractDir, err := os.MkdirTemp(runDir, "apk-edit-")
	if err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return "", err
	}
	defer os.RemoveAll(extractDir)

	entryMethods, err := unzipArchive(currentApk, extractDir)
	if err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return "", fmt.Errorf("unzip apk: %w", err)
	}

	meta := protectionMetadata{}
	modified := false

	if cfg.RandomPackage {
		manifestPath := filepath.Join(extractDir, "AndroidManifest.xml")
		prevPkg, newPkg, err := randomizeManifestPackage(manifestPath, cfg.PackagePrefix)
		if err != nil {
			step.Status = "failed"
			step.Details = err.Error()
			return "", fmt.Errorf("randomize package: %w", err)
		}
		meta.PreviousPkg = prevPkg
		meta.RandomPackage = newPkg
		modified = true
	}

	var key []byte
	var keyB64 string
	if cfg.DexEncrypt || cfg.MultiDexEncrypt || cfg.PseudoEncrypt {
		key, keyB64, err = deriveKey(cfg.EncryptionSecret)
		if err != nil {
			step.Status = "failed"
			step.Details = err.Error()
			return "", err
		}
		meta.EncryptionKey = keyB64
	}

	if cfg.DexEncrypt || cfg.MultiDexEncrypt {
		records, err := encryptDexFiles(extractDir, key, cfg.MultiDexEncrypt, cfg.CompressBeforeEncrypt)
		if err != nil {
			step.Status = "failed"
			step.Details = err.Error()
			return "", err
		}
		if len(records) > 0 {
			meta.DexEncrypted = records
			modified = true
		}
	}

	if cfg.PseudoEncrypt {
		record, err := createPseudoEncryption(extractDir)
		if err != nil {
			step.Status = "failed"
			step.Details = err.Error()
			return "", err
		}
		meta.Pseudo = record
		modified = true
	}

	if !modified {
		step.Status = "skipped"
		return "", nil
	}

	metadataPath, err := persistMetadata(extractDir, runDir, baseName, meta)
	if err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return "", err
	}
	report.Artifacts["protection_metadata"] = metadataPath

	newApk := filepath.Join(runDir, baseName+"-protected.apk")
	if err := zipDirectory(extractDir, newApk, entryMethods); err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return "", fmt.Errorf("repack apk: %w", err)
	}

	step.Status = "completed"
	step.Artifact = newApk
	return newApk, nil
}

func unzipArchive(src, dest string) (map[string]uint16, error) {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	methods := make(map[string]uint16, len(reader.File))
	for _, file := range reader.File {
		methods[file.Name] = file.Method
		if err := extractZipFile(file, dest); err != nil {
			return nil, err
		}
	}
	return methods, nil
}

func extractZipFile(file *zip.File, dest string) error {
	cleanName, err := cleanZipEntryName(file.Name)
	if err != nil {
		return fmt.Errorf("invalid zip entry: %s", file.Name)
	}
	targetPath := filepath.Join(dest, cleanName)
	if file.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return err
	}
	return nil
}

func cleanZipEntryName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty zip entry")
	}
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") || filepath.IsAbs(name) || filepath.IsAbs(normalized) {
		return "", fmt.Errorf("unsafe zip entry")
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe zip entry")
		}
	}
	cleanName := filepath.ToSlash(filepath.Clean(normalized))
	if cleanName == "." {
		return "", fmt.Errorf("unsafe zip entry")
	}
	return filepath.FromSlash(cleanName), nil
}

func zipDirectory(srcDir, dest string, originalMethods map[string]uint16) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := zip.NewWriter(out)
	writer.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})
	defer writer.Close()

	var files []string
	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(files)

	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		method := zip.Deflate
		if storedMethod, ok := originalMethods[rel]; ok {
			method = storedMethod
		}
		// Android R+ requires resources.arsc and native libs to remain uncompressed
		// so zipalign can produce installable artifacts.
		if strings.HasPrefix(rel, "lib/") || rel == "resources.arsc" {
			method = zip.Store
		}
		header.Method = method
		w, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, file); err != nil {
			file.Close()
			return err
		}
		file.Close()
	}
	return writer.Close()
}

func deriveKey(secret string) ([]byte, string, error) {
	if secret != "" {
		sum := sha256.Sum256([]byte(secret))
		return sum[:], base64.StdEncoding.EncodeToString(sum[:]), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	return buf, base64.StdEncoding.EncodeToString(buf), nil
}

func encryptDexFiles(root string, key []byte, encryptAll bool, compressBeforeEncrypt bool) ([]encryptionRecord, error) {
	dexFiles, err := findDexFiles(root)
	if err != nil {
		return nil, err
	}
	if len(dexFiles) == 0 {
		return nil, nil
	}
	targets := dexFiles
	if !encryptAll {
		targets = targets[:1]
	}
	destDir := filepath.Join(root, "assets", "protector", "dex")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}

	var records []encryptionRecord
	for _, rel := range targets {
		abs := filepath.Join(root, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		plainSize := len(data)
		if compressBeforeEncrypt {
			compressed, compErr := deflateBytes(data)
			if compErr != nil {
				return nil, compErr
			}
			data = compressed
		}
		ciphertext, nonce, err := encryptBytes(data, key)
		if err != nil {
			return nil, err
		}
		outName := strings.ReplaceAll(rel, "/", "_") + ".enc"
		dest := filepath.Join(destDir, outName)
		if err := os.WriteFile(dest, ciphertext, 0o644); err != nil {
			return nil, err
		}
		records = append(records, encryptionRecord{
			File:        rel,
			Nonce:       base64.StdEncoding.EncodeToString(nonce),
			Artifact:    filepath.ToSlash(filepath.Join("assets", "protector", "dex", outName)),
			Compressed:  compressBeforeEncrypt,
			PlainBytes:  plainSize,
			CipherBytes: len(ciphertext),
		})
	}
	return records, nil
}

func findDexFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "classes") && strings.HasSuffix(base, ".dex") {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func encryptBytes(data, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, data, nil)
	return ciphertext, nonce, nil
}

func deflateBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func createPseudoEncryption(root string) (*pseudoRecord, error) {
	manifestPath := filepath.Join(root, "AndroidManifest.xml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("pseudo encryption requires AndroidManifest.xml: %w", err)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	for i := range data {
		out[i] = data[i] ^ key[i%len(key)]
	}
	target := filepath.Join(root, "assets", "protector")
	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, err
	}
	filePath := filepath.Join(target, "manifest.pseudo")
	if err := os.WriteFile(filePath, out, 0o644); err != nil {
		return nil, err
	}
	return &pseudoRecord{
		Source:   "AndroidManifest.xml",
		Key:      base64.StdEncoding.EncodeToString(key),
		Artifact: "assets/protector/manifest.pseudo",
		Bytes:    len(data),
	}, nil
}

func persistMetadata(root, runDir, baseName string, meta protectionMetadata) (string, error) {
	if meta.RandomPackage == "" && len(meta.DexEncrypted) == 0 && meta.Pseudo == nil {
		return "", nil
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}
	targetDir := filepath.Join(root, "assets", "protector")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	internalPath := filepath.Join(targetDir, "metadata.json")
	if err := os.WriteFile(internalPath, data, 0o644); err != nil {
		return "", err
	}
	exportPath := filepath.Join(runDir, baseName+"-metadata.json")
	if err := os.WriteFile(exportPath, data, 0o644); err != nil {
		return "", err
	}
	return exportPath, nil
}

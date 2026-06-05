package app

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"unicode/utf16"
)

const (
	resXMLType          = 0x0003
	resStringPoolType   = 0x0001
	resResourceMapType  = 0x0180
	resStartElementType = 0x0102
	resValueTypeString  = 0x03
)

type manifestEditor struct {
	data             []byte
	stringPool       *stringPool
	packageStringIdx int
	packageName      string
}

type stringPool struct {
	data         []byte
	chunkStart   int
	stringsStart int
	count        int
	offsets      []int
	isUTF8       bool
}

type stringEntry struct {
	value     string
	dataStart int
	byteLen   int
	charLen   int
	isUTF8    bool
}

func randomizeManifestPackage(manifestPath, prefix string) (string, string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", "", err
	}
	editor, err := parseManifest(data)
	if err != nil {
		return "", "", err
	}
	oldName := editor.packageName
	if oldName == "" {
		return "", "", fmt.Errorf("manifest missing package attribute")
	}
	targetLen := len(oldName)
	if targetLen == 0 {
		targetLen = 10
	}
	newName := generatePackageName(prefix, targetLen, oldName)
	if err := editor.replacePackage(newName); err != nil {
		return "", "", err
	}
	if _, err := editor.rewritePackageLinkedValues(oldName, newName); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(manifestPath, editor.data, 0o644); err != nil {
		return "", "", err
	}
	return oldName, newName, nil
}

func parseManifest(data []byte) (*manifestEditor, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("manifest too small")
	}
	if binary.LittleEndian.Uint16(data[0:2]) != resXMLType {
		return nil, fmt.Errorf("unexpected XML chunk type")
	}
	headerSize := int(binary.LittleEndian.Uint16(data[2:4]))
	offset := headerSize
	if offset <= 0 || offset >= len(data) {
		return nil, fmt.Errorf("invalid XML header size")
	}

	sp, nextOffset, err := parseStringPool(data, offset)
	if err != nil {
		return nil, err
	}
	offset = nextOffset

	// optional resource map
	if offset+8 <= len(data) && binary.LittleEndian.Uint16(data[offset:offset+2]) == resResourceMapType {
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += chunkSize
	}

	packageIdx := -1
	packageName := ""
	for offset+8 <= len(data) {
		chunkType := binary.LittleEndian.Uint16(data[offset : offset+2])
		headerSize := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if chunkSize <= 0 || offset+chunkSize > len(data) {
			break
		}
		if chunkType == resStartElementType {
			if offset+headerSize > len(data) {
				return nil, fmt.Errorf("start element header exceeds bounds")
			}
			nameIdx := int(binary.LittleEndian.Uint32(data[offset+20 : offset+24]))
			name, ok := sp.safeString(nameIdx)
			if !ok {
				offset += chunkSize
				continue
			}
			if name == "manifest" {
				attrStart := int(binary.LittleEndian.Uint16(data[offset+24 : offset+26]))
				attrSize := int(binary.LittleEndian.Uint16(data[offset+26 : offset+28]))
				attrCount := int(binary.LittleEndian.Uint16(data[offset+28 : offset+30]))
				const nodeHeaderSize = 16
				attrOffset := offset + nodeHeaderSize + attrStart
				for i := 0; i < attrCount; i++ {
					cur := attrOffset + i*attrSize
					if cur+20 > offset+chunkSize {
						break
					}
					attrNameIdx := int(binary.LittleEndian.Uint32(data[cur+4 : cur+8]))
					attrName, ok := sp.safeString(attrNameIdx)
					if !ok {
						continue
					}
					if attrName != "package" {
						continue
					}
					rawValue := int32(binary.LittleEndian.Uint32(data[cur+8 : cur+12]))
					valueType := data[cur+14]
					valueData := int32(binary.LittleEndian.Uint32(data[cur+16 : cur+20]))
					switch {
					case rawValue != -1:
						packageIdx = int(rawValue)
					case valueType == resValueTypeString:
						packageIdx = int(valueData)
					default:
						return nil, fmt.Errorf("manifest package attribute missing string value")
					}
					if pkg, ok := sp.safeString(packageIdx); ok {
						value := pkg
						packageName = value
					}
					break
				}
				break
			}
		}
		offset += chunkSize
	}

	if packageIdx < 0 {
		return nil, fmt.Errorf("package attribute not found")
	}

	return &manifestEditor{
		data:             data,
		stringPool:       sp,
		packageStringIdx: packageIdx,
		packageName:      packageName,
	}, nil
}

func parseStringPool(data []byte, offset int) (*stringPool, int, error) {
	if offset+28 > len(data) {
		return nil, 0, fmt.Errorf("string pool truncated")
	}
	if binary.LittleEndian.Uint16(data[offset:offset+2]) != resStringPoolType {
		return nil, 0, fmt.Errorf("expected string pool chunk")
	}
	headerSize := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
	chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
	if chunkSize <= 0 || offset+chunkSize > len(data) {
		return nil, 0, fmt.Errorf("invalid string pool size")
	}
	if offset+headerSize > len(data) {
		return nil, 0, fmt.Errorf("string pool header overflow")
	}
	stringCount := int(binary.LittleEndian.Uint32(data[offset+8 : offset+12]))
	flags := binary.LittleEndian.Uint32(data[offset+16 : offset+20])
	stringsStart := int(binary.LittleEndian.Uint32(data[offset+20 : offset+24]))
	isUTF8 := flags&0x100 != 0

	cursor := offset + 28
	offsets := make([]int, stringCount)
	for i := 0; i < stringCount; i++ {
		if cursor+4 > len(data) {
			return nil, 0, fmt.Errorf("string offset table overflow")
		}
		offsets[i] = int(binary.LittleEndian.Uint32(data[cursor : cursor+4]))
		cursor += 4
	}

	return &stringPool{
		data:         data,
		chunkStart:   offset,
		stringsStart: stringsStart,
		count:        stringCount,
		offsets:      offsets,
		isUTF8:       isUTF8,
	}, offset + chunkSize, nil
}

func (sp *stringPool) getString(index int) (string, error) {
	entry, err := sp.getEntry(index)
	if err != nil {
		return "", err
	}
	return entry.value, nil
}

func (sp *stringPool) safeString(index int) (string, bool) {
	if index < 0 || index >= sp.count {
		return "", false
	}
	entry, err := sp.getEntry(index)
	if err != nil {
		return "", false
	}
	return entry.value, true
}

func (sp *stringPool) getEntry(index int) (*stringEntry, error) {
	if index < 0 || index >= sp.count {
		return nil, fmt.Errorf("string index %d out of range", index)
	}
	pos := sp.chunkStart + sp.stringsStart + sp.offsets[index]
	if pos >= len(sp.data) {
		return nil, fmt.Errorf("string offset beyond file")
	}
	if sp.isUTF8 {
		return readUTF8Entry(sp.data, pos)
	}
	return readUTF16Entry(sp.data, pos)
}

func (sp *stringPool) replace(entry *stringEntry, value string) error {
	if entry.isUTF8 {
		if len(value) != entry.byteLen {
			return fmt.Errorf("utf8 replacement must occupy %d bytes", entry.byteLen)
		}
		copy(sp.data[entry.dataStart:entry.dataStart+entry.byteLen], []byte(value))
		return nil
	}
	runes := []rune(value)
	if len(runes) != entry.charLen {
		return fmt.Errorf("utf16 replacement must contain %d runes", entry.charLen)
	}
	buf := make([]byte, entry.byteLen)
	encoded := utf16.Encode(runes)
	for i, r := range encoded {
		binary.LittleEndian.PutUint16(buf[i*2:], r)
	}
	copy(sp.data[entry.dataStart:entry.dataStart+entry.byteLen], buf)
	return nil
}

func (e *manifestEditor) replacePackage(newValue string) error {
	entry, err := e.stringPool.getEntry(e.packageStringIdx)
	if err != nil {
		return err
	}
	if len(newValue) != entry.charLen {
		return fmt.Errorf("package name must keep length %d, got %d", entry.charLen, len(newValue))
	}
	if err := e.stringPool.replace(entry, newValue); err != nil {
		return err
	}
	e.packageName = newValue
	return nil
}

// rewritePackageLinkedValues updates package-derived identifiers in selected manifest attributes
// (e.g. provider authorities/custom permissions) while avoiding component class names.
func (e *manifestEditor) rewritePackageLinkedValues(oldPkg, newPkg string) (int, error) {
	if oldPkg == "" || newPkg == "" || oldPkg == newPkg {
		return 0, nil
	}
	if len(oldPkg) != len(newPkg) {
		return 0, fmt.Errorf("package name length mismatch: %d != %d", len(oldPkg), len(newPkg))
	}

	headerSize := int(binary.LittleEndian.Uint16(e.data[2:4]))
	offset := headerSize
	if offset <= 0 || offset >= len(e.data) {
		return 0, fmt.Errorf("invalid XML header size")
	}

	_, nextOffset, err := parseStringPool(e.data, offset)
	if err != nil {
		return 0, err
	}
	offset = nextOffset

	if offset+8 <= len(e.data) && binary.LittleEndian.Uint16(e.data[offset:offset+2]) == resResourceMapType {
		chunkSize := int(binary.LittleEndian.Uint32(e.data[offset+4 : offset+8]))
		offset += chunkSize
	}

	replacements := make(map[int]string)
	for offset+8 <= len(e.data) {
		chunkType := binary.LittleEndian.Uint16(e.data[offset : offset+2])
		headerSize := int(binary.LittleEndian.Uint16(e.data[offset+2 : offset+4]))
		chunkSize := int(binary.LittleEndian.Uint32(e.data[offset+4 : offset+8]))
		if chunkSize <= 0 || offset+chunkSize > len(e.data) {
			break
		}
		if chunkType == resStartElementType {
			if offset+headerSize > len(e.data) {
				return 0, fmt.Errorf("start element header exceeds bounds")
			}

			elementNameIdx := int(binary.LittleEndian.Uint32(e.data[offset+20 : offset+24]))
			elementName, ok := e.stringPool.safeString(elementNameIdx)
			if !ok {
				offset += chunkSize
				continue
			}

			attrStart := int(binary.LittleEndian.Uint16(e.data[offset+24 : offset+26]))
			attrSize := int(binary.LittleEndian.Uint16(e.data[offset+26 : offset+28]))
			attrCount := int(binary.LittleEndian.Uint16(e.data[offset+28 : offset+30]))
			if attrSize <= 0 {
				offset += chunkSize
				continue
			}

			const nodeHeaderSize = 16
			attrOffset := offset + nodeHeaderSize + attrStart
			for i := 0; i < attrCount; i++ {
				cur := attrOffset + i*attrSize
				if cur+20 > offset+chunkSize {
					break
				}
				attrNameIdx := int(binary.LittleEndian.Uint32(e.data[cur+4 : cur+8]))
				attrName, ok := e.stringPool.safeString(attrNameIdx)
				if !ok || !shouldRewritePackageAttribute(elementName, attrName) {
					continue
				}

				valueIdx, ok := attrStringIndex(e.data, cur)
				if !ok {
					continue
				}
				value, ok := e.stringPool.safeString(valueIdx)
				if !ok {
					continue
				}
				rewritten, changed := rewritePackageReferenceValue(value, oldPkg, newPkg)
				if !changed {
					continue
				}
				replacements[valueIdx] = rewritten
			}
		}
		offset += chunkSize
	}

	updated := 0
	for idx, value := range replacements {
		entry, err := e.stringPool.getEntry(idx)
		if err != nil {
			return updated, err
		}
		if len(value) != entry.charLen {
			return updated, fmt.Errorf("replacement length mismatch at string index %d", idx)
		}
		if err := e.stringPool.replace(entry, value); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func attrStringIndex(data []byte, attrOffset int) (int, bool) {
	rawValue := int32(binary.LittleEndian.Uint32(data[attrOffset+8 : attrOffset+12]))
	valueType := data[attrOffset+14]
	valueData := int32(binary.LittleEndian.Uint32(data[attrOffset+16 : attrOffset+20]))
	switch {
	case rawValue != -1:
		return int(rawValue), true
	case valueType == resValueTypeString:
		return int(valueData), true
	default:
		return 0, false
	}
}

func shouldRewritePackageAttribute(elementName, attrName string) bool {
	if attrName == "name" {
		switch elementName {
		case "permission", "permission-group", "permission-tree", "uses-permission", "uses-permission-sdk-23", "uses-permission-sdk-m":
			return true
		default:
			return false
		}
	}
	switch attrName {
	case "authorities", "permission", "readPermission", "writePermission", "sharedUserId", "taskAffinity", "process":
		return true
	default:
		return false
	}
}

func rewritePackageReferenceValue(value, oldPkg, newPkg string) (string, bool) {
	if oldPkg == "" || newPkg == "" || !strings.Contains(value, oldPkg) {
		return "", false
	}
	rewritten := strings.ReplaceAll(value, oldPkg, newPkg)
	if rewritten == value {
		return "", false
	}
	return rewritten, true
}

func readUTF16Entry(data []byte, offset int) (*stringEntry, error) {
	charLen, used, err := readLength16(data, offset)
	if err != nil {
		return nil, err
	}
	offset += used
	byteLen := charLen * 2
	if offset+byteLen+2 > len(data) {
		return nil, fmt.Errorf("utf16 entry exceeds data")
	}
	raw := data[offset : offset+byteLen]
	u16 := make([]uint16, charLen)
	for i := 0; i < charLen; i++ {
		u16[i] = binary.LittleEndian.Uint16(raw[i*2 : i*2+2])
	}
	value := string(utf16.Decode(u16))
	return &stringEntry{
		value:     value,
		dataStart: offset,
		byteLen:   byteLen,
		charLen:   charLen,
		isUTF8:    false,
	}, nil
}

func readUTF8Entry(data []byte, offset int) (*stringEntry, error) {
	_, used, err := readLength8(data, offset)
	if err != nil {
		return nil, err
	}
	offset += used
	byteLen, used2, err := readLength8(data, offset)
	if err != nil {
		return nil, err
	}
	offset += used2
	if offset+byteLen+1 > len(data) {
		return nil, fmt.Errorf("utf8 entry exceeds data")
	}
	raw := data[offset : offset+byteLen]
	value := string(raw)
	return &stringEntry{
		value:     value,
		dataStart: offset,
		byteLen:   byteLen,
		charLen:   len(value),
		isUTF8:    true,
	}, nil
}

func readLength16(data []byte, offset int) (int, int, error) {
	start := offset
	if offset+2 > len(data) {
		return 0, 0, fmt.Errorf("invalid utf16 length")
	}
	first := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2
	if first&0x8000 != 0 {
		if offset+2 > len(data) {
			return 0, 0, fmt.Errorf("invalid utf16 length (extended)")
		}
		second := binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
		length := int(uint32(first&0x7FFF)<<16 | uint32(second))
		return length, offset - start, nil
	}
	return int(first), offset - start, nil
}

func readLength8(data []byte, offset int) (int, int, error) {
	start := offset
	if offset >= len(data) {
		return 0, 0, fmt.Errorf("invalid utf8 length")
	}
	first := int(data[offset])
	offset++
	if first&0x80 != 0 {
		if offset >= len(data) {
			return 0, 0, fmt.Errorf("invalid utf8 length (extended)")
		}
		second := int(data[offset])
		offset++
		length := ((first & 0x7F) << 8) | second
		return length, offset - start, nil
	}
	return first, offset - start, nil
}

func generatePackageName(prefix string, targetLen int, seed string) string {
	if targetLen < 3 {
		targetLen = 3
	}
	const minRandomChars = 4
	base := sanitizePrefix(prefix)
	if base == "" {
		base = "com.protector"
	}
	base = strings.Trim(base, ".")

	randomChars := minRandomChars
	maxRandom := targetLen - 1
	if maxRandom < 1 {
		maxRandom = 1
	}
	if randomChars > maxRandom {
		randomChars = maxRandom
	}
	maxBaseLen := targetLen - randomChars
	if maxBaseLen < 1 {
		maxBaseLen = 1
	}
	if len(base) > maxBaseLen {
		base = strings.Trim(base[:maxBaseLen], ".")
	}
	if base == "" {
		base = "a"
	}

	hashSeed := seed + "|" + base
	hash := sha256.Sum256([]byte(hashSeed))
	hashPos := 0
	nextChar := func() byte {
		v := hash[hashPos%len(hash)]
		hashPos++
		return pkgChars[int(v)%len(pkgChars)]
	}

	buf := []byte{}
	for i := 0; i < len(base) && len(buf) < targetLen; i++ {
		buf = append(buf, base[i])
	}
	if len(buf) < targetLen-randomChars {
		if len(buf) > 0 && buf[len(buf)-1] != '.' {
			buf = append(buf, '.')
		}
	}
	for len(buf) < targetLen {
		buf = append(buf, nextChar())
	}
	for len(buf) > 0 && buf[len(buf)-1] == '.' {
		buf = buf[:len(buf)-1]
	}
	if len(buf) == 0 {
		buf = []byte("com.secured.app")
	}
	if len(buf) > targetLen {
		buf = buf[:targetLen]
	}
	for len(buf) < targetLen {
		buf = append(buf, nextChar())
	}
	ensureValidPackage(buf)
	return string(buf)
}

func sanitizePrefix(prefix string) string {
	var b strings.Builder
	for i := 0; i < len(prefix); i++ {
		ch := prefix[i]
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteByte(ch)
		case ch >= 'A' && ch <= 'Z':
			b.WriteByte(ch + 32)
		case ch >= '0' && ch <= '9':
			b.WriteByte(ch)
		case ch == '.':
			b.WriteByte(ch)
		}
	}
	return b.String()
}

var pkgChars = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

// ensureValidPackage enforces: first char is letter; each segment (after '.') starts with letter;
// no trailing '.'; all chars are [a-z0-9.] with length preserved.
func ensureValidPackage(buf []byte) {
	if len(buf) == 0 {
		return
	}
	if buf[0] < 'a' || buf[0] > 'z' {
		buf[0] = 'a'
	}
	for i := 1; i < len(buf); i++ {
		if buf[i] == '.' {
			// avoid consecutive or trailing dot
			if buf[i-1] == '.' || i == len(buf)-1 {
				buf[i] = 'a'
			}
			continue
		}
		if buf[i-1] == '.' {
			if buf[i] < 'a' || buf[i] > 'z' {
				buf[i] = 'a'
			}
			continue
		}
		if !((buf[i] >= 'a' && buf[i] <= 'z') || (buf[i] >= '0' && buf[i] <= '9')) {
			buf[i] = 'a'
		}
	}
}

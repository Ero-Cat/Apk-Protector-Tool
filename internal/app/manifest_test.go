package app

import "testing"

func TestReadLength16Extended(t *testing.T) {
	data := []byte{0x01, 0x80, 0x34, 0x12}

	got, used, err := readLength16(data, 0)
	if err != nil {
		t.Fatalf("readLength16() error = %v", err)
	}
	want := (1 << 16) | 0x1234
	if got != want {
		t.Fatalf("readLength16() length = %d, want %d", got, want)
	}
	if used != 4 {
		t.Fatalf("readLength16() used = %d, want 4", used)
	}
}

func TestShouldRewritePackageAttribute(t *testing.T) {
	tests := []struct {
		name        string
		elementName string
		attrName    string
		want        bool
	}{
		{
			name:        "provider authorities should rewrite",
			elementName: "provider",
			attrName:    "authorities",
			want:        true,
		},
		{
			name:        "receiver permission should rewrite",
			elementName: "receiver",
			attrName:    "permission",
			want:        true,
		},
		{
			name:        "uses permission name should rewrite",
			elementName: "uses-permission",
			attrName:    "name",
			want:        true,
		},
		{
			name:        "activity name should not rewrite",
			elementName: "activity",
			attrName:    "name",
			want:        false,
		},
		{
			name:        "meta data name should not rewrite",
			elementName: "meta-data",
			attrName:    "name",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRewritePackageAttribute(tt.elementName, tt.attrName)
			if got != tt.want {
				t.Fatalf("shouldRewritePackageAttribute(%q, %q) = %v, want %v", tt.elementName, tt.attrName, got, tt.want)
			}
		})
	}
}

func TestRewritePackageReferenceValue(t *testing.T) {
	oldPkg := "net.ahwater.fxt"
	newPkg := "com.protector.z"

	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "provider authority",
			input:   "net.ahwater.fxt.fileprovider",
			want:    "com.protector.z.fileprovider",
			changed: true,
		},
		{
			name:    "custom permission",
			input:   "net.ahwater.fxt.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION",
			want:    "com.protector.z.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION",
			changed: true,
		},
		{
			name:    "unrelated value",
			input:   "android.permission.INTERNET",
			want:    "",
			changed: false,
		},
		{
			name:    "multiple occurrences",
			input:   "net.ahwater.fxt:net.ahwater.fxt",
			want:    "com.protector.z:com.protector.z",
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := rewritePackageReferenceValue(tt.input, oldPkg, newPkg)
			if changed != tt.changed {
				t.Fatalf("rewritePackageReferenceValue(%q) changed = %v, want %v", tt.input, changed, tt.changed)
			}
			if got != tt.want {
				t.Fatalf("rewritePackageReferenceValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeneratePackageNameStableForSameSeed(t *testing.T) {
	prefix := "com.protector"
	targetLen := len("net.ahwater.fxt")
	seed := "net.ahwater.fxt"

	first := generatePackageName(prefix, targetLen, seed)
	second := generatePackageName(prefix, targetLen, seed)
	if first != second {
		t.Fatalf("generatePackageName should be stable, got %q and %q", first, second)
	}
	if len(first) != targetLen {
		t.Fatalf("generatePackageName length = %d, want %d", len(first), targetLen)
	}
}

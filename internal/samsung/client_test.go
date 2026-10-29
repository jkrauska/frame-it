package samsung

import "testing"

func TestIsLegacyArtAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    bool
	}{
		{"0.97", true},     // 2018 Frame (LS03N)
		{"1.07", true},     // 2017 Frame (LS003)
		{"2.03", false},    // 2021 Frame (LS03A)
		{"4.3.4.0", false}, // 2022 Frame (LS03B)
		{"5.0.0", false},
		{"", false},
		{"unknown", false},
	}

	for _, tc := range tests {
		if got := isLegacyArtAPI(tc.version); got != tc.want {
			t.Errorf("isLegacyArtAPI(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

// The D2D request and socket header take the lowercase form; the legacy
// single-message upload takes the upper-cased one.
func TestFileType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path       string
		wantD2D    string
		wantLegacy string
	}{
		{"photo.jpg", "jpg", "JPEG"},
		{"photo.jpeg", "jpg", "JPEG"},
		{"PHOTO.JPG", "jpg", "JPEG"},
		{"art.png", "png", "PNG"},
		{"/tmp/dir.d/art.PNG", "png", "PNG"},
	}

	for _, tc := range tests {
		got := fileTypeFromPath(tc.path)
		if got != tc.wantD2D {
			t.Errorf("fileTypeFromPath(%q) = %q, want %q", tc.path, got, tc.wantD2D)
		}
		if legacy := fileTypeHeader(got); legacy != tc.wantLegacy {
			t.Errorf("fileTypeHeader(%q) = %q, want %q", got, legacy, tc.wantLegacy)
		}
	}
}

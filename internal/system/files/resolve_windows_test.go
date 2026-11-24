package files

import "testing"

func TestVolumeRootKeepsTheTrailingSeparator(t *testing.T) {
	for _, path := range []string{
		`\\?\Volume{11111111-2222-3333-4444-555555555555}`,
		`\\?\Volume{11111111-2222-3333-4444-555555555555}\`,
		`\\?\C:`,
		`\\?\C:\`,
		`C:`,
		`C:\`,
		`\\server\share`,
		`\\server\share\`,
	} {
		got := volumeRoot(path)
		if got == "" || got[len(got)-1] != '\\' {
			t.Errorf("volumeRoot(%q) = %q, want a trailing separator", path, got)
		}
	}
}

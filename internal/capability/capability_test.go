package capability

import "testing"

func TestCapabilityIdentifier(t *testing.T) {
	for _, identifier := range []string{"filesystem.read", "shell.execute", "github.pr.merge"} {
		capability, err := New(identifier)
		if err != nil || !capability.Valid() {
			t.Errorf("expected %q to be valid, capability=%q err=%v", identifier, capability, err)
		}
	}
	for _, identifier := range []string{
		"", "Filesystem.Read", "filesystem-read", "filesystem_read",
		"filesystem..read", " shell.execute", "filesystem.read ",
	} {
		if _, err := New(identifier); err == nil {
			t.Errorf("expected %q to be rejected", identifier)
		}
	}
}

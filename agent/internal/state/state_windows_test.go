package state

import (
	"testing"

	"golang.org/x/sys/windows"
)

// checkPrivate verifies the directory DACL is protected (no inherited
// "Users: read" from ProgramData) and has exactly the three expected entries.
func checkPrivate(t *testing.T, dir string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Error("DACL inherits entries from the parent directory")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl.AceCount != 3 {
		t.Errorf("DACL has %d entries, want 3 (SYSTEM, Administrators, current user)", dacl.AceCount)
	}
}

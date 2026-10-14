package state

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// DefaultDir is where the agent service keeps its state. ProgramData is
// readable by all users by default, so secureDir replaces the inherited ACL.
const DefaultDir = `C:\ProgramData\SmartCluster\agent`

// secureDir gives full access to SYSTEM, Administrators and the current user
// (SYSTEM when running as the service) and removes inherited entries.
func secureDir(dir string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sddl := fmt.Sprintf("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;%s)", user.User.Sid.String())
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

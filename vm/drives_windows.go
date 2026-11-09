//go:build windows

package vm

import "golang.org/x/sys/windows"

// fixedDrive — lecteur fixe (ni réseau, ni CD, ni amovible) : scanner la
// racine d'un lecteur RÉSEAU en profondeur 3, c'est des minutes de stall
// SMB au premier lancement — le « scan qui freeze ».
func fixedDrive(d string) bool {
	p, err := windows.UTF16PtrFromString(d)
	if err != nil {
		return false
	}
	switch windows.GetDriveType(p) {
	case windows.DRIVE_FIXED, windows.DRIVE_RAMDISK:
		return true
	}
	return false
}

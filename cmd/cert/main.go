package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW  = shell32.NewProc("ShellExecuteW")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

// isAdmin checks if the current process has administrator privileges
func isAdmin() bool {
	var sid *windows.SID

	// Use windows package for SID initialization
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	// Check if the current token is a member of the Admin group
	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

// runMeElevated attempts to restart the current process with "runas" (UAC prompt)
func runMeElevated() {
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString("")

	// Execute with "runas" verb to trigger UAC
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(argPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(1), // SW_SHOWNORMAL
	)

	if ret <= 32 {
		fmt.Printf("Error: Elevation failed or declined (Code: %d)\n", ret)
		fmt.Println("Press Enter to exit...")
		fmt.Scanln()
		os.Exit(1)
	}
}

func main() {
	if !isAdmin() {
		fmt.Println("Requesting administrator privileges...")
		runMeElevated()
		os.Exit(0)
	}
	setConsoleOutputCP.Call(65001)

	fmt.Println("Running with administrator privileges.")

	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error: Could not get executable path: %v\n", err)
		return
	}

	tempCertPath := filepath.Join(os.TempDir(), "cert.cer")
	psScript := fmt.Sprintf(
		"$sig = Get-AuthenticodeSignature '%s'; "+
			"if($sig.SignerCertificate) { "+
			"[System.IO.File]::WriteAllBytes('%s', $sig.SignerCertificate.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Cert)) "+
			"} else { exit 1 }",
		exePath, tempCertPath,
	)

	extractCmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	fmt.Println(extractCmd)
	if err := extractCmd.Run(); err != nil {
		fmt.Println("Error: Could not extract certificate.")
		return
	}

	cmd := exec.Command("certutil", "-addstore", "-f", "TrustedPeople", tempCertPath)
	fmt.Println(cmd)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Run()

	fmt.Println("Process complete. Press Enter to exit...")
	fmt.Scanln()
}

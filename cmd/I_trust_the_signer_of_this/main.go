//go:build windows

package main

import (
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW  = shell32.NewProc("ShellExecuteW")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	setConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

func isAdmin() bool {
	var sid *windows.SID
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
	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func runMeElevated() {
	fmt.Println("Requesting administrator privileges...")
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()
	verbPtr, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	cwdPtr, _ := windows.UTF16PtrFromString(cwd)

	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		0,
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(1),
	)

	if ret <= 32 {
		fmt.Printf("Error: Failed to elevate privileges (Code: %d)\n", ret)
		os.Exit(1)
	}
}

func extractCertFromExe(exePath string) ([]byte, error) {
	pathPtr, _ := windows.UTF16PtrFromString(exePath)
	var encoding, contentType, formatType uint32
	var hStore, hMsg windows.Handle

	err := windows.CryptQueryObject(
		windows.CERT_QUERY_OBJECT_FILE,
		unsafe.Pointer(pathPtr),
		windows.CERT_QUERY_CONTENT_FLAG_PKCS7_SIGNED_EMBED,
		windows.CERT_QUERY_FORMAT_FLAG_BINARY,
		0,
		&encoding,
		&contentType,
		&formatType,
		&hStore,
		&hMsg,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("file is not signed or read error: %v", err)
	}
	defer windows.CertCloseStore(hStore, 0)

	pCertContext, err := windows.CertEnumCertificatesInStore(hStore, nil)
	if err != nil {
		return nil, fmt.Errorf("certificate not found: %v", err)
	}

	certData := make([]byte, pCertContext.Length)
	copy(certData, (*[1 << 20]byte)(unsafe.Pointer(pCertContext.EncodedCert))[:pCertContext.Length])

	return certData, nil
}

func printFormattedCertInfo(certData []byte) {
	cert, err := x509.ParseCertificate(certData)
	if err != nil {
		fmt.Println("Error parsing certificate:", err)
		return
	}

	fmt.Printf("Serial Number: %x\n", cert.SerialNumber)
	fmt.Printf("Issuer: %s\n", cert.Issuer.String())
	fmt.Printf(" NotBefore: %s\n", cert.NotBefore.Local().Format("02.01.2006 15:04"))
	fmt.Printf(" NotAfter: %s\n", cert.NotAfter.Local().Format("02.01.2006 15:04"))
	fmt.Printf("Subject: %s\n", cert.Subject.String())
	hash := sha1.Sum(cert.Raw)
	fmt.Printf("Cert Hash(sha1): %x\n", hash)
}

func main() {
	if !isAdmin() {
		runMeElevated()
		os.Exit(0)
	}
	setConsoleOutputCP.Call(65001)

	defer func() {
		fmt.Println("\nPress Enter to exit...")
		fmt.Scanln()
	}()

	exePath, err := os.Executable()
	if err != nil {
		fmt.Println(err)
		return
	}

	certData, err := extractCertFromExe(exePath)
	if err != nil {
		fmt.Println(err)
		return
	}

	printFormattedCertInfo(certData)
	fmt.Println()

	tempCertPath := filepath.Join(os.TempDir(), "cert.cer")
	err = os.WriteFile(tempCertPath, certData, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.Remove(tempCertPath)

	fmt.Println("Installing certificate to 'TrustedPeople'...")
	cmd := exec.Command("certutil", "-addstore", "-f", "TrustedPeople", tempCertPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

# Get the digital signature from this script file
$scriptPath = $PSCommandPath
$signature = Get-AuthenticodeSignature -FilePath $scriptPath

# Check if the script is actually signed
if ($signature.Status -eq "Valid" -and $signature.SignerCertificate) {
    try {
        # The certificate object is extracted directly from the signature
        $cert = $signature.SignerCertificate
        
        Write-Host "Certificate Details:" -ForegroundColor Cyan
        Write-Host "  Subject: $($cert.Subject)" -ForegroundColor Gray
        Write-Host "  Thumbprint: $($cert.Thumbprint)" -ForegroundColor Gray
        
        # Open TrustedPeople store for CurrentUser
        $store = New-Object System.Security.Cryptography.X509Certificates.X509Store("TrustedPeople", "CurrentUser")
        $store.Open("ReadWrite")
        
        # Check if certificate already exists
        $existingCerts = $store.Certificates.Find(
            [System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,
            $cert.Thumbprint,
            $false
        )
        
        if ($existingCerts.Count -eq 0) {
            # Add the certificate to the store
            $store.Add($cert)
            Write-Host "`nSuccess: The certificate has been installed into TrustedPeople store of Current User." -ForegroundColor Green
        } 
        else {
            Write-Host "`nInfo: Certificate already exists in TrustedPeople store." -ForegroundColor Yellow
        }
        
        $store.Close()
    }
    catch {
        Write-Host "`nError: Failed to install certificate." -ForegroundColor Red
        Write-Host "Exception: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "Stack Trace: $($_.Exception.StackTrace)" -ForegroundColor DarkGray
    }
}
else {
    Write-Host "Error: This script is not digitally signed or signature is invalid." -ForegroundColor Yellow
    Write-Host "Signature Status: $($signature.Status)" -ForegroundColor Gray
    
    if ($signature.Status -eq "NotSigned") {
        Write-Host "`nTo sign the script, use:" -ForegroundColor Cyan
        Write-Host "Set-AuthenticodeSignature -FilePath `"$scriptPath`" -Certificate `$(Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert)[0]" -ForegroundColor Gray
    }
}
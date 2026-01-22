$scriptPath = $PSCommandPath
$signature = Get-AuthenticodeSignature -FilePath $scriptPath

if ($signature.Status -eq "NotSigned") {
    Write-Host "Error: This script is not digitally signed." -ForegroundColor Yellow
    Write-Host "Signature Status: $($signature.Status)" -ForegroundColor Gray
}
else {
    try {
        $cert = $signature.SignerCertificate
        
        Write-Host "Certificate Details:" -ForegroundColor Cyan
        Write-Host "  Subject: $($cert.Subject)" -ForegroundColor Gray
        Write-Host "  Thumbprint: $($cert.Thumbprint)" -ForegroundColor Gray
        
        $store = New-Object System.Security.Cryptography.X509Certificates.X509Store("TrustedPeople", "LocalMachine")
        $store.Open("ReadWrite")
        
        $existingCerts = $store.Certificates.Find(
            [System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,
            $cert.Thumbprint,
            $false
        )
        
        if ($existingCerts.Count -eq 0) {
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
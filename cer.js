/*
-----BEGIN CERTIFICATE-----
MIIDmDCCAoCgAwIBAgIJAKOw0eUsuDLVMA0GCSqGSIb3DQEBDAUAMHkxCzAJBgNV
BAYTAlJVMRYwFAYDVQQIEw1Sb3N0b3YgT2JsYXN0MRIwEAYDVQQHEwlNaWxsZXJv
dm8xHDAaBgNVBAoTE0tvbnN0YW50aW4gQWJha3Vtb3YxETAPBgNVBAsTCFBlcnNv
bmFsMQ0wCwYDVQQDEwRjcm9jMCAXDTI2MDExMTEwNDMyNloYDzIwODAxMDE0MTA0
MzI2WjB5MQswCQYDVQQGEwJSVTEWMBQGA1UECBMNUm9zdG92IE9ibGFzdDESMBAG
A1UEBxMJTWlsbGVyb3ZvMRwwGgYDVQQKExNLb25zdGFudGluIEFiYWt1bW92MREw
DwYDVQQLEwhQZXJzb25hbDENMAsGA1UEAxMEY3JvYzCCASIwDQYJKoZIhvcNAQEB
BQADggEPADCCAQoCggEBALsLD9cBnG//hyTBBWvH5SMHHmyCg22Ynq5znSBN+E1G
PDaNg3dY4cEodN+ujdwYhLmo88bFa6FYf7upSjRbHJ9UcoskBfCaNLncxbzvrjjL
swWRrMsxGmKlZHRDo88Mqq0o5Ra00u1nG2Ht98WNvBWCw7aqsLrrtWQ657KmjQEr
WmTrWYqO6zZr8H/H5Iyvde92Sl6fqOq8yaf9HnY6tUgSh8xwfRH8IAs7Jhx2XPd3
hNh5LNMv0U7I07SbuccVi24snn2tryPFvDJ2sVvDysKlLy8napTA3HFR3wjL2X6q
4Ce0jrArKHyD6CIv82OMjI79xLC+9YeQtqo8QoARutcCAwEAAaMhMB8wHQYDVR0O
BBYEFLQVUHz3RSLrH9OR6OFDDC1GI242MA0GCSqGSIb3DQEBDAUAA4IBAQBfUGfc
DmDnlVMRO0vWXpn+rusHE498+vXNENrSGs89zModxyYUALOTOLw7beao4W7MGL2b
N8BmCTYgl7xBAZkKasci/psTDxGMWIgTI8fA73KvitRbe85vaJ2JWqP4U3CHxpEi
9WjPjnqldiz5bG+fRERXXiIacAxcEzoctk2+P0A/bEtCWFjzB+Y64uaksRK30X5c
FkbYjtFG8pHeGLb1zJ3HEiGDtsGr0RkX/tXQlwHzOpv1tIdWv2Gf1B171+1MjLpZ
3k+kzC2CONCyd10HRw5n2D6iL+g5FrWRjinTU2lWPNW+04ibeSbW2mK1r9CkJmXT
mC9IGtskCdvDfPb1
-----END CERTIFICATE-----
*/

var WshShell = new ActiveXObject("WScript.Shell");
var fso = new ActiveXObject("Scripting.FileSystemObject");

function extractCertificate() {
    var scriptPath = WScript.ScriptFullName;
    var file = fso.OpenTextFile(scriptPath, 1);
    var content = file.ReadAll();
    file.Close();
    
    var beginPos = content.indexOf("-----BEGIN CERTIFICATE-----");
    var endPos = content.indexOf("-----END CERTIFICATE-----");
    
    if (beginPos === -1 || endPos === -1) {
        return null;
    }
    
    return content.substring(beginPos, endPos + "-----END CERTIFICATE-----".length);
}

function createTempCertFile(certContent) {
    var tempDir = WshShell.ExpandEnvironmentStrings("%TEMP%");
    var tempFile = tempDir + "\\temp_cert.cer";
    
    var file = fso.CreateTextFile(tempFile, true);
    file.Write(certContent);
    file.Close();
    
    return tempFile;
}

function installCertificate(certFilePath) {
    var command = 'certutil -addstore -f TrustedPeople "' + certFilePath + '"';
    var exitCode = WshShell.Run(command, 0, true);
    
    if (fso.FileExists(certFilePath)) {
        fso.DeleteFile(certFilePath, true);
    }
    
    return exitCode === 0;
}

var certificateContent = extractCertificate();

if (certificateContent) {
    var tempCertFile = createTempCertFile(certificateContent);
    
    if (tempCertFile) {
        var success = installCertificate(tempCertFile);
        
        if (success) {
            WScript.Echo("Certificate installed successfully");
        } else {
            WScript.Echo("Failed to install certificate");
        }
    } else {
        WScript.Echo("Failed to create temporary file");
    }
} else {
    WScript.Echo("Certificate not found in script");
}
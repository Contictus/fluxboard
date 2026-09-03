# Phase 6 admin + api-keys + analytics + observability end-to-end smoke
# (docs/build/PHASE-6-ADMIN-OBS.md §9). Run from the repo root after:
#   make up && make migrate
#
#   powershell -ExecutionPolicy Bypass -File scratchpad/smoke6.ps1
#
# Exit code 0 = all checks passed.

$ErrorActionPreference = 'Stop'

$Api     = $env:SMOKE_API;     if (-not $Api)     { $Api     = 'http://localhost:8080' }
$Mailpit = $env:SMOKE_MAILPIT; if (-not $Mailpit) { $Mailpit = 'http://localhost:8025' }
$V1 = "$Api/api/v1"

# Owner DSN for the adminctl grant (compose owner role on the host-exposed port).
$OwnerDSN = $env:SMOKE_OWNER_DSN
if (-not $OwnerDSN) { $OwnerDSN = 'postgres://fluxboard_owner:owner_pw@localhost:5432/fluxboard?sslmode=disable' }

$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$adminEmail  = "admin$stamp@example.com"
$ownerEmail  = "tenant$stamp@example.com"
$password    = "Sup3r-Secret-Pw!"

function Info($m) { Write-Host "[..] $m" }
function Pass($m) { Write-Host "[OK] $m" -ForegroundColor Green }
function Fail($m) { Write-Host "[XX] $m" -ForegroundColor Red; exit 1 }

function ApiPost($path, $body, $token) {
    $headers = @{}
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    $json = if ($body -ne $null) { $body | ConvertTo-Json -Depth 8 } else { $null }
    return Invoke-RestMethod -Method Post -Uri "$V1$path" -Headers $headers -ContentType 'application/json' -Body $json
}
function ApiGet($path, $token) {
    $headers = @{}
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    return Invoke-RestMethod -Method Get -Uri "$V1$path" -Headers $headers
}

# ExpectStatus runs a request and asserts a specific HTTP status (for negative
# cases: 403 on impersonated writes / read-scoped-key writes).
function ExpectStatus($method, $path, $token, $body, $want) {
    $headers = @{ 'Authorization' = "Bearer $token" }
    $json = if ($body -ne $null) { $body | ConvertTo-Json -Depth 8 } else { $null }
    try {
        Invoke-WebRequest -Method $method -Uri "$V1$path" -Headers $headers -ContentType 'application/json' -Body $json -UseBasicParsing | Out-Null
        return 200
    } catch {
        if ($_.Exception.Response) { return [int]$_.Exception.Response.StatusCode }
        throw
    }
}

function MailpitBody($addr, $subjectLike) {
    for ($i = 0; $i -lt 20; $i++) {
        $msgs = Invoke-RestMethod -Method Get -Uri "$Mailpit/api/v1/messages?limit=50"
        $hit = $msgs.messages | Where-Object {
            (($_.To | ForEach-Object { $_.Address }) -contains $addr) -and
            ((-not $subjectLike) -or ($_.Subject -match $subjectLike))
        } | Select-Object -First 1
        if ($hit) {
            $full = Invoke-RestMethod -Method Get -Uri "$Mailpit/api/v1/message/$($hit.ID)"
            return $full.Text
        }
        Start-Sleep -Milliseconds 500
    }
    return $null
}
function TokenFromQuery($text) { if ($text -match 'token=([A-Za-z0-9._\-]+)') { return $Matches[1] } return $null }

function RegisterVerifyLogin($email) {
    ApiPost '/auth/register' @{ email = $email; password = $password; name = $email.Split('@')[0] } $null | Out-Null
    $body = MailpitBody $email
    if (-not $body) { Fail "no verification email for $email" }
    $tok = TokenFromQuery $body
    ApiPost '/auth/verify-email/confirm' @{ token = $tok } $null | Out-Null
    $login = ApiPost '/auth/login' @{ email = $email; password = $password } $null
    if (-not $login.access_token) { Fail "login returned no access token for $email" }
    return $login.access_token
}

# ---- RFC 6238 TOTP (SHA1, 6 digits, 30s) to drive the admin 2FA gate --------
function ConvertFrom-Base32($s) {
    $s = ($s -replace '=', '').ToUpper()
    $alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
    $bits = ''
    foreach ($c in $s.ToCharArray()) {
        $idx = $alphabet.IndexOf($c)
        if ($idx -lt 0) { continue }
        $bits += [Convert]::ToString($idx, 2).PadLeft(5, '0')
    }
    $bytes = New-Object System.Collections.Generic.List[byte]
    for ($i = 0; $i + 8 -le $bits.Length; $i += 8) {
        $bytes.Add([Convert]::ToByte($bits.Substring($i, 8), 2))
    }
    return , $bytes.ToArray()
}
function Get-Totp($secret) {
    $key = ConvertFrom-Base32 $secret
    $counter = [int64][math]::Floor(([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()) / 30)
    $ctr = [BitConverter]::GetBytes($counter)
    if ([BitConverter]::IsLittleEndian) { [Array]::Reverse($ctr) }
    $hmac = New-Object System.Security.Cryptography.HMACSHA1
    $hmac.Key = $key
    $hash = $hmac.ComputeHash($ctr)
    $offset = $hash[$hash.Length - 1] -band 0x0f
    $bin = (($hash[$offset] -band 0x7f) -shl 24) -bor (($hash[$offset + 1] -band 0xff) -shl 16) -bor (($hash[$offset + 2] -band 0xff) -shl 8) -bor ($hash[$offset + 3] -band 0xff)
    return ('{0:D6}' -f ($bin % 1000000))
}

Info "health check"
if ((Invoke-WebRequest -Uri "$Api/healthz" -UseBasicParsing).StatusCode -ne 200) { Fail "healthz not 200" }
Pass "api healthy"

# ---- Admin bootstrap: register, enroll+activate TOTP, grant platform_role ----
Info "register admin + tenant owner"
$adminTok = RegisterVerifyLogin $adminEmail
$ownerTok = RegisterVerifyLogin $ownerEmail
Pass "two users verified"

Info "admin enrolls + activates TOTP"
$enroll = ApiPost '/auth/2fa/enroll' $null $adminTok
$secret = $enroll.secret
if (-not $secret) { Fail "2fa enroll returned no secret" }
ApiPost '/auth/2fa/activate' @{ code = (Get-Totp $secret) } $adminTok | Out-Null
Pass "admin 2FA active"

Info "grant platform admin via cmd/adminctl"
$env:DATABASE_URL = 'postgres://fluxboard_app:app_pw@localhost:5432/fluxboard?sslmode=disable'
$env:REDIS_ADDR = 'localhost:6379'
$env:DATABASE_URL_MIGRATE = $OwnerDSN
$repoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$backendDir = Join-Path $repoRoot 'backend'
Push-Location $backendDir
$grantOut = & go run ./cmd/adminctl grant $adminEmail 2>&1
$grantOk = $?
Pop-Location
if (-not $grantOk -or ($grantOut -notmatch 'granted')) { Fail "adminctl grant failed: $grantOut" }
Pass "granted platform_role=admin ($grantOut)"

Info "admin re-login through the 2FA step"
$login = ApiPost '/auth/login' @{ email = $adminEmail; password = $password } $null
if ($login.status -ne '2fa_required') { Fail "admin login did not require 2FA (status=$($login.status))" }
$verify = ApiPost '/auth/2fa/verify' @{ pending_token = $login.pending_token; code = (Get-Totp $secret) } $null
$adminTok = $verify.access_token
if (-not $adminTok) { Fail "2fa verify returned no access token" }
Pass "admin session established via TOTP"

# ---- Tenant owner sets up an org + project (the impersonation target) --------
Info "tenant owner creates org + project"
$org = ApiPost '/orgs' @{ name = "Tenant $stamp"; slug = "tenant-$stamp" } $ownerTok
$orgId = $org.id
$proj = ApiPost "/orgs/$orgId/projects" @{ key = "TEN"; name = "Work"; color = "#22aa88" } $ownerTok
$projId = $proj.id
if (-not $orgId -or -not $projId) { Fail "org/project setup failed" }
Pass "org=$orgId project=$projId"

# ---- 6.9.1 admin tenant list + detail ---------------------------------------
Info "6.9.1 admin lists tenants + reads detail"
$tenants = Invoke-RestMethod -Method Get -Uri "$Api/admin/tenants" -Headers @{ Authorization = "Bearer $adminTok" }
$row = $tenants.tenants | Where-Object { $_.org_id -eq $orgId } | Select-Object -First 1
if (-not $row) { Fail "admin tenant list did not include $orgId" }
if ($null -eq $row.mrr) { Fail "tenant summary missing mrr field" }
$detail = Invoke-RestMethod -Method Get -Uri "$Api/admin/tenants/$orgId" -Headers @{ Authorization = "Bearer $adminTok" }
if (-not $detail) { Fail "tenant detail empty" }
Pass "tenant list + detail (mrr=$($row.mrr))"

Info "6.9.1b /admin refused without platform admin (tenant owner)"
try {
    Invoke-WebRequest -Method Get -Uri "$Api/admin/tenants" -Headers @{ Authorization = "Bearer $ownerTok" } -UseBasicParsing | Out-Null
    Fail "tenant owner reached /admin (should be 403)"
} catch {
    if ([int]$_.Exception.Response.StatusCode -ne 403) { Fail "non-admin /admin status = $([int]$_.Exception.Response.StatusCode), want 403" }
}
Pass "non-admin blocked from /admin"

# ---- 6.9.2 impersonation: read ok, write 403, dual-identity audit ------------
Info "6.9.2 admin impersonates the org"
$imp = Invoke-RestMethod -Method Post -Uri "$Api/admin/tenants/$orgId/impersonate" -Headers @{ Authorization = "Bearer $adminTok" }
$impTok = $imp.token
if (-not $impTok) { Fail "impersonation returned no token" }
$readOK = ApiGet "/orgs/$orgId/projects" $impTok
if ($null -eq $readOK) { Fail "impersonated read failed" }
$w = ExpectStatus 'POST' "/orgs/$orgId/projects" $impTok @{ key = "X"; name = "nope" } 403
if ($w -ne 403) { Fail "impersonated write status = $w, want 403" }
Pass "impersonation read ok / write 403"

Info "6.9.2b global audit shows both identities"
$audit = Invoke-RestMethod -Method Get -Uri "$Api/admin/audit?action=admin.impersonate_start" -Headers @{ Authorization = "Bearer $adminTok" }
$aRow = $audit.entries | Where-Object { $_.org_id -eq $orgId } | Select-Object -First 1
if (-not $aRow) { Fail "no admin.impersonate_start audit row for $orgId" }
if (-not $aRow.impersonator_user_id) { Fail "audit row missing impersonator identity" }
Pass "impersonation audited with both identities"

# ---- 6.9.3 API key: one-time reveal, scoped call, rate-limit header ----------
Info "6.9.3 tenant owner mints a read API key"
$key = ApiPost "/orgs/$orgId/api-keys" @{ name = "ci"; scopes = @("read") } $ownerTok
$secretKey = $key.secret
if (-not $secretKey) { Fail "api-key create did not reveal a secret" }
$resp = Invoke-WebRequest -Method Get -Uri "$V1/orgs/$orgId/projects" -Headers @{ Authorization = "Bearer $secretKey" } -UseBasicParsing
if ($resp.StatusCode -ne 200) { Fail "api-key GET status = $($resp.StatusCode)" }
if (-not $resp.Headers['X-RateLimit-Limit']) { Fail "missing X-RateLimit-Limit header" }
$wk = ExpectStatus 'POST' "/orgs/$orgId/projects" $secretKey @{ key = "Y"; name = "nope" } 403
if ($wk -ne 403) { Fail "read-scoped key write status = $wk, want 403" }
Pass "api-key scoped read ok (RateLimit=$($resp.Headers['X-RateLimit-Limit'])) / write 403"

# ---- 6.9.4 analytics + usage + openapi --------------------------------------
Info "6.9.4 analytics + usage + openapi"
$an = ApiGet "/orgs/$orgId/projects/$projId/analytics" $ownerTok
if ($null -eq $an.project_id) { Fail "project analytics missing project_id" }
$usage = ApiGet "/orgs/$orgId/usage" $ownerTok
if ($null -eq $usage.plan) { Fail "usage dashboard missing plan" }
$spec = Invoke-RestMethod -Method Get -Uri "$V1/openapi.json"
if (-not $spec.openapi) { Fail "openapi.json missing 'openapi' version field" }
Pass "analytics + usage + openapi ($($spec.openapi))"

Write-Host ""
Pass "smoke6 all checks passed"

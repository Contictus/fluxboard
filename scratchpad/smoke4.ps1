# smoke4.ps1 — Phase 4 billing e2e against the dockerized stack (§4.9).
# Prereqs: make up (or compose up), migrations applied, plans seeded
# (cmd/stripeseed), STRIPE_WEBHOOK_SECRET set in .env (stub HMAC key).
# Covers: summary(free) → checkout(stub) → signed webhook → entitlement upgrade
# → replay dedup → payment_failed → past_due + dunning mail in Mailpit →
# free-plan 402 on project cap → invoice mirror list → 429 rate limit.
# PowerShell 5.1-compatible.

$ErrorActionPreference = 'Stop'
$Api     = if ($env:SMOKE_API)     { $env:SMOKE_API }     else { 'http://localhost:8080/api/v1' }
$Mailpit = if ($env:SMOKE_MAILPIT) { $env:SMOKE_MAILPIT } else { 'http://localhost:8025' }
$Secret  = if ($env:STRIPE_WEBHOOK_SECRET) { $env:STRIPE_WEBHOOK_SECRET } else { 'whsec_stub_dev' }

$script:failures = 0
function Pass([string]$msg) { Write-Host "PASS  $msg" -ForegroundColor Green }
function Fail([string]$msg) { Write-Host "FAIL  $msg" -ForegroundColor Red; $script:failures++ }

# Invoke-Api: PS5.1-safe HTTP call that never throws on non-2xx; returns
# @{Status=<int>; Body=<parsed json or $null>}.
function Invoke-Api([string]$Method, [string]$Url, $BodyObj, [hashtable]$Headers = @{}) {
    $json = $null
    if ($null -ne $BodyObj) { $json = ($BodyObj | ConvertTo-Json -Depth 8) }
    try {
        $resp = Invoke-WebRequest -Method $Method -Uri $Url -Headers $Headers `
            -ContentType 'application/json' -Body $json -UseBasicParsing
        $parsed = $null
        if ($resp.Content) { try { $parsed = $resp.Content | ConvertFrom-Json } catch {} }
        return @{ Status = [int]$resp.StatusCode; Body = $parsed }
    } catch {
        $r = $_.Exception.Response
        if ($null -eq $r) { throw }
        # PS 5.1: ErrorDetails carries the body; the response stream is often
        # already drained by the time the catch runs.
        $content = $_.ErrorDetails.Message
        if (-not $content) {
            $stream = $r.GetResponseStream()
            if ($stream.CanSeek) { $stream.Position = 0 }
            $content = (New-Object IO.StreamReader($stream)).ReadToEnd()
        }
        $parsed = $null
        if ($content) { try { $parsed = $content | ConvertFrom-Json } catch {} }
        return @{ Status = [int]$r.StatusCode; Body = $parsed }
    }
}

# Sign-Stub: hex HMAC-SHA256 of the exact body bytes (stripex.Sign equivalent).
function Sign-Stub([string]$Body) {
    $hmac = New-Object System.Security.Cryptography.HMACSHA256
    $hmac.Key = [Text.Encoding]::UTF8.GetBytes($Secret)
    $hash = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($Body))
    ($hash | ForEach-Object { $_.ToString('x2') }) -join ''
}

# Raw-body webhook post (signature is over exact bytes — no re-serialization).
function Post-WebhookRaw([string]$Body) {
    try {
        $resp = Invoke-WebRequest -Method POST -Uri "$Api/webhooks/stripe" `
            -Headers @{ 'X-Stub-Signature' = (Sign-Stub $Body) } `
            -ContentType 'application/json' -Body $Body -UseBasicParsing
        return [int]$resp.StatusCode
    } catch {
        if ($null -eq $_.Exception.Response) { throw }
        return [int]$_.Exception.Response.StatusCode
    }
}

function New-VerifiedUser([string]$Tag) {
    $email = "smoke4-$Tag-$(Get-Random)@e2e.test"
    $pw = 'Smoke4!pass9'
    $r = Invoke-Api POST "$Api/auth/register" @{ email = $email; password = $pw; name = "Smoke $Tag" }
    if ($r.Status -ge 300) { throw "register failed: $($r.Status)" }

    # Pull the verification token out of the Mailpit message body.
    $token = $null
    foreach ($i in 1..20) {
        Start-Sleep -Milliseconds 500
        $search = Invoke-RestMethod "$Mailpit/api/v1/search?query=to:$email"
        if ($search.messages.Count -gt 0) {
            $msg = Invoke-RestMethod "$Mailpit/api/v1/message/$($search.messages[0].ID)"
            if ($msg.Text -match 'token=([^\s&"]+)') { $token = $Matches[1]; break }
        }
    }
    if (-not $token) { throw "no verification mail for $email in Mailpit" }
    $r = Invoke-Api POST "$Api/auth/verify-email/confirm" @{ token = $token }
    if ($r.Status -ge 300) { throw "verify confirm failed: $($r.Status)" }

    $r = Invoke-Api POST "$Api/auth/login" @{ email = $email; password = $pw }
    if ($r.Status -ne 200) { throw "login failed: $($r.Status)" }
    return @{ Email = $email; Auth = @{ Authorization = "Bearer $($r.Body.access_token)" } }
}

Write-Host "== smoke4: Phase 4 billing e2e ($Api) ==" -ForegroundColor Cyan

# ---- setup: verified user + org -------------------------------------------
$user = New-VerifiedUser 'owner'
$slug = "smoke4-$(Get-Random)"
$r = Invoke-Api POST "$Api/orgs" @{ name = 'Smoke4 Org'; slug = $slug } $user.Auth
if ($r.Status -ge 300) { throw "org create failed: $($r.Status)" }
$orgID = $r.Body.id
Pass "setup: verified user + org $orgID"

# ---- 1. summary starts on Free ----------------------------------------------
$r = Invoke-Api GET "$Api/orgs/$orgID/billing/summary" $null $user.Auth
if ($r.Status -eq 200 -and $r.Body.plan -eq 'free' -and -not $r.Body.has_subscription) {
    Pass 'summary: fresh org resolves Free, no subscription'
} else { Fail "summary(free): status=$($r.Status) plan=$($r.Body.plan)" }

# ---- 2. checkout returns stub URL --------------------------------------------
$r = Invoke-Api POST "$Api/orgs/$orgID/billing/checkout" @{ plan = 'pro'; seats = 1 } $user.Auth
if ($r.Status -eq 200 -and $r.Body.url) { Pass "checkout: stub URL $($r.Body.url)" }
else { Fail "checkout: status=$($r.Status)" }

# ---- 3. checkout.session.completed webhook → pro/active (4.9.1) --------------
$now = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$periodEnd = $now + 30*24*3600
$evtCheckout = @"
{"id":"evt_smoke4_co_$now","type":"checkout.session.completed","created":$now,"org_id":"$orgID","subscription":{"subscription_id":"sub_smoke4_$now","customer_id":"cus_stub_$orgID","status":"active","plan_code":"pro","current_period_end":$periodEnd,"cancel_at_period_end":false}}
"@.Trim()
$code = Post-WebhookRaw $evtCheckout
if ($code -eq 200) { Pass 'webhook: checkout.session.completed accepted (200)' }
else { Fail "webhook checkout: status=$code" }

$r = Invoke-Api GET "$Api/orgs/$orgID/billing/summary" $null $user.Auth
if ($r.Body.plan -eq 'pro' -and $r.Body.status -eq 'active' -and $r.Body.has_subscription) {
    Pass 'entitlements: summary upgraded to pro/active'
} else { Fail "summary(pro): plan=$($r.Body.plan) status=$($r.Body.status)" }

# ---- 4. replay same body → still 200, state unchanged (idempotency) ----------
$code = Post-WebhookRaw $evtCheckout
$r = Invoke-Api GET "$Api/orgs/$orgID/billing/summary" $null $user.Auth
if ($code -eq 200 -and $r.Body.plan -eq 'pro') { Pass 'webhook: replay deduped (200, state unchanged)' }
else { Fail "webhook replay: status=$code plan=$($r.Body.plan)" }

# ---- 5. bad signature → 400 ---------------------------------------------------
try {
    $resp = Invoke-WebRequest -Method POST -Uri "$Api/webhooks/stripe" `
        -Headers @{ 'X-Stub-Signature' = 'deadbeef' } -ContentType 'application/json' `
        -Body $evtCheckout -UseBasicParsing
    Fail "webhook bad sig: status=$([int]$resp.StatusCode), want 400"
} catch {
    if ([int]$_.Exception.Response.StatusCode -eq 400) { Pass 'webhook: bad signature rejected (400)' }
    else { Fail "webhook bad sig: $([int]$_.Exception.Response.StatusCode)" }
}

# ---- 6. invoice.payment_failed → past_due + dunning mail (4.9.2) --------------
$now2 = $now + 60
$evtFailed = @"
{"id":"evt_smoke4_pf_$now2","type":"invoice.payment_failed","created":$now2,"org_id":"$orgID","invoice":{"invoice_id":"in_smoke4_$now2","number":"SMOKE4-001","status":"open","amount_due":2900,"amount_paid":0,"currency":"usd","hosted_pdf_url":"","period_start":$now,"period_end":$periodEnd}}
"@.Trim()
$code = Post-WebhookRaw $evtFailed
$r = Invoke-Api GET "$Api/orgs/$orgID/billing/summary" $null $user.Auth
if ($code -eq 200 -and $r.Body.status -eq 'past_due' -and $r.Body.past_due_warning) {
    Pass 'webhook: payment_failed → past_due with warning flag'
} else { Fail "payment_failed: status=$code sub_status=$($r.Body.status)" }

# Dunning mail proves outbox → outbox:drain → email:send → SMTP (worker path).
$gotMail = $false
foreach ($i in 1..30) {
    Start-Sleep -Seconds 2
    $search = Invoke-RestMethod "$Mailpit/api/v1/search?query=to:$($user.Email)+payment"
    if ($search.messages.Count -gt 0) { $gotMail = $true; break }
}
if ($gotMail) { Pass 'jobs: dunning email reached Mailpit (outbox drained)' }
else { Fail 'jobs: no dunning email in Mailpit after 60s' }

# ---- 7. invoice mirror listed -------------------------------------------------
$r = Invoke-Api GET "$Api/orgs/$orgID/billing/invoices" $null $user.Auth
if ($r.Status -eq 200 -and $r.Body.invoices.Count -ge 1) {
    Pass "invoices: mirror lists $($r.Body.invoices.Count) invoice(s)"
} else { Fail "invoices: status=$($r.Status) count=$($r.Body.invoices.Count)" }

# ---- 8. Free plan 402 on project cap (4.9.3) ----------------------------------
$user2 = New-VerifiedUser 'freecap'
$slug2 = "smoke4f-$(Get-Random)"
$r = Invoke-Api POST "$Api/orgs" @{ name = 'Smoke4 Free'; slug = $slug2 } $user2.Auth
$org2 = $r.Body.id
$last = $null
foreach ($k in 'SMA', 'SMB', 'SMC', 'SMD') { # key = 2-6 uppercase letters (FR-PROJ-001)
    $last = Invoke-Api POST "$Api/orgs/$org2/projects" `
        @{ key = $k; name = "Proj $k"; visibility = 'private' } $user2.Auth
}
if ($last.Status -eq 402 -and $last.Body.error.code -eq 'plan_limit_exceeded' -and
    $last.Body.error.details.max -eq 3) {
    Pass "402: 4th project blocked (limit=$($last.Body.error.details.limit) current=$($last.Body.error.details.current) max=$($last.Body.error.details.max))"
} else { Fail "402: status=$($last.Status) code=$($last.Body.error.code)" }

# ---- 9. Free plan rate limit → 429 --------------------------------------------
$got429 = $false
foreach ($i in 1..70) {
    $r = Invoke-Api GET "$Api/orgs/$org2/billing/summary" $null $user2.Auth
    if ($r.Status -eq 429) { $got429 = $true; break }
}
if ($got429) { Pass "429: free-plan rate limit tripped after ~$i requests" }
else { Fail '429: 70 rapid requests never rate-limited' }

# ---- verdict -------------------------------------------------------------------
Write-Host ''
if ($script:failures -eq 0) { Write-Host '== smoke4: ALL GREEN ==' -ForegroundColor Green; exit 0 }
else { Write-Host "== smoke4: $($script:failures) FAILURE(S) ==" -ForegroundColor Red; exit 1 }

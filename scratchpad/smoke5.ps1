# Phase 5 realtime + notifications end-to-end smoke (docs/build/PHASE-5 §9).
#
# Exercises the SSE stream, replay/resync, and the @mention -> notification +
# email fan-out against the live `make up` stack. Run from the repo root after:
#   make up && make migrate
# Ports below are the compose defaults; override with the env vars if you set
# host-port overrides in .env.
#
#   powershell -ExecutionPolicy Bypass -File scratchpad/smoke5.ps1
#
# Exit code 0 = all checks passed.

$ErrorActionPreference = 'Stop'

$Api     = $env:SMOKE_API;     if (-not $Api)     { $Api     = 'http://localhost:8080' }
$Mailpit = $env:SMOKE_MAILPIT; if (-not $Mailpit) { $Mailpit = 'http://localhost:8025' }
$RedisContainer = $env:SMOKE_REDIS; if (-not $RedisContainer) { $RedisContainer = 'fluxboard-redis-1' }
$V1 = "$Api/api/v1"

$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$ownerEmail  = "owner$stamp@example.com"
$memberEmail = "member$stamp@example.com"
$password    = "Sup3r-Secret-Pw!"

function Info($m)  { Write-Host "[..] $m" }
function Pass($m)  { Write-Host "[OK] $m" -ForegroundColor Green }
function Fail($m)  { Write-Host "[XX] $m" -ForegroundColor Red; exit 1 }

function ApiPost($path, $body, $token, $extraHeaders) {
    $headers = @{}
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    if ($extraHeaders) { $extraHeaders.GetEnumerator() | ForEach-Object { $headers[$_.Key] = $_.Value } }
    $json = if ($body -ne $null) { $body | ConvertTo-Json -Depth 8 } else { $null }
    return Invoke-RestMethod -Method Post -Uri "$V1$path" -Headers $headers -ContentType 'application/json' -Body $json
}
function ApiPatch($path, $body, $token) {
    $headers = @{}
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    $json = if ($body -ne $null) { $body | ConvertTo-Json -Depth 8 } else { $null }
    return Invoke-RestMethod -Method Patch -Uri "$V1$path" -Headers $headers -ContentType 'application/json' -Body $json
}
function ApiGet($path, $token) {
    $headers = @{}
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    return Invoke-RestMethod -Method Get -Uri "$V1$path" -Headers $headers
}

# Poll Mailpit for the newest message to `addr` (optionally whose Subject matches
# `subjectLike`), return its text body. Mailpit lists newest-first.
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

function TokenFromQuery($text) {
    if ($text -match 'token=([A-Za-z0-9._\-]+)') { return $Matches[1] }
    return $null
}
function TokenFromInvitePath($text) {
    if ($text -match '/invite/([A-Za-z0-9._\-]+)') { return $Matches[1] }
    return $null
}

# Register -> verify email via Mailpit -> login. Returns the access token of a
# verified session.
function RegisterVerifyLogin($email) {
    ApiPost '/auth/register' @{ email = $email; password = $password; name = $email.Split('@')[0] } $null $null | Out-Null
    $body = MailpitBody $email
    if (-not $body) { Fail "no verification email for $email" }
    $tok = TokenFromQuery $body
    if (-not $tok) { Fail "no verify token in email for $email" }
    ApiPost '/auth/verify-email/confirm' @{ token = $tok } $null $null | Out-Null
    $login = ApiPost '/auth/login' @{ email = $email; password = $password } $null $null
    if (-not $login.access_token) { Fail "login returned no access token for $email" }
    return $login.access_token
}

Info "health check"
$code = (Invoke-WebRequest -Uri "$Api/healthz" -UseBasicParsing).StatusCode
if ($code -ne 200) { Fail "healthz = $code" }
Pass "api healthy"

Info "register + verify two users"
$ownerTok  = RegisterVerifyLogin $ownerEmail
$memberTok = RegisterVerifyLogin $memberEmail
$memberLocal = $memberEmail.Split('@')[0]
Pass "owner + member verified"

Info "owner creates org + project"
$org = ApiPost '/orgs' @{ name = "Smoke $stamp"; slug = "smoke-$stamp" } $ownerTok $null
$orgId = $org.id
if (-not $orgId) { Fail "org create returned no id" }
$proj = ApiPost "/orgs/$orgId/projects" @{ key = "SMK"; name = "Realtime"; color = "#3366ff" } $ownerTok $null
$projId = $proj.id
$board = ApiGet "/orgs/$orgId/projects/$projId/board" $ownerTok
$colId = $board.columns[0].id
if (-not $colId) { Fail "board has no columns" }
Pass "org=$orgId project=$projId column=$colId"

Info "member joins org + project"
$inv = ApiPost "/orgs/$orgId/invitations" @{ email = $memberEmail; role = "MEMBER" } $ownerTok @{ 'Idempotency-Key' = "inv-$stamp" }
$inviteBody = MailpitBody $memberEmail 'invited'
$inviteTok = TokenFromInvitePath $inviteBody
if (-not $inviteTok) { Fail "no invite token in email for $memberEmail" }
ApiPost '/invitations/accept' @{ token = $inviteTok } $memberTok $null | Out-Null
# Resolve member user id from the org member list, then add to the project.
$members = ApiGet "/orgs/$orgId/members" $ownerTok
$memberId = ($members.items | Where-Object { $_.email -eq $memberEmail } | Select-Object -First 1).user_id
if (-not $memberId) { Fail "could not resolve member user id" }
ApiPost "/orgs/$orgId/projects/$projId/members" @{ user_id = $memberId; role = "CONTRIBUTOR" } $ownerTok $null | Out-Null
Pass "member $memberId joined org + project"

# ---- 5.9.1 live SSE: task.created + task.moved ----------------------------
Info "5.9.1 open SSE, create + move a task"
$sseFile = Join-Path $env:TEMP "smoke5-sse-$stamp.txt"
$sseArgs = @('-sN', '--max-time', '9', '-H', "Authorization: Bearer $ownerTok", "$V1/orgs/$orgId/events")
$sseJob = Start-Job -ScriptBlock {
    param($curlArgs, $outFile)
    & curl.exe @curlArgs | Out-File -FilePath $outFile -Encoding ascii
} -ArgumentList $sseArgs, $sseFile
Start-Sleep -Seconds 2  # let the stream attach before we publish

$task = ApiPost "/orgs/$orgId/projects/$projId/tasks" @{ column_id = $colId; title = "Realtime card" } $ownerTok $null
$taskId = $task.id
# Move to a second column if one exists, else re-rank in place.
$targetCol = $colId
if ($board.columns.Count -gt 1) { $targetCol = $board.columns[1].id }
ApiPatch "/orgs/$orgId/tasks/$taskId/position" @{ column_id = $targetCol; rank = "h" } $ownerTok | Out-Null

Wait-Job $sseJob -Timeout 15 | Out-Null
Remove-Job $sseJob -Force
$sse = if (Test-Path $sseFile) { Get-Content $sseFile -Raw } else { "" }

if ($sse -match 'event:\s*task\.created') { Pass "received task.created" } else { Fail "no task.created in SSE stream:`n$sse" }
if ($sse -match 'event:\s*task\.moved')   { Pass "received task.moved" }   else { Fail "no task.moved in SSE stream:`n$sse" }

# Capture the first event id for the replay test.
$firstId = $null
if ($sse -match 'id:\s*([0-9]+-[0-9]+)') { $firstId = $Matches[1] }

# ---- 5.9.2 reconnect with Last-Event-ID replays the backlog ----------------
Info "5.9.2 reconnect with Last-Event-ID replays missed events"
if (-not $firstId) { Fail "no event id captured for replay" }
$replay = & curl.exe -sN --max-time 4 -H "Authorization: Bearer $ownerTok" -H "Last-Event-ID: $firstId" "$V1/orgs/$orgId/events"
$replayText = ($replay -join "`n")
if ($replayText -match 'task\.moved') { Pass "replay delivered events after $firstId" } else { Fail "replay from $firstId missing task.moved:`n$replayText" }

# ---- 5.9.3 @mention -> notification + email --------------------------------
Info "5.9.3 @mention a project member in a comment"
$mtask = ApiPost "/orgs/$orgId/projects/$projId/tasks" @{ column_id = $colId; title = "Mention card"; assignee_id = $memberId } $ownerTok $null
ApiPost "/orgs/$orgId/tasks/$($mtask.id)/comments" @{ body = "hey @$memberLocal please review" } $ownerTok $null | Out-Null

# In-app notification for the member (poll; worker + fan-out are async).
$found = $false
for ($i = 0; $i -lt 20; $i++) {
    $n = ApiGet "/orgs/$orgId/notifications?unread=1" $memberTok
    if ($n.notifications.Count -gt 0) { $found = $true; break }
    Start-Sleep -Milliseconds 500
}
if ($found) { Pass "member has an in-app notification" } else { Fail "no in-app notification for member" }

# Email in Mailpit for the member (fan-out enqueued email:send -> drained -> sent).
# Filter on the mention subject so we do not match the earlier verify/invite mail.
$memMail = MailpitBody $memberEmail 'mention'
if ($memMail) { Pass "member received a mention notification email" } else { Fail "no mention notification email in Mailpit for member" }

# ---- 5.9.4 FLUSHALL -> resync on reconnect ---------------------------------
Info "5.9.4 FLUSHALL redis, reconnect with a stale id -> resync"
try {
    & docker exec $RedisContainer redis-cli FLUSHALL | Out-Null
} catch {
    Fail "could not FLUSHALL via 'docker exec $RedisContainer redis-cli' (set SMOKE_REDIS to the redis container name)"
}
$resync = & curl.exe -sN --max-time 4 -H "Authorization: Bearer $ownerTok" -H "Last-Event-ID: $firstId" "$V1/orgs/$orgId/events"
$resyncText = ($resync -join "`n")
if ($resyncText -match 'event:\s*resync') { Pass "stale reconnect emitted resync" } else { Fail "expected resync after FLUSHALL, got:`n$resyncText" }

Write-Host ""
Pass "smoke5 all checks passed"

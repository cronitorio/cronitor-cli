
function ExitWithCode($exitcode) {
    Write-Host "existing with code 123"
    $host.SetShouldExit($exitcode)
    exit $exitcode
}

ExitWithCode 123


function ExitWithCode($exitcode) {
    Write-Host "exiting with code 123"
    $host.SetShouldExit($exitcode)
    exit $exitcode
}

ExitWithCode 123

#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR
}

@test "Auth help lists login status logout" {
  run ../cronitor auth --help
  [ "$status" -eq 0 ]
  echo "$output" | grep -q "login"
  echo "$output" | grep -q "status"
  echo "$output" | grep -q "logout"
}

@test "Auth status without a key exits non-zero" {
  run env -u CRONITOR_API_KEY ../cronitor --config /tmp/cronitor-auth-missing.json auth status
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "Not logged in"
}

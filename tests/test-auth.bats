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

@test "Signup help is an alias for auth login" {
  run ../cronitor signup --help
  [ "$status" -eq 0 ]
  echo "$output" | grep -qi "alias"
  echo "$output" | grep -q "auth login"
}

@test "Auth status without a key exits non-zero" {
  run env -u CRONITOR_API_KEY ../cronitor --config /tmp/cronitor-auth-missing.json auth status
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "Not logged in"
}

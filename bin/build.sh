#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR
cd ../

if [ "$CRONITORCLI_SENTRY_DSN" ]
  then echo "Adding Sentry to Build..."
  SENTRY_DSN_ESCAPED=$(printf '%s\n' "$CRONITORCLI_SENTRY_DSN" | sed -e 's/[\/&]/\\&/g')
  perl -pi -e "s/\/\/\ SetDSN/raven.SetDSN(\"${SENTRY_DSN_ESCAPED}\")/g" main.go
fi

go build

if [ "$CRONITORCLI_SENTRY_DSN" ]
  then perl -pi -e "s/raven\.SetDSN\(\"${SENTRY_DSN_ESCAPED}\"\)/\/\/\ SetDSN/g" main.go
fi
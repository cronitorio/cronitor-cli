SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ "$CRONITORCLI_SENTRY_DSN" ]
  sed -i 's/// raven.SetDSN("")/raven.SetDSN("' + $CRONITORCLI_SENTRY_DSN + '")/g' ../main.go
fi

cd ../
go build

if [ "$CRONITORCLI_SENTRY_DSN" ]
  sed -i 's/raven.SetDSN("' + $CRONITORCLI_SENTRY_DSN + '")/// raven.SetDSN("")/g' ../main.go
fi
SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ -z "$1" ]
  then echo "Usage: release-alpha.sh <version number>" && exit 1
fi

if [ -z "$CRONITORCLI_EQUINOX_TOKEN" ]
  then echo "Usage: requires CRONITORCLI_EQUINOX_TOKEN env variable" && exit 1
fi

git tag $1
git push --tags

equinox release \
 --version="$1" \
 --platforms="darwin_amd64 linux_amd64 linux_386 windows_amd64" \
 --signing-key=../equinox.key \
 --app="app_itoJoCoW8dr" \
 --token=$CRONITORCLI_EQUINOX_TOKEN \
 --channel="alpha" \
cronitor

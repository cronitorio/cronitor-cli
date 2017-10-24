SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ -z "$1" ]
  then echo "Usage: create-release.sh <version number>" && exit 1
fi

git tag $1
git push --tags

equinox release \
 --version="$1" \
 --platforms="darwin_amd64 linux_amd64 windows_amd64" \
 --signing-key=../equinox.key \
 --app="app_itoJoCoW8dr" \
 --token="***REMOVED***" \
cronitor

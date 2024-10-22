#!/bin/bash

COMMAND="$1" # comment | uncomment
PATTERN=""

function print_out_usage {
	cat <<EOF
Usage: ./scripts/mockery_comment_fix.sh <(comment | uncomment)>
EOF
}

if [ "${COMMAND}" == "comment" ]; then
  PATTERN="2,$$ s/^/\/\//"
elif [ "${COMMAND}" == "uncomment" ]; then
  PATTERN="2,$$ s/^\/\///"
else
  echo "error: unsupported or missing argument" >&2
	print_out_usage >&2
  exit 1
fi

case "$( uname -s )" in
    "Linux" ) # For GNU/Linux systems the -i flag doesn't accept the extension.
        sed -i "$PATTERN" pkg/dao/registry_mock.go
        ;;
    "Darwin" | * ) # For Darwin/BSD systems the -i flag needs an extension specified, can be left blank to not add an extension.
        sed -i '' "$PATTERN" pkg/dao/registry_mock.go
        ;;
esac

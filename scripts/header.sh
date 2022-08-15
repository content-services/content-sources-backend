#!/bin/bash

ORG_ID="$1"
ACCOUNT_NUMBER="$2"


function print_out_usage {
	cat <<EOF
Usage: ./scripts/header.sh <org_id> [account_number]
EOF
}

function error {
	local err=$?
	print_out_usage >&2
	printf "error: %s\n" "$*" >&2
	exit $err
}

[ "${ORG_ID}" != "" ] || error "ORG_ID is required and cannot be empty"

ENC="$(echo "{\"identity\":{\"account_number\":\"$2\",\"internal\":{\"org_id\":\"$1\"}}}" | base64 -w0)"
echo "x-rh-identity: $ENC"

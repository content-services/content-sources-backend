#!/bin/bash

ACCOUNT_NUMBER="$1"
ORG_ID="$2"


function print_out_usage {
	cat <<EOF
Usage: ./scripts/header.sh <account_number> <org_id>
EOF
}

function error {
	local err=$?
	print_out_usage >&2
	printf "error:%s\n" "$*" >&2
	exit $err
}

[ "${ACCOUNT_NUMBER}" != "" ] || error "ACCOUNT_NUMBER is required and cannot be empty"
[ "${ORG_ID}" != "" ] || error "ORD_ID is required and cannot be empty"

ENC="$(echo "{\"identity\":{\"account_number\":\"$1\",\"internal\":{\"org_id\":\"$2\"}}}" | base64 -w0)"
echo "x-rh-identity: $ENC"

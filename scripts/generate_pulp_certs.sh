#!/bin/bash

CERT_DIR="compose_files/pulp/assets/certs/dev_certs"
mkdir -p "${CERT_DIR}"

CA_KEY="${CERT_DIR}/ca.key"
CA_CRT="${CERT_DIR}/ca.crt"
CA_SRL="${CERT_DIR}/ca.srl"
SERVER_KEY="${CERT_DIR}/server.key"
SERVER_CSR="${CERT_DIR}/server.csr"
SERVER_CRT="${CERT_DIR}/server.crt"
CLIENT_KEY="${CERT_DIR}/client.key"
CLIENT_CSR="${CERT_DIR}/client.csr"
CLIENT_CRT="${CERT_DIR}/client.crt"

if [[ -f "${CA_KEY}" && -f "${CA_CRT}" && -f "${SERVER_KEY}" && -f "${SERVER_CRT}" && -f "${CLIENT_KEY}" && -f "${CLIENT_CRT}" ]]; then
  echo "dev certs exist, skipping"
  exit 0
fi

rm -rf "${CERT_DIR}"/*

# generate CA key and cert with long expiration (10 years)
openssl genrsa -out "${CA_KEY}" 4096

openssl req -new -x509 -days 3650 -key "${CA_KEY}" -out "${CA_CRT}" \
  -subj "/CN=DevCA"

# generate server key and cert
openssl genrsa -out "${SERVER_KEY}" 4096

openssl req -new -key "${SERVER_KEY}" -out "${SERVER_CSR}" \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

openssl x509 -req -days 3650 -in "${SERVER_CSR}" \
  -CA "${CA_CRT}" -CAkey "${CA_KEY}" -CAcreateserial \
  -out "${SERVER_CRT}" \
  -extensions v3_req \
  -extfile <(echo -e "[v3_req]\nkeyUsage = digitalSignature, keyEncipherment\nextendedKeyUsage = serverAuth\nsubjectAltName = DNS:localhost,IP:127.0.0.1")

# generate client cert and key
openssl genrsa -out "${CLIENT_KEY}" 4096

openssl req -new -key "${CLIENT_KEY}" -out "${CLIENT_CSR}" \
  -subj "/CN=dev-client/O=DevOrg" \
  -addext "subjectAltName=DNS:dev-client,DNS:localhost"

openssl x509 -req -days 3650 \
  -in "${CLIENT_CSR}" \
  -CA "${CA_CRT}" -CAkey "${CA_KEY}" -CAcreateserial \
  -out "${CLIENT_CRT}" \
  -extensions v3_req \
  -extfile <(echo -e "[v3_req]\nkeyUsage = digitalSignature, keyEncipherment\nextendedKeyUsage = clientAuth\nsubjectAltName = DNS:dev-client,DNS:localhost")

echo "dev certs generated successfully in ${CERT_DIR}"

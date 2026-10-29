# We cannot use the official golang images from docker.io because it's blocked
# on our CI. Instead, we use image maintained by Quay team which is updated every
# few hours automatically from their GitHub. For more info:
#
#  https://github.com/quay/claircore/actions/workflows/golang-image.yml
#  https://github.com/quay/claircore/blob/main/.github/workflows/golang-image.yml
FROM registry.redhat.io/ubi9/go-toolset:9.8-1792906297 as builder

WORKDIR /go/src/app
COPY . .

USER 0

RUN make get-deps build


FROM registry.redhat.io/ubi10/ubi-minimal:latest as ca-source

# Append our internal IT root CA to the system CA bundle. The final image is
# ubi-micro and has no shell/tooling, so build the combined bundle here.
COPY build/2022-IT-Root-CA.pem /tmp/2022-IT-Root-CA.pem
RUN cat /tmp/2022-IT-Root-CA.pem >> /etc/pki/tls/certs/ca-bundle.crt

FROM registry.redhat.io/ubi10/ubi-micro:latest

WORKDIR /


COPY --from=ca-source /etc/pki/tls/certs/ca-bundle.crt /etc/pki/tls/certs/

COPY --from=builder /go/src/app/release/* ./
RUN mkdir ./db/
COPY --from=builder /go/src/app/db ./db/

USER 1001

CMD ["/content-sources", "api"]

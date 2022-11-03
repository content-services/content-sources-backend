# We cannot use the official golang images from docker.io because it's blocked
# on our CI. Instead, we use image maintained by Quay team which is updated every
# few hours automatically from their GitHub. For more info:
#
#  https://github.com/quay/claircore/actions/workflows/golang-image.yml
#  https://github.com/quay/claircore/blob/main/.github/workflows/golang-image.yml
FROM quay.io/projectquay/golang:1.18 as builder

WORKDIR /go/src/app
COPY . .

USER 0

RUN make get-deps build

FROM registry.redhat.io/ubi8/ubi-minimal:latest

WORKDIR /

COPY --from=builder /go/src/app/release/* ./
RUN mkdir ./db/
COPY --from=builder /go/src/app/db ./db/

USER 1001

CMD ["/content-sources"]

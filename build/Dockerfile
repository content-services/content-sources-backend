FROM registry.redhat.io/ubi8/go-toolset:latest as builder

WORKDIR /go/src/app
COPY . .

USER 0

RUN make get-deps build

FROM registry.redhat.io/ubi8/ubi-minimal:8.5

WORKDIR /

COPY --from=builder /go/src/app/release/dbmigrate ./dbmigrate
COPY --from=builder /go/src/app/release/content-sources ./content-sources
RUN mkdir ./db/
COPY --from=builder /go/src/app/db ./db/

USER 1001

CMD ["/content-sources"]

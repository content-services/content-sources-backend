FROM registry.redhat.io/ubi8/go-toolset:latest as builder

WORKDIR /go/src/app
COPY . .

USER 0

RUN go get -d ./... && \
    go build -o content-sources  ./cmd/content-sources/main.go

RUN cp /go/src/app/content-sources /usr/bin/

FROM registry.redhat.io/ubi8/ubi-minimal:8.5

WORKDIR /

COPY --from=builder /go/src/app/content-sources ./content-sources

USER 1001

CMD ["/content-sources"]

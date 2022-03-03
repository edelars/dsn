ARG GOLANG_VER=1.14.2

FROM golang:${GOLANG_VER}-alpine AS builder
ARG SVC_NAME

RUN apk --no-cache add ca-certificates make

WORKDIR /build
COPY . .
RUN make build \
    && mv build/device-status-aggregator /exe

FROM scratch
COPY --from=builder /exe /

ENTRYPOINT ["/exe"]

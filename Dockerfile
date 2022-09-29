FROM golang:1.19-alpine AS build_deps

RUN apk add --no-cache git

WORKDIR /workspace

COPY go.mod .
COPY go.sum .

RUN go mod download

FROM build_deps AS build

COPY . .

RUN CGO_ENABLED=0 go build -o dhcpd-coredns -ldflags '-w -extldflags "-static"' .

FROM alpine

RUN apk add --no-cache ca-certificates

COPY --from=build /workspace/dhcpd-coredns /usr/local/bin/dhcpd-coredns

ENTRYPOINT ["dhcpd-coredns"]

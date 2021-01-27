FROM golang:latest AS build

COPY ./ /go/src/github.com/lexfrei/vk2tg/
WORKDIR /go/src/github.com/lexfrei/vk2tg/cmd/vk2tg

RUN go get ./... && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o vk2tg

FROM alpine:3.13.0

COPY --from=build /go/src/github.com/lexfrei/vk2tg/cmd/vk2tg/vk2tg /

ENTRYPOINT ["/vk2tg"]
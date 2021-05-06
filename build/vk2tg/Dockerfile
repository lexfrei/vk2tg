FROM golang:1.16.3-alpine3.13 AS build

COPY ./ /go/src/github.com/lexfrei/vk2tg/
WORKDIR /go/src/github.com/lexfrei/vk2tg/

RUN go get ./... && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" ./cmd/vk2tg/vk2tg.go 

# Disabled for arm compactability
# RUN apk add --update --no-cache upx && upx --best --lzma vk2tg

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /go/src/github.com/lexfrei/vk2tg/vk2tg /

ENTRYPOINT ["/vk2tg"]
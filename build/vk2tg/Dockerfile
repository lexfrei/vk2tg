FROM golang:1.15.7-alpine3.13 AS build

COPY ./ /go/src/github.com/lexfrei/vk2tg/
WORKDIR /go/src/github.com/lexfrei/vk2tg/

RUN go get ./... && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" ./cmd/vk2tg/vk2tg.go 

FROM harshavardhanj/upx:3.95-scratch AS zip

COPY --from=build /go/src/github.com/lexfrei/vk2tg/vk2tg /

RUN upx --best --lzma vk2tg

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=zip /vk2tg /

ENTRYPOINT ["/vk2tg"]
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# 替换为国内源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories

ENV GO111MODULE="on"
ENV GOPROXY="https://goproxy.cn,direct"
ENV CGO_ENABLED=0

WORKDIR /build
COPY . .

RUN apk add --no-cache git ca-certificates tzdata && \
    update-ca-certificates && \
    go build -ldflags="-s -w" -o wecom-nezha

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    update-ca-certificates

WORKDIR /root

COPY --from=builder /build/wecom-nezha .

EXPOSE 8080

CMD ["./wecom-nezha"]

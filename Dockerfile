FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .

RUN apk update \
    && apk add --no-cache git \
    && apk add --no-cache ca-certificates \
    && apk add --update gcc musl-dev build-base \
    && update-ca-certificates

RUN CGO_CFLAGS="-std=gnu11 -D_GNU_SOURCE" GOOS=linux go build -o tezpay -a -v .

FROM alpine:3.10

RUN apk update && apk add ca-certificates

COPY --from=builder /app/tezpay /app/tezpay

WORKDIR /app

ENTRYPOINT [ "/app/tezpay" ]
CMD ["continual"]
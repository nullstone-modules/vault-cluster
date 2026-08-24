FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY config/ config/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /vault-utils ./cmd/vault-utils

FROM alpine:3.21
COPY --from=build /vault-utils /usr/local/bin/vault-utils
ENTRYPOINT ["/usr/local/bin/vault-utils"]

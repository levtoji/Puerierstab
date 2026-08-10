FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/puerierstab .

FROM alpine:3.22

RUN adduser -D -H appuser

USER appuser

COPY --from=build /out/puerierstab /usr/local/bin/puerierstab

ENTRYPOINT ["/usr/local/bin/puerierstab"]

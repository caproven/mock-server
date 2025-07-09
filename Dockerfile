FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build -o ./mock-server

FROM scratch

USER 1001:1001

WORKDIR /app

COPY --from=builder /build/mock-server ./

ENTRYPOINT ["/app/mock-server"]

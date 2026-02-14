FROM golang:latest as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY ServiceB/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build --ldflags="-w -s" -o serviceb main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/serviceb /app/serviceb
EXPOSE 8090
CMD ["/app/serviceb"]
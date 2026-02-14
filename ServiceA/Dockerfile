FROM golang:latest as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY ServiceA/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build --ldflags="-w -s" -o servicea main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/servicea /app/servicea
EXPOSE 8080
CMD ["/app/servicea"]
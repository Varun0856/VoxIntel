FROM golang:1.26.1-alpine AS  builder
WORKDIR /build
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o voxintel .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /build/voxintel .

RUN mkdir -p /app/data
EXPOSE 8080
ENTRYPOINT ["/app/voxintel"]

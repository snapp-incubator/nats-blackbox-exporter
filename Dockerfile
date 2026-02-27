# Start from the latest golang base image
FROM golang:1.26-alpine3.23 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
WORKDIR /app/cmd/nats-blackbox-exporter
RUN go build -o /nats-blackbox-exporter

FROM alpine:3.23

WORKDIR /app/

COPY --from=builder /nats-blackbox-exporter .

ENTRYPOINT ["./nats-blackbox-exporter"]
CMD [ "--configPath=./setting/config.yaml" ]

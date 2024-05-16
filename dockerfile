# Start from the official golang:1.19 base image
FROM golang:1.19 AS build

# Install build dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    gcc \
    libc-dev \
    pkg-config \
    imagemagick \
    potrace \
    && rm -rf /var/lib/apt/lists/*

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to the working directory
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code from the current directory to the working directory inside the container
COPY . .

# Build the Go application
RUN go build -o /go/bin/app

# Set permissions for the application binary
RUN chmod +x /go/bin/app

# Start a new stage from a base Debian image
FROM debian:buster-slim

# Install runtime dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    imagemagick \
    potrace \
    && rm -rf /var/lib/apt/lists/*

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the pre-built binary from the previous stage
COPY --from=build /go/bin/app /app/app
# Copy templates and static files
COPY --from=build /app/templates /app/templates
COPY --from=build /app/uploads /app/uploads

# Set permissions for the application binary
RUN chmod +x /app/app

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["/app/app"]

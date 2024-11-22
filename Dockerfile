FROM golang:1.21-bookworm
WORKDIR /satpulse
RUN apt-get update && apt-get install -y \
    make \
    dpkg-dev \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make


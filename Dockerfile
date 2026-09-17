FROM golang:1.26-trixie
WORKDIR /satpulse
RUN apt-get update && apt-get install -y \
    make \
    dpkg-dev \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make


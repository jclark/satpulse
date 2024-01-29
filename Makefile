CONFIG_FILE=/usr/local/etc/satpulse.toml
CMD=github.com/jclark/satpulse/internal/cmd
BUILD_DATE:=$(shell date -u --rfc-3339=seconds)
GIT_VERSION:=$(shell git describe --tags --always --dirty=-modified)
XFLAGS:=-X \"$(CMD)/daemon.defaultConfigFile=$(CONFIG_FILE)\" -X \"$(CMD).gitVersion=$(GIT_VERSION)\" -X \"$(CMD).buildDate=$(BUILD_DATE)\"
TAGS=netgo,osusergo
# The GOARCHs we support.
ALL_GOARCH=arm64 amd64

ARCH:=$(shell uname -m)

# Set goarch based on detected architecture
ifeq ($(ARCH),x86_64)
GOARCH:=amd64
USE_CONFIG=default
else ifeq ($(ARCH),aarch64)
GOARCH:=arm64
USE_CONFIG=cm4
else
$(error Unknown architecture $(ARCH))
endif

all: $(GOARCH)

allarch: $(ALL_GOARCH)

$(ALL_GOARCH):
	env GOOS=linux GOARCH=$@ go build -tags "$(TAGS)" -o out/$@/ -ldflags "$(XFLAGS)" ./...

install: out/$(GOARCH)/satpulsed out/$(GOARCH)/satpulsetool
	cp out/$(GOARCH)/satpulsed /usr/local/sbin/satpulsed
	cp out/$(GOARCH)/satpulsetool /usr/local/bin/satpulsetool
	cp satpulse@.service /etc/systemd/system/
	[ -f "$(CONFIG_FILE)" ] || cp configs/$(USE_CONFIG).toml $(CONFIG_FILE)
	systemctl daemon-reload

test:
	go test -v ./...

clean:
	-rm -rf out

.PHONY: $(ALL_GOARCH) all test install clean

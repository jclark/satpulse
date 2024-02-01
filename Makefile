CONFIG_DIR=/usr/local/etc/satpulse
CMD=github.com/jclark/satpulse/internal/cmd
BUILD_DATE:=$(shell date -u --rfc-3339=seconds)
GIT_VERSION:=$(shell git describe --tags --always --dirty=-modified)
DEB_VERSION=1
DEB_PKG_VERSION:= 0.0~git$(shell git log -1 --format="%cd.%h" --date=format:%Y%m%d)-$(DEB_VERSION)
XFLAGS:= -X \"$(CMD).gitVersion=$(GIT_VERSION)\" -X \"$(CMD).buildDate=$(BUILD_DATE)\"
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

out/arm64/default.toml: configs/default.toml
	sed -e '/^interface/s/enp1s0/eth0/' -e '/^device/s/ttyUSB0/ttyAMA0/' $< > $@

out/amd64/default.toml: configs/default.toml
	cp $< $@

install: out/$(GOARCH)/satpulsed out/$(GOARCH)/satpulsetool out/$(GOARCH)/satpulse.toml
	install out/$(GOARCH)/satpulsed /usr/local/sbin/satpulsed
	install out/$(GOARCH)/satpulsetool /usr/local/bin/satpulsetool
	install satpulse@.service /etc/systemd/system/
	[ -f "$(CONFIG_DIR)/default.toml" ] || install -D out/$(GOARCH)/default.toml "$(CONFIG_DIR)/default.toml"
	systemctl daemon-reload

test:
	go test -v ./...

clean:
	-rm -rf out

deb: out/satpulse_$(DEB_PKG_VERSION)_arm64.deb out/satpulse_$(DEB_PKG_VERSION)_amd64.deb

out/satpulse_$(DEB_PKG_VERSION)_%.deb: % out/%/default.toml
	install -D -m 644 debian/conffiles out/deb.$*/DEBIAN/conffiles
	install -D debian/postinst out/deb.$*/DEBIAN/postinst
	sed -e '/^Architecture:/s/any/$*/' -e '/^Version:/s/:.*/: $(DEB_PKG_VERSION)/' debian/control > out/deb.$*/DEBIAN/control
	install -D out/$*/satpulsed out/deb.$*/usr/sbin/satpulsed
	install -D out/$*/satpulsetool out/deb.$*/usr/bin/satpulsetool
	install -D -m 644 out/$*/default.toml out/deb.$*/etc/satpulse/default.toml
	mkdir -p out/deb.$*/lib/systemd/system
	sed -e 's;/usr/local/etc/;/etc/;g' -e 's;/usr/local/;/usr/;g' satpulse@.service > out/deb.$*/lib/systemd/system/satpulse@.service
	dpkg-deb --root-owner-group --build out/deb.$* out

.PHONY: $(ALL_GOARCH) all test install clean deb

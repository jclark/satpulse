CONFIG_DIR=/usr/local/etc/satpulse
CMD=github.com/jclark/satpulse/internal/cmd
BUILD_DATE:=$(shell date -u --rfc-3339=seconds)
DIRTY:=$(shell git diff-index --quiet HEAD || echo .dirty)
GIT_VERSION:=$(shell env TZ=UTC git log -1 --format="%cd.%h" --date=format-local:%Y%m%d)$(DIRTY)
DEB_VERSION=1
DEB_PKG_VERSION:= 0.0~git$(GIT_VERSION)-$(DEB_VERSION)
RPM_RELEASE=1
RPM_VERSION=$(shell env TZ=UTC git log -1 --format="0^%cdgit%h" --date=format-local:%Y%m%d)
RPM_PKG_VERSION=$(RPM_VERSION)-$(RPM_RELEASE)
XFLAGS:=-X \"$(CMD).gitVersion=$(GIT_VERSION)\" -X \"$(CMD).buildDate=$(BUILD_DATE)\"
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

DEB_PATTERN=out/%/satpulse_$(DEB_PKG_VERSION)_%.deb
DEBS=$(patsubst %,$(DEB_PATTERN), $(ALL_GOARCH))

deb: $(DEBS)

out/satpulse@.service: satpulse@.service
	sed -e 's;/usr/local/etc/;/etc/;g' -e 's;/usr/local/;/usr/;g' $< >$@

$(DEB_PATTERN): % out/%/default.toml out/satpulse@.service
	install -D -m 644 debian/conffiles out/$*/deb/DEBIAN/conffiles
	install -D debian/postinst out/$*/deb/DEBIAN/postinst
	install -D out/$*/satpulsed out/$*/deb/usr/sbin/satpulsed
	install -D out/$*/satpulsetool out/$*/deb/usr/bin/satpulsetool
	install -D -m 644 out/$*/default.toml out/$*/deb/etc/satpulse/default.toml
	install -D -m 644 configs/ptp4l.service out/$*/deb/usr/share/doc/satpulse/ptp4l.service
	install -D -m 644 configs/chrony.conf out/$*/deb/usr/share/doc/satpulse/chrony.conf
	install -D -m 644 LICENSE out/$*/deb/usr/share/doc/satpulse/copyright
	install -D -m 644 out/satpulse@.service out/$*/deb/lib/systemd/system/satpulse@.service
	installed_size=`du -s -k out/$*/deb | cut -f1`;\
	sed -e '/^Architecture:/s/any/$*/' -e '/^Package:/a\
	Version: $(DEB_PKG_VERSION)' -e '/^Maintainer:/a\
	Installed-Size: '"$$installed_size" debian/control >out/$*/deb/DEBIAN/control
	dpkg-deb --root-owner-group --build out/$*/deb out/$*

RPM_PATTERN=out/%/satpulse-$(RPM_PKG_VERSION).%.rpm
ALL_RPM_ARCH=aarch64 x86_64
RPMS=$(patsubst %,$(RPM_PATTERN),$(ALL_RPM_ARCH))
TOMLS:=$(patsubst %,out/%/default.toml,$(ALL_GOARCH))
rpm: $(RPMS)

$(RPM_PATTERN): $(ALL_GOARCH) $(TOMLS) out/satpulse@.service
	goarch=$(subst x86_64,amd64,$(subst aarch64,arm64,$*)); \
	test -L out/$* || ln -s $$goarch out/$*; \
	cwd=`pwd`; \
	rpmbuild -bb --target $* --define "goarch $$goarch" \
	--build-in-place \
	--buildroot "$$cwd/out/$*/rpm" \
	--define "version $(RPM_VERSION)" \
	--define "_rpmdir $$cwd/out" \
	satpulse.spec

.PHONY: $(ALL_GOARCH) all test install clean deb rpm

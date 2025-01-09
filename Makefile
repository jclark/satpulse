# Makefile for satpulse: requires GNU make
# Where the config file will be installed by the install target.
CONFIG_FILE=/usr/local/etc/satpulse.toml
CMD=github.com/jclark/satpulse/internal/cmd
BUILD_DATE:=$(shell date -u --rfc-3339=seconds)
DIRTY:=$(shell git diff-index --quiet HEAD || echo .dirty)
GIT_VERSION:=$(shell env TZ=UTC git log -1 --format="%cd.%h" --date=format-local:%Y%m%d)$(DIRTY)
DEB_VERSION=1
DEB_PKG_VERSION:= 0.0~git$(GIT_VERSION)-$(DEB_VERSION)
RPM_RELEASE=1
RPM_VERSION=$(shell env TZ=UTC git log -1 --format="0^%cdgit%h" --date=format-local:%Y%m%d)
GITHUB_RELEASE=$(shell env TZ=UTC git log -1 --format="%cd" --date=format-local:%Y%m%d)
RPM_PKG_VERSION=$(RPM_VERSION)-$(RPM_RELEASE)
XFLAGS:=-X \"$(CMD).gitVersion=$(GIT_VERSION)\" -X \"$(CMD).buildDate=$(BUILD_DATE)\"
TAGS=netgo,osusergo
# The GOARCHs we support.
ALL_GOARCH=arm64 amd64
TOMLS:=$(patsubst %,out/%/satpulse.toml,$(ALL_GOARCH))
ARCH:=$(shell uname -m)

# Set goarch based on detected architecture
ifeq ($(ARCH),x86_64)
GOARCH:=amd64
else ifeq ($(ARCH),aarch64)
GOARCH:=arm64
else
$(error Unsupported architecture $(ARCH))
endif

all: $(GOARCH) out/$(GOARCH)/satpulse.toml

allarch: $(ALL_GOARCH) $(TOMLS)

$(ALL_GOARCH):
	env GOOS=linux GOARCH=$@ go build -tags "$(TAGS)" -o out/$@/ -ldflags "$(XFLAGS)" ./...

out/arm64/satpulse.toml: configs/satpulse.toml
	sed -e '/^#:schema /s; \./; /usr/share/doc/satpulse/;' -e '/^interface/s/enp1s0/eth0/' -e '/^device/s/ttyUSB0/ttyAMA0/' $< > $@

out/amd64/satpulse.toml: configs/satpulse.toml
	sed -e '/^#:schema /s; \./; /usr/share/doc/satpulse/;' $< > $@

install: out/$(GOARCH)/satpulsed out/$(GOARCH)/satpulsetool out/$(GOARCH)/satpulse.toml
	install out/$(GOARCH)/satpulsed /usr/local/sbin/satpulsed
	install out/$(GOARCH)/satpulsetool /usr/local/bin/satpulsetool
	sed -e 's;/etc/satpulse.toml;$(CONFIG_FILE);g' \
	  -e 's;/etc/satpulse.d/;/usr/local/etc/satpulse.d/;g' \
	  -e 's;/usr/sbin/satpulsed;/usr/local/sbin/satpulsed;g' \
	  configs/satpulse@.service >/etc/systemd/system/satpulse@.service
	[ -f "$(CONFIG_FILE)" ] || sed -e '/^#:schema /s;/usr/;/usr/local/;' out/$(GOARCH)/satpulse.toml >"$(CONFIG_FILE)"
	install -m 644 -D configs/config-schema.json /usr/local/share/doc/satpulse/config-schema.json
	install -D -m 644 doc/config.md /usr/local/share/doc/satpulse/config.md
	install -D -m 644 doc/quickstart.md /usr/local/share/doc/satpulse/quickstart.md
	systemctl daemon-reload

test:
	go test -v ./...

clean:
	-rm -rf out

DEB_PATTERN=out/%/satpulse_$(DEB_PKG_VERSION)_%.deb
DEBS=$(patsubst %,$(DEB_PATTERN), $(ALL_GOARCH))
deb: $(DEBS)

$(DEB_PATTERN): % out/%/satpulse.toml
	install -D -m 644 debian/conffiles out/$*/deb/DEBIAN/conffiles
	install -D debian/postinst out/$*/deb/DEBIAN/postinst
	install -D out/$*/satpulsed out/$*/deb/usr/sbin/satpulsed
	install -D out/$*/satpulsetool out/$*/deb/usr/bin/satpulsetool
	install -D -m 644 out/$*/satpulse.toml out/$*/deb/etc/satpulse.toml
	install -D -m 644 configs/ptp4l.service out/$*/deb/usr/share/doc/satpulse/ptp4l.service
	install -D -m 644 configs/chrony.conf out/$*/deb/usr/share/doc/satpulse/chrony.conf
	install -D -m 644 configs/config-schema.json out/$*/deb/usr/share/doc/satpulse/config-schema.json
	install -D -m 644 doc/config.md out/$*/deb/usr/share/doc/satpulse/config.md
	install -D -m 644 doc/quickstart.md out/$*/deb/usr/share/doc/satpulse/quickstart.md
	install -D -m 644 LICENSE out/$*/deb/usr/share/doc/satpulse/copyright
	install -D -m 644 configs/satpulse@.service out/$*/deb/lib/systemd/system/satpulse@.service
	installed_size=`du -s -k out/$*/deb | cut -f1`;\
	sed -e '/^Architecture:/s/any/$*/' -e '/^Package:/a\
	Version: $(DEB_PKG_VERSION)' -e '/^Maintainer:/a\
	Installed-Size: '"$$installed_size" debian/control >out/$*/deb/DEBIAN/control
	dpkg-deb --root-owner-group --build out/$*/deb out/$*

RPM_PATTERN=out/%/satpulse-$(RPM_PKG_VERSION).%.rpm
ALL_RPM_ARCH=aarch64 x86_64
RPMS=$(patsubst %,$(RPM_PATTERN),$(ALL_RPM_ARCH))
rpm: $(RPMS)

$(RPM_PATTERN): $(ALL_GOARCH) $(TOMLS)
	goarch=$(subst x86_64,amd64,$(subst aarch64,arm64,$*)); \
	test -L out/$* || ln -s $$goarch out/$*; \
	cwd=`pwd`; \
	rpmbuild -bb --target $* --define "goarch $$goarch" \
	--build-in-place \
	--buildroot "$$cwd/out/$*/rpm" \
	--define "version $(RPM_VERSION)" \
	--define "_rpmdir $$cwd/out" \
	satpulse.spec

release: $(DEBS) $(RPMS)
	@if ! gh auth status >/dev/null 2>&1; then \
		echo "GitHub CLI is not authenticated. Run 'gh auth login --insecure-storage' on the host and try again."; \
		exit 1; \
	fi
	gh release create "v$(GITHUB_RELEASE)" \
		--repo "jclark/satpulse" \
		--title "Release v$(GITHUB_RELEASE)" \
		--draft \
		$(DEBS) $(RPMS)

.PHONY: $(ALL_GOARCH) all test install clean deb rpm release

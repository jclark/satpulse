# Makefile for satpulse: requires GNU make
# Where the config file will be installed by the install target.
CONFIG_FILE=/usr/local/etc/satpulse.toml
CMD=github.com/jclark/satpulse/gps/app/cmd
VERSION:=$(shell cat VERSION)
VERSION_TAG:=v$(VERSION)
BUILD_DATE:=$(shell date -u --rfc-3339=seconds)
DIRTY:=$(shell git diff-index --quiet HEAD || echo .dirty)
GIT_VERSION:=$(shell env TZ=UTC git log -1 --format="%cd.%h" --date=format-local:%Y%m%d)$(DIRTY)
DEB_VERSION=1
RPM_RELEASE=1
CMD_VERSION:=$(VERSION)
DEB_PKG_VERSION:=$(VERSION)-$(DEB_VERSION)
RPM_VERSION:=$(VERSION)
GH_RELEASE:=$(VERSION)
CURRENT_TAG:=$(shell git describe --tags --exact-match 2>/dev/null)
ifneq ($(CURRENT_TAG)$(DIRTY),$(VERSION_TAG))
# It's a prerelease
CMD_VERSION:=$(VERSION)-pre.$(GIT_VERSION)
DEB_PKG_VERSION:=$(VERSION)~git$(GIT_VERSION)-$(DEB_VERSION)
RPM_VERSION:=$(VERSION)~$(shell env TZ=UTC git log -1 --format="%cdgit%h" --date=format-local:%Y%m%d)
GH_RELEASE:=$(VERSION)-pre-$(shell env TZ=UTC git log -1 --format="%cd" --date=format-local:%Y%m%d)
endif
RPM_PKG_VERSION=$(RPM_VERSION)-$(RPM_RELEASE)
XFLAGS:=-X \"$(CMD).version=$(CMD_VERSION)\" -X \"$(CMD).buildDate=$(BUILD_DATE)\"
TAGS=netgo,osusergo
# The GOARCHs we support.
ALL_GOARCH=arm64 amd64
TOMLS:=$(patsubst %,out/%/satpulse.toml,$(ALL_GOARCH))
ARCH:=$(shell uname -m)
MAN_PAGES=satpulsetool.1 satpulsetool-gps.1 satpulsetool-sdp.1 satpulse.toml.5 satpulsed.8
MAN_TARGETS = $(addprefix out/, $(MAN_PAGES))
MAN_GZ_TARGETS = $(addsuffix .gz, $(MAN_TARGETS))

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

man: $(MAN_TARGETS)

man.gz: $(MAN_GZ_TARGETS)

out/%: docs/man/%.md
	pandoc -s --metadata=title="$(basename $*)" --metadata=section="$(subst .,,$(suffix $*))" --metadata=author="James Clark" -t man -o $@ $<

out/%.gz: out/%
	gzip -c $< > $@

install: out/$(GOARCH)/satpulsed out/$(GOARCH)/satpulsetool out/$(GOARCH)/satpulse.toml $(MAN_TARGETS)
	install out/$(GOARCH)/satpulsed /usr/local/sbin/satpulsed
	install out/$(GOARCH)/satpulsetool /usr/local/bin/satpulsetool
	sed -e 's;/etc/satpulse.toml;$(CONFIG_FILE);g' \
	  -e 's;/etc/satpulse.d/;/usr/local/etc/satpulse.d/;g' \
	  -e 's;/usr/sbin/satpulsed;/usr/local/sbin/satpulsed;g' \
	  configs/satpulse@.service >/etc/systemd/system/satpulse@.service
	[ -f "$(CONFIG_FILE)" ] || sed -e '/^#:schema /s;/usr/;/usr/local/;' out/$(GOARCH)/satpulse.toml >"$(CONFIG_FILE)"
	install -m 644 -D configs/config-schema.json /usr/local/share/doc/satpulse/config-schema.json
	install -D -m 644 out/satpulsetool.1 /usr/local/share/man/man1/satpulsetool.1
	install -D -m 644 out/satpulsetool-gps.1 /usr/local/share/man/man1/satpulsetool-gps.1
	install -D -m 644 out/satpulsetool-sdp.1 /usr/local/share/man/man1/satpulsetool-sdp.1
	install -D -m 644 out/satpulse.toml.5 /usr/local/share/man/man5/satpulse.toml.5
	install -d /usr/local/share/man/man8
	sed 's;/etc/satpulse.toml;$(CONFIG_FILE);g' out/satpulsed.8 > /usr/local/share/man/man8/satpulsed.8
	systemctl daemon-reload

uninstall:
	systemctl stop 'satpulse@*.service'
	rm -f /etc/systemd/system/satpulse@.service
	rm -f /usr/local/sbin/satpulsed
	rm -f /usr/local/bin/satpulsetool
	# we don't uninstall /usr/local/etc/satpulse.toml
	rm -f /usr/local/share/doc/satpulse/config-schema.json
	rm -f /usr/local/share/man/man1/satpulsetool.1
	rm -f /usr/local/share/man/man1/satpulsetool-gps.1
	rm -f /usr/local/share/man/man1/satpulsetool-sdp.1
	rm -f /usr/local/share/man/man5/satpulse.toml.5
	rm -f /usr/local/share/man/man8/satpulsed.8
	systemctl daemon-reload

test:
	go test ./...

clean:
	-rm -rf out

pkg: deb rpm

DEB_PATTERN=out/satpulse_$(DEB_PKG_VERSION)_%.deb
GH_DEB_PATTERN=out/satpulse_$(GH_RELEASE)_%.deb
DEBS:=$(patsubst %,$(DEB_PATTERN), $(ALL_GOARCH))
GH_DEBS:=$(patsubst %,$(GH_DEB_PATTERN), $(ALL_GOARCH))
deb: $(GH_DEBS)

$(GH_DEBS): $(DEBS)

$(GH_DEB_PATTERN): $(DEB_PATTERN)
	ln -sf $(notdir $<) $@

$(DEB_PATTERN): % out/%/satpulse.toml $(MAN_GZ_TARGETS)
	rm -fr out/$*/deb
	install -D -m 644 debian/conffiles out/$*/deb/DEBIAN/conffiles
	install -D debian/postinst out/$*/deb/DEBIAN/postinst
	install -D out/$*/satpulsed out/$*/deb/usr/sbin/satpulsed
	install -D out/$*/satpulsetool out/$*/deb/usr/bin/satpulsetool
	install -D -m 644 out/$*/satpulse.toml out/$*/deb/etc/satpulse.toml
	install -D -m 644 configs/ptp4l.service out/$*/deb/usr/share/doc/satpulse/ptp4l.service
	install -D -m 644 configs/chrony.conf out/$*/deb/usr/share/doc/satpulse/chrony.conf
	install -D -m 644 configs/config-schema.json out/$*/deb/usr/share/doc/satpulse/config-schema.json
	install -D -m 644 LICENSE out/$*/deb/usr/share/doc/satpulse/copyright
	install -D -m 644 configs/satpulse@.service out/$*/deb/lib/systemd/system/satpulse@.service
	install -D -m 644 out/satpulsetool.1.gz out/$*/deb/usr/share/man/man1/satpulsetool.1.gz
	install -D -m 644 out/satpulsetool-gps.1.gz out/$*/deb/usr/share/man/man1/satpulsetool-gps.1.gz
	install -D -m 644 out/satpulsetool-sdp.1.gz out/$*/deb/usr/share/man/man1/satpulsetool-sdp.1.gz
	install -D -m 644 out/satpulse.toml.5.gz out/$*/deb/usr/share/man/man5/satpulse.toml.5.gz
	install -D -m 644 out/satpulsed.8.gz out/$*/deb/usr/share/man/man8/satpulsed.8.gz
	installed_size=`du -s -k out/$*/deb | cut -f1`;\
	sed -e '/^Architecture:/s/any/$*/' -e '/^Package:/a\
	Version: $(DEB_PKG_VERSION)' -e '/^Maintainer:/a\
	Installed-Size: '"$$installed_size" debian/control >out/$*/deb/DEBIAN/control
	dpkg-deb --root-owner-group --build out/$*/deb out

RPM_PATTERN=out/satpulse-$(RPM_PKG_VERSION).%.rpm
GH_RPM_PATTERN=out/satpulse-$(GH_RELEASE).%.rpm
ALL_RPM_ARCH=aarch64 x86_64
RPMS:=$(patsubst %,$(RPM_PATTERN),$(ALL_RPM_ARCH))
GH_RPMS:=$(patsubst %,$(GH_RPM_PATTERN),$(ALL_RPM_ARCH))
rpm: $(GH_RPMS)

$(GH_RPMS): $(RPMS)

# The challenge here is that rpmbuild wants to put the generated RPMs in a subdirectory named by the architecture.
# But if we follow that we will get endless pain from having patterns with two %s in them.
# So we symlink each architecture to the current directory to avoid having the subdirectories.
$(RPM_PATTERN): $(ALL_GOARCH) $(TOMLS) $(MAN_GZ_TARGETS)
	goarch=$(subst x86_64,amd64,$(subst aarch64,arm64,$*)); \
	test -L out/$* || ln -s . out/$*; \
	echo ls -l out/$*; \
	cwd=`pwd`; \
	rpmbuild -bb --target $* --define "goarch $$goarch" \
	--build-in-place \
	--buildroot "$$cwd/out/$$goarch/rpm" \
	--define "debug_package %{nil}" \
	--define "version $(RPM_VERSION)" \
	--define "_rpmdir $$cwd/out" \
	satpulse.spec

$(GH_RPM_PATTERN): $(RPM_PATTERN)
	ln -sf $(notdir $<) $@

release: $(GH_DEBS) $(GH_RPMS)
	@if ! gh auth status >/dev/null 2>&1; then \
		echo "GitHub CLI is not authenticated. Run 'gh auth login --insecure-storage' on the host and try again."; \
		exit 1; \
	fi
	gh release create "v$(GH_RELEASE)" \
		--repo "jclark/satpulse" \
		--title "Release v$(GH_RELEASE)" \
		--notes "Automatically generated draft release" \
		--draft \
		$^

tag:
	git diff-index --exit-code HEAD
	git tag -f -a "$(VERSION_TAG)" -m "Release $(VERSION_TAG)"

untag:
	git tag -d "$(VERSION_TAG)"

.PHONY: $(ALL_GOARCH) all test install uninstall clean pkg deb rpm release man man.gz tag untag


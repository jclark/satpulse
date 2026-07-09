.PHONY: build deb dev lint clean realclean setup deb-docker-amd64 deb-docker-arm64

# WebKitGTK version Wails links against on Linux. 4.1 (libsoup3) is shipped by
# all current distros (Ubuntu 24.04+, Debian trixie+, Fedora 40+); use 4.0
# (libsoup2) on older distros. Override: `make WEBKIT=4.0`. macOS (WKWebView)
# and Windows (WebView2) ignore this -- no tag is needed there.
WEBKIT = 4.1

# TAGFLAG resolves, in the recipe shell, to the Wails build tag: the webkit2_41
# tag on Linux when WEBKIT=4.1, and empty otherwise. Kept in the shell so the
# Makefile needs no GNU-specific conditionals.
TAGFLAG = $$([ "$$(uname -s)" = Linux ] && [ "$(WEBKIT)" = 4.1 ] && echo -tags webkit2_41)

# The frontend's @satpulse/workbench file: dependency is a symlink into
# the webui workspace, whose own dependencies must be installed first.
build:
	[ -d ../webui/node_modules ] || npm -C ../webui install
	wails build $(TAGFLAG)

# Package a .deb for the Debian-based distro this runs on (builds first if
# needed), plus the friendly GH_RELEASE-named symlink.
deb:
	./build-deb.sh
	./deb-symlink.sh

# Build the .deb in a container for a specific architecture (default base
# debian:trixie; override via env vars documented in docker-build-deb.sh).
deb-docker-amd64:
	./docker-build-deb.sh amd64

deb-docker-arm64:
	./docker-build-deb.sh arm64

setup:
	cd .. && go work init . ./desktop

lint:
	cd frontend && npm run lint

dev:
	wails dev $(TAGFLAG)

clean:
	rm -rf build/bin

realclean:
	rm -rf build frontend/wailsjs ../go.work ../go.work.sum

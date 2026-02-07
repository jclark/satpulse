.PHONY: build dev clean realclean setup

build:
	wails build

setup:
	cd .. && go work init . ./desktop

dev:
	wails dev

clean:
	rm -rf build/bin

realclean:
	rm -rf build frontend/wailsjs ../go.work ../go.work.sum

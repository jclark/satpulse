# Makefile to regenerate the convobs golden RINEX fixtures: requires GNU make.
# Each golden <name>.obs.gz is RTKLIB Explorer convbin output, gzipped. This
# Makefile is the single source of truth for the convbin flags; README.md and
# the TestGoldenFiles comment in convobs_test.go refer here rather than repeat
# them. Regenerate only when the reference decoder changes, and review the
# observation differences before committing (see README.md).
#
# CONVBIN must be the RTKLIB Explorer build (commit pinned in README.md), not
# the system convbin. Override it to point at that build:
#   make CONVBIN=$(HOME)/rtklib-ex/bin/convbin
CONVBIN ?= convbin
# Common convbin flags: RINEX 3.04 output with Doppler and SNR observables.
CONVBIN_FLAGS = -v 3.04 -od -os
GOLDEN = m8t-20251217.obs.gz f9t-20251217.obs.gz packet-rtcm-20260519.obs.gz \
	um980-rtcm-20260527.obs.gz um980-uncb-20260527.obs.gz

all: $(GOLDEN)

# Specifying -o suppresses convbin's default nav outputs, so only the .obs is
# written; gzip -n keeps the golden reproducible (no name or timestamp).

# u-blox UBX captures.
%.obs.gz: %.ubx
	$(CONVBIN) -r ubx $(CONVBIN_FLAGS) -ro "-MULTICODE -MAX_STD_CP=15" -o $(basename $@) $<
	gzip -nf $(basename $@)

# Unicore OBSVM stream.
%.obs.gz: %.uncb
	$(CONVBIN) -r unicore $(CONVBIN_FLAGS) -o $(basename $@) $<
	gzip -nf $(basename $@)

# RTCM MSM streams: -tr gives convbin the start date to resolve the GPS week.
packet-rtcm-20260519.obs.gz: packet-rtcm-20260519.rtcm
	$(CONVBIN) -r rtcm3 $(CONVBIN_FLAGS) -ro "-MULTIMODE -INVPRR" -tr 2026/5/19 0:0:0 -o $(basename $@) $<
	gzip -nf $(basename $@)

um980-rtcm-20260527.obs.gz: um980-rtcm-20260527.rtcm
	$(CONVBIN) -r rtcm3 $(CONVBIN_FLAGS) -ro "-MULTIMODE -INVPRR" -tr 2026/5/27 0:0:0 -o $(basename $@) $<
	gzip -nf $(basename $@)

clean:
	-rm -f *.obs

.PHONY: all clean

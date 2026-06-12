ARCH =$(shell uname -m)
ifeq ($(ARCH),x86_64)
GOARCH =amd64
else ifeq ($(ARCH),aarch64)
GOARCH =arm64
else
$(error Unsupported architecture $(ARCH))
endif

SATPULSETOOL = ../../out/$(GOARCH)/satpulsetool
SATPULSETOOL_FLAGS = -v
SYNCSIM_FLAGS = --stats 600

CONFIGS =cm4-f9t i225-m8t timehat-f9t

RESULTS =$(addsuffix .result.toml,$(CONFIGS))
LOGS =$(addsuffix .syncsim.log,$(CONFIGS))
TS_LOGS =$(addsuffix .ts.jsonl,$(CONFIGS))
CLOCK_LOGS =$(addsuffix .clock.log,$(CONFIGS))

all: $(RESULTS)

%.result.toml: %.syncsim.toml
	$(SATPULSETOOL) $(SATPULSETOOL_FLAGS) syncsim $(SYNCSIM_FLAGS) \
	  --clock-log $*.clock.log \
	  --ts-log $*.ts.jsonl \
	  $< >$@ 2>$*.syncsim.log

clean:
	rm -f $(RESULTS) $(LOGS) $(TS_LOGS) $(CLOCK_LOGS)

.PHONY: all clean

CC ?= cc
CFLAGS ?= -Wall -Wextra
LDLIBS = -framework IOKit -framework CoreFoundation
OUTDIR = .

$(OUTDIR)/find-serial: find-serial.c
	mkdir -p $(OUTDIR)
	$(CC) $(CFLAGS) -o $@ $< $(LDLIBS)

clean:
	rm -f $(OUTDIR)/find-serial

.PHONY: clean

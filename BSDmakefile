# bmake reads this in preference to Makefile (.MAKE.MAKEFILE_PREFERENCE is
# "BSDmakefile makefile Makefile"), so make on FreeBSD never parses the
# GNU-make-only Makefile. GNU make never reads this file.
.DEFAULT:
	@${MAKE} -f Makefile.unix $@

# .DEFAULT covers every goal but these two: it supplies no default goal, so a
# bare make needs all, and it does not apply to a goal that exists as a file,
# so the smoketest directory would otherwise be reported up to date. They get
# their own rule because bmake ignores extra targets on the .DEFAULT rule line.
all smoketest: .PHONY
	@${MAKE} -f Makefile.unix $@

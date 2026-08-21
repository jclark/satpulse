# bmake reads this in preference to Makefile (its .MAKE.MAKEFILE_PREFERENCE is
# "BSDmakefile makefile Makefile"), so plain make on FreeBSD never parses the
# GNU-make-only Makefile. GNU make never reads this file. .DEFAULT supplies the
# recipe for any goal that has no rule, including goals named on the command
# line; bmake will not let other targets share that rule line, so they need
# their own.
.DEFAULT:
	@${MAKE} -f Makefile.unix $@

# all so that a bare make works, smoketest because .DEFAULT does not apply to a
# goal that exists as a file (here, the smoketest directory).
all smoketest: .PHONY
	@${MAKE} -f Makefile.unix $@

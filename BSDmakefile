# BSD make reads this in preference to Makefile, which is GNU make only.
.DEFAULT:
	@${MAKE} -f Makefile.unix $@

# all is needed because a make without arguments builds the first target in the
# file, but .DEFAULT does not work for this. smoketest is needed because .DEFAULT
# is not used for a goal that names an existing file or directory.
all smoketest: .PHONY
	@${MAKE} -f Makefile.unix $@

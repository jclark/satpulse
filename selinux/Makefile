
satpulse.pp.bz2: satpulse.pp
	bzip2 -k -f -9 $<

satpulse.pp: satpulse.mod
	semodule_package -o $@ -m $<

satpulse.mod: satpulse.te
	checkmodule -M -m -o $@ $<

clean:
	-rm -f satpulse.mod satpulse.pp

realclean: clean
	-rm -f satpulse.pp.bz2

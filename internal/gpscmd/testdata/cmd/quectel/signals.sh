# Test cases for signal selection on the LG290P (PQTMCFGSIGNAL/CNST).
# Signal changes are stored-only (signalOnlyWithReset): a request
# without --save --reload is skipped with a warning, and an effective
# change needs the save plus the PQTMSRR reboot.
t --gnss GPS,GAL
# Disable GLONASS effectively: gate-only CFGCNST write + save + reload
t --gnss GPS,GAL,BDS,QZSS,NAVIC --save --reload
sleep 20
t --show-config
# Restore the full default set
t --gnss GPS,GLO,GAL,BDS,QZSS,NAVIC --save --reload
sleep 20
t --show-config

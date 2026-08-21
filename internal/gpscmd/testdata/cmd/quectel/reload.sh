# Test --reload (PQTMSRR restart, reloading NVM) on the LG290P.
# Make a live change, confirm it, reload from NVM, confirm it reverted.
t --min-elev 10
t --show-config
t --reload
# PQTMSRR reboots the module; wait for it to come back before probing.
sleep 20
t --show-config

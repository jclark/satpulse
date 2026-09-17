# Test saving configuration to NVM: a minimal save after a single
# change, save-all, and whether the saved configuration survives
# reload (V5 reloads from NVM; V6 firmware cannot reload). Ends by
# saving the default minimum elevation back.

t --min-elev 10 --save
t --reload
sleep 2
t --show-config
t --min-elev 5 --save-all
t --reload
sleep 2
t --show-config
t --min-elev 0 --save

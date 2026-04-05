# Test cases for --gnss and --band options
# Tests various GNSS constellation and band configurations
# Single combos
t --gnss GPS
t --gnss GAL
t --gnss BDS
t --gnss GLO
# Combos of two
t --gnss GPS,GAL
t --gnss GPS,GLO
t --gnss GPS,BDS
t --gnss GLO,GAL
t --gnss GLO,BDS
t --gnss GAL,BDS
# Combos of three
t --gnss GPS,BDS,GAL
t --gnss GPS,BDS,GLO
t --gnss GAL,BDS,GLO
t --gnss GPS,GAL,GLO
# All four
t --gnss GLO,GPS,BDS,GAL
# All four and QZSS
t --gnss GLO,GPS,BDS,GAL,QZSS
# F10T this works; but without --band it doesn't
t --gnss GAL,GLO,GPS --band L1
# Now some bands
t --gnss BDS --band L1,L2
t --gnss GPS,GAL --band L1
t --gnss BDS --band L1
t --gnss GPS,GAL --band L1,L5
t --gnss BDS,GAL --band L1,L5
t --gnss GPS,GAL,BDS --band L1,E5
t --gnss GPS,GAL,BDS --band L1,E5,E6

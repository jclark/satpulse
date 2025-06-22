# Test cases for --gnss and --band options
# Tests various GNSS constellation and band configurations
# GPS only
t --gnss GPS
# GAL only
t --gnss GAL
# GPS + Galileo
t --gnss GPS,GAL
# GPS + Galileo + GLONASS
t --gnss GPS,GAL,GLO
# BeiDou only
t --gnss BDS
# BeiDou with specific bands
t --gnss BDS --band L1,L2
t --gnss GPS,GAL --band L1
t --gnss BDS --band L1
t --gnss GLO
t --gnss GPS,BDS,GLO
t --gnss GLO,GPS,BDS,GAL,QZSS

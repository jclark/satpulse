# Test cases for --gnss and --band options
# Tests various GNSS constellation and band configurations

delay=0.5

# GPS only
t --gnss GPS
sleep $delay

# GPS + Galileo
t --gnss GPS,GAL
sleep $delay

# GPS + Galileo + GLONASS
t --gnss GPS,GAL,GLO
sleep $delay

# BeiDou only
t --gnss BDS
sleep $delay

# BeiDou with specific bands
t --gnss BDS --band L1,L2
sleep $delay

t --gnss GPS,GAL --band L1
sleep $delay

t --gnss BDS --band L1
sleep $delay

t --gnss GLO
sleep $delay

t --gnss GLO,GPS,BDS,GAL,QZSS
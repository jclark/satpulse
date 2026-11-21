# Test cases for --nmea-out option on the LG290P (PQTMCFGMSGRATE)
# Single sentence
t --nmea-out RMC
# Two sentences
t --nmea-out RMC,GGA
# A wider set
t --nmea-out GGA,GSA,GSV
# Turn all NMEA off
t --nmea-out none
# Restore the full as-found NMEA table
t --nmea-out RMC,GGA,GSA,GSV,VTG,GLL

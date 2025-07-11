# Test --binary flag functionality
# Tests enabling binary output and disabling NMEA

# Start off with --nmea enabled
t --nmea

# Basic binary mode (should disable NMEA, enable default PVT messages)
t --binary

# Binary mode with explicit PVT output
t --binary --pvt-out pos

# Binary mode with time output
t --binary --pvt-out time

# Binary mode with multiple PVT flags
t --binary --pvt-out pos,vel,time

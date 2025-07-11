# Test cases for --raw-out option
# Enable observation/measurement messages (for RINEX)
t --raw-out obs
# Enable navigation data messages (e.g., GPS subframe data)
t --raw-out nav
# Enable both obs and nav messages
t --raw-out obs,nav
# Disable all raw data messages
t --raw-out none
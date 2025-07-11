# Test cases for --rtcm-out option
# Enable MSM4 messages for all enabled GNSS constellations
t --rtcm-out MSM4
# Enable MSM7 messages for all enabled GNSS constellations
t --rtcm-out MSM7
# Enable Antenna Reference Point messages (RTCM message type 1005)
t --rtcm-out ARP
# Disable all RTCM messages
t --rtcm-out none
# Combined MSM4 and ARP
t --rtcm-out MSM4,ARP
# Combined MSM7 and ARP
t --rtcm-out MSM7,ARP
# intelligent selection of MSM messages
t --rtcm-out auto
# intelligent selection of MSM messages with MSM7 preference
t --rtcm-out auto,MSM7
# Leave nothing enabled
t --rtcm-out none

# Test cases for --pvt-out option
# Basic position output
t --pvt-out pos
# Basic velocity output
t --pvt-out vel
# Basic time output
t --pvt-out time
# Time pulse output
t --pvt-out tp
# Leap second information
t --pvt-out leap
# Survey-in progress messages
t --pvt-out survey
# Position with ECEF coordinates
t --pvt-out pos,ecef
# Velocity with ECEF coordinates
t --pvt-out vel,ecef
# Time with TAI preference
t --pvt-out time,tai
# Time pulse with TAI preference
t --pvt-out tp,tai
# Time pulse with after message
t --pvt-out tp,after
# Turn off non-explicit PVT messages
#t --pvt-out off
# Time pulse daemon flags with off
t --pvt-out tp,after,tai,leap,survey,off
# Combined position, velocity, time
t --pvt-out pos,vel,time
# Time pulse with after and TAI
t --pvt-out tp,after,tai
# All position/velocity with ECEF
t --pvt-out pos,vel,ecef
# Time and leap second info with TAI
t --pvt-out time,tai,leap
# Full combo with survey
t --pvt-out pos,vel,time,tp,leap,survey

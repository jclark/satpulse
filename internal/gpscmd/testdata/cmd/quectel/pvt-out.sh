# Test cases for --pvt-out option on the LG290P (PQTMPVT/EPE/DOP/EOE/SVINSTATUS)
# Basic position
t --pvt-out pos
# Position, velocity, time
t --pvt-out pos,vel,time
# Accuracy estimate (PQTMEPE) and end-of-epoch (PQTMEOE)
t --pvt-out qual,epoch
# Survey-in status (PQTMSVINSTATUS; mode-gated, refused in rover mode)
t --pvt-out survey
# Full combination
t --pvt-out pos,vel,time,tp,leap,qual,epoch,survey
# Turn PVT output off
t --pvt-out off

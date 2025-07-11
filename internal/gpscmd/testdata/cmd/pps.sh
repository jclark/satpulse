# Test --pps, --time-gnss and --ant-cable-delay
t --pps 0.1
t --pps 0.1 --time-gnss GAL
t --pps 0.5 --time-gnss BDS
t --pps 0
t --pps 0.01 
t --ant-cable-delay 150
t --pps 0.1 --ant-cable-delay 100 --time-gnss GPS


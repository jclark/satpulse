# Test --show-port output for read-only port discovery
# Exercises both the standalone form (port + serial speed when applicable)
# and composition with --show-config.

t --show-port
t -c --show-port

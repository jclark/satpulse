These are files that I use to automate system testing of SatPulse.

`clocklog.py` analyzes clock logs (in /var/log/satpulse.clock.IFNAME.log), enabled by `clock = true` in the `[log]` table in satpulse.toml.

The `.yml` files are Ansible playbooks:

* `inventory.yml` is the Ansible inventory that defines variables and hosts
* `install.yml` installs the SatPulse package
* `config.yml` edits the satpulse.toml configuration file
* `start.yml` starts the satpulse daemon
* `check.yml` checks that the current run of the satpulse daemon has functioned correctly
* `stop.yml` stops the satpulse daemon

Typically I would first deploy a new set of packages to my testing machines.

```
ansible-playbook -K -i inventory.yml install.yml -l testing
``` 

Then start them up:

```
ansible-playbook -K -i inventory.yml start.yml -l testing
```

Then wait 30 seconds or so and do:

```
ansible-playbook -K -i inventory.yml check.yml -l testing
```

Then repeat this again after some number of hours.

## Adding a new machine

`newhost.yml` commissions a Raspberry Pi freshly imaged with Raspberry Pi Imager (Raspberry Pi OS Lite)
as a test host: passwordless sudo, authorized keys, a static address, UARTs, chrony and linuxptp,
then a reboot, `install.yml` and `config.yml`. Add the host to the inventory (`install` group and its
receiver settings), then run it with the `newhost` script:

```
./newhost -s SITE-VARS.yml [-a A.B.C.D] HOST
```

Without `-a` the host stays on DHCP.

The site variables file (gateway, DNS, authorized keys, NTP sources; described at the top of the
playbook) can also be given by `SATPULSE_SITE_VARS`. `./newhost -h` shows all options.

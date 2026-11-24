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
then a reboot, `install.yml` and `config.yml`.

While it is commissioned the host has two names: `HOST.lan`, its permanent name, which DNS gives
the static address it is to have, and `HOST.local`, the mDNS name it announces at its temporary DHCP
address. So, first:

1. Add `HOST.lan` to DNS with the static address.
2. Image the Pi with Raspberry Pi Imager: hostname `HOST`, user and password, ssh enabled. Connect
   eth0 to the network (the static address goes on eth0) and boot it.
3. Add `HOST.lan` to the inventory: the `install` group, its receiver settings, and any `rpi_*`
   settings (UART overlays, PPS GPIO; listed at the top of the playbook). No `ansible_host`.

Then run:

```
./newhost -s SITE-VARS.yml HOST
```

It looks up the static address, connects as `HOST.local` (putting your ssh key on the host first if
needed), runs the playbook, which reconnects at the static address after the reboot, and asks for
the login and sudo passwords as it needs them. `--via ADDR` reaches the host at ADDR instead of
`HOST.local`; `--dhcp` leaves it on DHCP; `--address` overrides the DNS address. It can be run again
on a commissioned host.

The site variables file (gateway, DNS, authorized keys, NTP sources; described at the top of the
playbook) can also be given by `SATPULSE_SITE_VARS`. `./newhost -h` shows all options.

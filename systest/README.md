These are Ansible playbooks that I use to automate system testing of SatPulse.

* `inventory.yml` is the Ansible inventory that defines variables and hosts
* `install.yml` installs the SatPulse package
* `config.yml` edits the satpulse.toml configuration file
* `start.yml` starts the satpulse daemon
* `check.yml` checks that the current run of the satpulse daemon has functioned correctly
* `stop.yml` stops the satpulse daemon

Typically I would first deploy a new set of packages to my testing machines.

```
ansible-playbook -K -i install.yml start.yml -l testing
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
---
title: Network configuration
---

For a time server, you probably want a static IP address. How you configure one depends on how your operating system is managing the network.

* NetworkManager is the default on both Raspberry Pi OS, starting with Bookworm, and on Fedora

## Network Manager


You can see the current connections using:

```
nmcli c show
```


Assuming the connection on the interface is named `Wired connection 1`, you can change it to a static IP using:

```
nmcli c mod "Wired connection 1" ipv4.method manual \
  ipv4.addresses 192.168.1.5/24 \
  ipv4.gateway 192.168.1.1 \
  ipv4.dns 8.8.8.8 \
  ipv4.dns-search lan
```

Adjust the IP addresses to match your network configuration.

You can activate the changes with:

```
nmcli d reapply eth0
```

Replace `eth0` with your actual interface name if different.

## systemd-networkd

If you have problems with NetworkManager, you can try systemd-networkd which is part of the systemd suite. In this case, you may want to

* enable `systemd-networkd-wait-online@eth0.service` (replacing eth0 with your interface name) to ensure the network is ready before starting timing services and
* disable NetworkManager-wait-online.service.



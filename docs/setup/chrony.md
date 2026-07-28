---
title: Chrony setup
toc: false
sitemap: false
---

Make sure chrony is installed. The package is called `chrony` on both Fedora and Debian.

Add this to `satpulse.toml`:

```
[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

This will make satpulsed send timing samples to chrony using its SOCK protocol through a socket at `/var/run/chrony.satpulse.sock`. Then restart the satpulse service.

Add this line to your chrony configuration:

```
refclock SOCK /var/run/chrony.satpulse.sock poll 2 filter 4 refid GNSS
```

On Fedora, just add it to `/etc/chrony.conf`.
On Debian, I suggest making it a separate file `/etc/chrony/conf.d/satpulse.conf`.

This adds a refclock called `GNSS` to chrony that will read samples from  `/var/run/chrony.satpulse.sock` using the SOCK protocol.
The socket path in the chrony configuration needs to match the socket path in the satpulse configuration.

Then restart chrony. The service is named `chrony` on Debian, and `chronyd` on Fedora.

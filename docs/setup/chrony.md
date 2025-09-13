---
title: Chrony setup
---

Make sure chrony is installed. The package is called `chrony` on both Fedora and Debian.

Add this line to your chrony configuration:

```
refclock SOCK /var/run/chrony.satpulse.sock poll 2 filter 4 refid GNSS
```

On Fedora, just add it to `/etc/chrony.conf`.
On Debian, I suggest making it a separate file `/etc/chrony/conf.d/satpulse.conf`.
The socket path here `/var/run/chrony.satpulse.sock` needs to match that specified by the `sock.path` key in the `ntp` table in `satpulse.toml`.

Then restart chrony. The service is named `chrony` on Debian, and `chronyd` on Fedora.

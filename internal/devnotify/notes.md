# Netlink for udev events

Want to listen to family `NETLINK_KOBJECT_UEVENT`. The group is GROUP_UDEV which is defined in private systemd header as 2.

See
* https://insujang.github.io/2018-11-27/udev-device-manager-for-the-linux-kernel-in-userspace/

Format of message is different for udev generated messages.
Relevant code is in systemd `src/libsystemd/sd-device/device-monitor.c` in `device_monitor_receive_device`.

See also https://github.com/snapcore/snapd/blob/master/osutil/udev/netlink/uevent.go

See also https://github.com/mdlayher/kobject/ (this deals with kernel events)
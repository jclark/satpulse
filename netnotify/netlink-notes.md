
# Notes on how to use netlink

We need to detect CARRIER, NO-CARRIER events as in dhcpcd

This describes states:

https://www.kernel.org/doc/html/latest/networking/operstates.html

Corresponds to IFF_RUNNING flag in the interface flags.

https://man7.org/linux/man-pages/man7/netdevice.7.html

With SIOCGIFFLAGS ioctl, will be ifr_flags in struct ifreq `<net/if.h>`.


Example from https://mdlayher.com/blog/linux-netlink-and-go-part-3-packages-netlink-genetlink-and-wifi/

```
// Listen to rtnetlink for modification of network interfaces
const rtnetlink = 0
const rtmGroupLink = 0x1

conn, _ := netlink.Dial(rtnetlink, nil)
defer conn.Close()

// Join multicast group: Receive will block until messages arrive.
_ = conn.JoinGroup(rtmGroupLink)
msgs, _ := conn.Receive()
_ = conn.LeaveGroup(rtmGroupLink)
```

Also:

https://pkg.go.dev/github.com/mdlayher/netlink#example-Conn-ListenMulticast

https://man7.org/linux/man-pages/man7/rtnetlink.7.html

Need RTM_NEWLINK message. Will be in ifi_flags.

```
struct ifinfomsg {
    unsigned char  ifi_family; /* AF_UNSPEC */
    unsigned short ifi_type;   /* Device type */
    int            ifi_index;  /* Interface index */
    unsigned int   ifi_flags;  /* Device flags  */
    unsigned int   ifi_change; /* change mask */
};
```

This is unix.Ifinfomsg. Also SizeofIfInfomsg

Also see:

* https://gitlab.com/mergetb/tech/rtnl
* https://github.com/jsimonetti/rtnetlink

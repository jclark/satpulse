#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <errno.h>
#include <getopt.h>
#include <limits.h>
#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sysexits.h>
#include <unistd.h>

#define EXIT_AMBIGUOUS 2

struct dev {
	struct dev *next;
	char *path;
	char *model, *vendor, *serial;
	uint32_t loc;
	uint16_t vid, pid;
};

struct opts {
	bool have_vid, have_pid, have_loc, do_exec, do_wait;
	uint32_t vid, pid, loc;
	const char *serial;
};

static void free_dev(struct dev *dev);

static void fatal(int code, const char *fmt, ...)
{
	va_list ap;
	fprintf(stderr, "find-serial: ");
	va_start(ap, fmt);
	vfprintf(stderr, fmt, ap);
	va_end(ap);
	fputc('\n', stderr);
	exit(code);
}

static void *xmalloc(size_t n)
{
	void *p = malloc(n);
	if (!p)
		fatal(EXIT_FAILURE, "out of memory");
	return p;
}

static void *xcalloc(size_t n, size_t size)
{
	void *p;
	if (n && size > (size_t)-1 / n)
		fatal(EXIT_FAILURE, "out of memory");
	p = xmalloc(n * size);
	memset(p, 0, n * size);
	return p;
}

static char *xstrdup(const char *s)
{
	char *p = strdup(s);
	if (!p)
		fatal(EXIT_FAILURE, "out of memory");
	return p;
}

static void usage(FILE *f)
{
	fprintf(f, "usage: find-serial [-v|--vid HEX] [-p|--pid HEX] [-s|--serial STR] [-l|--location HEX]\n"
	           "                   [-w|--wait] [-e|--exec -- command ... {} ...]\n");
}

static int hexarg(const char *s, unsigned long max, uint32_t *v)
{
	char *end;
	unsigned long n;
	errno = 0;
	n = strtoul(s, &end, 16);
	if (errno || end == s || *end || n > max)
		return -1;
	*v = (uint32_t)n;
	return 0;
}

static char *string_prop(io_registry_entry_t e, CFStringRef key)
{
	char buf[PATH_MAX];
	CFTypeRef p = IORegistryEntryCreateCFProperty(e, key, kCFAllocatorDefault, 0);
	if (!p)
		return NULL;
	if (CFGetTypeID(p) != CFStringGetTypeID() ||
	    !CFStringGetCString((CFStringRef)p, buf, sizeof(buf), kCFStringEncodingUTF8))
		buf[0] = 0;
	CFRelease(p);
	return buf[0] ? xstrdup(buf) : NULL;
}

static char *callout_device(io_registry_entry_t e)
{
	for (int i = 0; i < 5; i++) {
		char *path = string_prop(e, CFSTR("IOCalloutDevice"));
		if (path)
			return path;
		usleep(50000);
	}
	return NULL;
}

// uint_prop reads an unsigned integer property. It fetches as long long
// because locationID does not fit a signed 32-bit read once the top bit is
// set, and the kernel may publish it as either width.
static bool uint_prop(io_registry_entry_t e, CFStringRef key, uint32_t *v)
{
	long long n = 0;
	bool ok = false;
	CFTypeRef p = IORegistryEntryCreateCFProperty(e, key, kCFAllocatorDefault, 0);
	if (p) {
		ok = CFGetTypeID(p) == CFNumberGetTypeID() &&
		    CFNumberGetValue((CFNumberRef)p, kCFNumberLongLongType, &n);
		CFRelease(p);
	}
	if (ok)
		*v = (uint32_t)n;
	return ok;
}

// usb_info walks up from a serial service to its USB device node, filling in
// the device's vendor/product IDs and location ID, and, when present, its own
// serial number, product and vendor name strings (left NULL otherwise). It
// reads the kUSB*String keys rather than the "USB Product Name"/"USB Vendor
// Name"/"USB Serial Number" ones, as the former carry the descriptor strings
// faithfully while the latter mangle some characters.
static bool usb_info(io_registry_entry_t e, struct dev *d)
{
	io_registry_entry_t cur = e, parent;
	kern_return_t kr;
	char cls[128];
	for (;;) {
		kr = IORegistryEntryGetParentEntry(cur, kIOServicePlane, &parent);
		if (cur != e)
			IOObjectRelease(cur);
		if (kr != KERN_SUCCESS)
			return false;
		cur = parent;
		kr = IOObjectGetClass(cur, cls);
		if (kr != KERN_SUCCESS)
			fatal(EXIT_FAILURE, "IOObjectGetClass failed: %d", kr);
		if (strcmp(cls, "IOUSBDevice") == 0 || strcmp(cls, "IOUSBHostDevice") == 0) {
			uint32_t vid = 0, pid = 0;
			if (!uint_prop(cur, CFSTR("idVendor"), &vid) ||
			    !uint_prop(cur, CFSTR("idProduct"), &pid) ||
			    !uint_prop(cur, CFSTR("locationID"), &d->loc))
				fatal(EXIT_FAILURE, "USB device is missing idVendor, idProduct or locationID");
			d->vid = (uint16_t)vid;
			d->pid = (uint16_t)pid;
			d->serial = string_prop(cur, CFSTR("kUSBSerialNumberString"));
			d->model = string_prop(cur, CFSTR("kUSBProductString"));
			d->vendor = string_prop(cur, CFSTR("kUSBVendorString"));
			IOObjectRelease(cur);
			return true;
		}
	}
}

// serial_matching returns a matching dictionary for serial-port devices. The
// caller passes ownership to an IOKit function that consumes the reference.
static CFMutableDictionaryRef serial_matching(void)
{
	CFMutableDictionaryRef m = IOServiceMatching("IOSerialBSDClient");
	if (!m)
		fatal(EXIT_FAILURE, "out of memory");
	return m;
}

// match reports whether a device satisfies every filter given. A device that
// publishes no serial number never matches --serial.
static bool match(const struct opts *o, const struct dev *d)
{
	return (!o->have_vid || o->vid == d->vid) && (!o->have_pid || o->pid == d->pid) &&
	    (!o->have_loc || o->loc == d->loc) &&
	    (!o->serial || (d->serial && strcmp(o->serial, d->serial) == 0));
}

static struct dev *scan1(const struct opts *o, bool *valid)
{
	io_iterator_t it;
	io_service_t s;
	kern_return_t kr;
	struct dev *head = NULL, *tail = NULL;
	kr = IOServiceGetMatchingServices(kIOMainPortDefault, serial_matching(), &it);
	if (kr != KERN_SUCCESS)
		fatal(EXIT_FAILURE, "IOServiceGetMatchingServices failed: %d", kr);
	while ((s = IOIteratorNext(it))) {
		struct dev *d = xcalloc(1, sizeof(*d));
		d->path = callout_device(s);
		if (d->path && usb_info(s, d) && match(o, d)) {
			if (tail)
				tail->next = d;
			else
				head = d;
			tail = d;
		} else {
			free_dev(d);
		}
		IOObjectRelease(s);
	}
	*valid = IOIteratorIsValid(it);
	IOObjectRelease(it);
	return head;
}

// scan enumerates matching devices, setting *stable to whether the registry
// held still long enough to trust the result. An empty result is reported as
// stable even when the iterator went invalid: nothing was found, so there is no
// device (this happens with startup registry churn, e.g. on CI runners). Only a
// non-empty result from a changing registry is retried, with a backoff doubling
// from 1ms, so 5 tries wait at most 15ms total; if it never settles, *stable is
// cleared and NULL returned, leaving the caller to decide whether that is fatal.
static struct dev *scan(const struct opts *o, bool *stable)
{
	useconds_t delay = 1000;
	*stable = true;
	for (int tries = 5;;) {
		bool valid;
		struct dev *dev = scan1(o, &valid);
		if (valid || !dev)
			return dev;
		free_dev(dev);
		if (--tries == 0)
			break;
		usleep(delay);
		delay *= 2;
	}
	*stable = false;
	return NULL;
}

static void free_dev(struct dev *dev)
{
	while (dev) {
		struct dev *next = dev->next;
		free(dev->path);
		free(dev->model);
		free(dev->vendor);
		free(dev->serial);
		free(dev);
		dev = next;
	}
}

struct wait_ctx {
	const struct opts *o;
	struct dev *dev;
};

// wait_cb fires on each IOSerialBSDClient first-match notification. The
// iterator must be fully drained to re-arm the notification; the drained
// services are not used directly. Once at least one matching device is found
// it stops the run loop, leaving the result in ctx->dev. A registry too
// unstable to scan is ignored, so we keep waiting for the next notification.
static void wait_cb(void *refcon, io_iterator_t iter)
{
	struct wait_ctx *ctx = refcon;
	io_service_t s;
	bool stable;
	while ((s = IOIteratorNext(iter)))
		IOObjectRelease(s);
	if (ctx->dev)
		return;
	ctx->dev = scan(ctx->o, &stable);
	if (ctx->dev)
		CFRunLoopStop(CFRunLoopGetCurrent());
}

// wait_for_dev blocks using IOKit notifications until scan() finds at least one
// matching device, then returns the list. It does not poll.
static struct dev *wait_for_dev(const struct opts *o)
{
	struct wait_ctx ctx = {o, NULL};
	IONotificationPortRef np = IONotificationPortCreate(kIOMainPortDefault);
	if (!np)
		fatal(EXIT_FAILURE, "IONotificationPortCreate failed");
	CFRunLoopSourceRef src = IONotificationPortGetRunLoopSource(np);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), src, kCFRunLoopDefaultMode);
	io_iterator_t iter;
	kern_return_t kr = IOServiceAddMatchingNotification(np, kIOFirstMatchNotification,
	    serial_matching(), wait_cb, &ctx, &iter);
	if (kr != KERN_SUCCESS)
		fatal(EXIT_FAILURE, "IOServiceAddMatchingNotification failed: %d", kr);
	wait_cb(&ctx, iter);
	if (!ctx.dev)
		CFRunLoopRun();
	IOObjectRelease(iter);
	CFRunLoopRemoveSource(CFRunLoopGetCurrent(), src, kCFRunLoopDefaultMode);
	IONotificationPortDestroy(np);
	return ctx.dev;
}

static int check_exec(int argc, char **argv)
{
	int n = 0;
	for (int i = optind; i < argc; i++)
		if (strcmp(argv[i], "{}") == 0)
			n++;
	if (optind == argc) {
		fprintf(stderr, "find-serial: --exec requires a command\n");
		return -1;
	}
	if (n != 1) {
		fprintf(stderr, "find-serial: --exec requires exactly one {} argument\n");
		return -1;
	}
	return 0;
}

static int parse_opts(int argc, char **argv, struct opts *o)
{
	static struct option longopts[] = {
		{"vid", required_argument, 0, 1},
		{"pid", required_argument, 0, 2},
		{"exec", no_argument, 0, 3},
		{"help", no_argument, 0, 4},
		{"wait", no_argument, 0, 5},
		{"serial", required_argument, 0, 6},
		{"location", required_argument, 0, 7},
		{0, 0, 0, 0},
	};
	int c;
	opterr = 0;
	while ((c = getopt_long(argc, argv, "v:p:s:l:ew", longopts, NULL)) != -1) {
		switch (c) {
		case 1:
		case 'v':
			o->have_vid = true;
			if (hexarg(optarg, 0xffff, &o->vid) < 0) {
				usage(stderr);
				return EX_USAGE;
			}
			break;
		case 2:
		case 'p':
			o->have_pid = true;
			if (hexarg(optarg, 0xffff, &o->pid) < 0) {
				usage(stderr);
				return EX_USAGE;
			}
			break;
		case 7:
		case 'l':
			o->have_loc = true;
			if (hexarg(optarg, 0xffffffff, &o->loc) < 0) {
				usage(stderr);
				return EX_USAGE;
			}
			break;
		case 6:
		case 's': o->serial = optarg; break;
		case 3:
		case 'e': o->do_exec = true; break;
		case 5:
		case 'w': o->do_wait = true; break;
		case 4: usage(stdout); return 0;
		default: usage(stderr); return EX_USAGE;
		}
	}
	if (o->do_exec) {
		if (check_exec(argc, argv) < 0)
			return EX_USAGE;
		return -1;
	}
	if (optind != argc) {
		usage(stderr);
		return EX_USAGE;
	}
	return -1;
}

// print_field writes ` KEY="value"` for a non-NULL value, mapping any double
// quote or non-printable-ASCII byte to '_'. This keeps the quoted field
// unambiguous (no embedded quote can close it early) without escaping; spaces
// and other printable ASCII pass through verbatim.
static void print_field(const char *key, const char *val)
{
	if (!val)
		return;
	printf(" %s=\"", key);
	for (const unsigned char *p = (const unsigned char *)val; *p; p++)
		putchar(*p == '"' || *p < 0x20 || *p > 0x7e ? '_' : *p);
	putchar('"');
}

static int list_or_exec(struct dev *dev, const struct opts *o, char **argv)
{
	if (!o->do_exec) {
		for (struct dev *d = dev; d; d = d->next) {
			printf("device=%s vid=%04X pid=%04X location=%X", d->path, d->vid,
			    d->pid, d->loc);
			print_field("serial", d->serial);
			print_field("model", d->model);
			print_field("vendor", d->vendor);
			putchar('\n');
		}
		return 0;
	}
	if (!dev) {
		fprintf(stderr, "find-serial: no matching USB serial device\n");
		return EX_UNAVAILABLE;
	}
	if (dev->next) {
		fprintf(stderr, "find-serial: multiple matching USB serial devices\n");
		return EXIT_AMBIGUOUS;
	}
	for (int i = optind; argv[i]; i++)
		if (strcmp(argv[i], "{}") == 0)
			argv[i] = dev->path;
	execvp(argv[optind], argv + optind);
	fprintf(stderr, "find-serial: exec %s: %s\n", argv[optind], strerror(errno));
	return EXIT_FAILURE;
}

int main(int argc, char **argv)
{
	struct opts o = {0};
	struct dev *dev;
	int ret = parse_opts(argc, argv, &o);
	if (ret >= 0)
		return ret;
	bool stable;
	dev = scan(&o, &stable);
	if (!stable)
		fatal(EXIT_FAILURE, "I/O registry changed during enumeration");
	if (!dev && o.do_wait)
		dev = wait_for_dev(&o);
	ret = list_or_exec(dev, &o, argv);
	free_dev(dev);
	return ret;
}

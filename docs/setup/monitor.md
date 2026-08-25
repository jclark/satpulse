---
title: Monitoring satpulsed
---

There are multiple ways to monitor the operation of `satpulsed`.

In the examples below, `ptp.lan` is used as the hostname of the server running `satpulsed`. Replace this with your actual server hostname or IP address.

## Application-specific log files

SatPulse can write detailed logs to files in `/var/log/satpulse/`.

To enable logging, add a `[log]` table to your configuration file:

```toml
[log]
clock = true
packet = true
```

The available log types are:

* `clock` - logs PHC offsets and servo adjustments in text format
* `packet` - logs all packets received from and sent to the GPS receiver in JSON Lines format
* `event` - logs events in JSON Lines format; events are the device-independent model of information from the receiver; they are derived from packets and pulse edges

By default, log files are written to `/var/log/satpulse/`. You can change this with the `dir` key:

```toml
[log]
packet = true
dir = "/var/log/satpulse-logs"
```

The clock log file is named e.g. `clock.eth0.log` where `eth0` is the network interface name. It uses a line-oriented format with space-separated fields. A comment line starting with `#` gives the meaning of the fields. This format is designed to be convenient for use with Unix tools such as `awk`.

There is also a Python analysis script [`clocklog.py`](https://github.com/jclark/satpulse/blob/master/systest/clocklog.py) that analyzes a clock log file and prints various statistics.

```
python3 clocklog.py /var/log/satpulse/clock.eth0.log
```

The packet log file is named e.g. `packet.ttyAMA0.jsonl` where `ttyAMA0` is the serial device name. It is in [JSON Lines](https://jsonlines.org/) format (one JSON object per line), making it easy to process with tools like `jq`.

The event log file is named e.g. `event.ttyAMA0.jsonl`. Each line is one event with a `type` field such as `time`, `posGeo`, `satellites` or `phcPulseEdge` and a `data` field with structured content. `satpulsetool replay` generates the same events from a packet log, except that this does not include pulse edge events.

SatPulse includes an example [logrotate configuration](https://github.com/jclark/satpulse/blob/master/configs/satpulse.logrotate) that can be installed at `/etc/logrotate.d/satpulse` to rotate these logs.

## HTTP monitoring interface

SatPulse provides an HTTP monitoring interface with both a web GUI and Prometheus metrics endpoint.

To enable HTTP monitoring, add an `[[http]]` table to your configuration file:

```toml
[[http]]
listen = ":2000"
```

This will start an HTTP server on port 2000 on all network interfaces.

By default, both the web GUI and Prometheus metrics are enabled. You can disable either one:

```toml
[[http]]
listen = ":2000"
gui = false        # Disable web GUI 
metrics = false    # Disable Prometheus metrics
```

The HTTP interface provides read-only monitoring and does not require authentication.
Make sure your firewall rules are appropriate for your network environment.
To listen only on a specific interface:

```toml
[[http]]
listen = "192.168.1.100:2000"
```

### Web GUI

The web interface can be accessed at `http://ptp.lan:2000/` in a web browser,
where `2000` is whatever port you specified for `listen`.

The web interface can display detailed satellite information including a sky view and a signal strength bar graph, but the GPS receiver must be configured to output this data. If

- you have both HTTP monitoring and GPS configuration enabled,
- you have a GPS that satpulsed knows how to configure, and
- the serial speed is at least 38400 baud

`satpulsed` will automatically configure the GPS to provide this information.
If you are configuring the GPS receiver yourself,
include NMEA GSV and GSA messages to enable the satellite views.

Note that the sky view uses opacity to show signal strength
(satellites with weaker signals will appear more faint.)

### Prometheus and Grafana

[Prometheus](https://prometheus.io/) is an open source monitoring system,
which can fetch metrics over HTTP and store them in a database.
[Grafana](https://grafana.com/) is a visualization tool that can query Prometheus and display metrics on customizable dashboards.

SatPulse can provide Prometheus metrics relating to the PHC and to the GPS satellites,
which can then be displayed visually in Grafana.
This is more complex than using the Web GUI, but is much more flexible and powerful.
Grafana supports JavaScript plugins, which even make it possible to generate a satellites sky view
from the metrics.

The metrics names all start with `satpulse_`.
You can view the `/metrics` endpoint in your browser (e.g., `http://ptp.lan:2000/metrics`) to see all available metrics with their documentation.

#### Prometheus

On Debian-based systems, install Prometheus via apt:

```
sudo apt install prometheus
```

Add this to your Prometheus `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'satpulse'
    scrape_interval: 1s
    static_configs:
      - targets: ['ptp.lan:2000']
```

A `scrape_interval` of 1 second provides the most detailed information and allows you to see second-by-second changes in PHC offset and frequency. However, if this creates too much load on your Prometheus server, you can use a longer interval (e.g., `5s` or `10s`).

#### Grafana

Follow the [installation instructions](https://grafana.com/docs/grafana/latest/setup-grafana/installation/) for your platform.

SatPulse includes some [example Grafana dashboards](https://github.com/jclark/satpulse/blob/master/configs/grafana/README.md), which you can import into Grafana. You will need to configure Grafana to use your Prometheus instance as the data source.

## TCP proxy for Windows applications

There are a number of Windows applications for GPS monitoring that can work over a TCP connection.
In particular

- most GPS vendors have an application that works with their GPS modules, for example
  - u-blox have [u-center](https://www.u-blox.com/en/product/u-center)
  - Unicore have [UPrecise](https://en.unicore.com/products/uprecise.html)
  - AllyStar has Satrack
- [Lady Heather](http://www.ke5fx.com/heather/readme.htm) is an open source GPS monitoring tool (use with `/ip` option)

To use these with SatPulse, add a `[[proxy.tcp]]` table to your configuration:

```toml
[[proxy.tcp]]
listen = ":2006"
```

SatPulse does not perform any access control on the TCP proxy. Anyone on the network can connect to the proxy and interact with your GPS receiver. Vendor tools typically require read-write access to configure the GPS, which means anyone with network access can change your GPS configuration. Only enable the TCP proxy on trusted networks.

You can then connect your GPS tool to `ptp.lan:2006`.

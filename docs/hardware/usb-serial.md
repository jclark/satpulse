---
title: USB serial adapters
---

In some cases you can directly connect a GPS receiver to a computer without a USB serial adapter:

- an SBC such as a Raspberry Pi typically has GPIO pins which can be connected to the pins on a GPS module
- some desktop computers have RS232 DB9 male connectors which can be connected to a GPS enclosure that has an RS232 DB9 female connector
- some GPS receivers have a USB connector which can be connected to a computer's USB port

In other cases, you will typically need a USB serial adapter.

The serial connection can be used for a PPS signal as well as for transmitting and receiving serial data.

There are two main kinds of adapter.

## TTL level

Unless your GPS module has an RS-232 DB9 connector, you will need a TTL-level adapter using 3.3V signals.
These typically have male or female Dupont 2.54mm pitch connectors.
GPS modules typically have a connector with at least 4 or 5 pins.
It may be a male Dupont 2.54mm pitch connector.
It may just have solder holes, which you have to solder a 2.54mm pin header onto.
Or it may use some sort of JST-style wire-to-board connector.

The pins on the board are typically:

- VIN or VCC pin for supplying electrical voltage to the module
- GND for ground
- TXD for transmitting messages from the module to the computer
- RXD for receiving messages to the module from the computer
- PPS for a PPS signal from the module to the computer

Some modules lack a PPS pin and they are not suitable for timing applications.

In choosing this kind of USB serial adapter, you need to consider the following points.

1. The signals need to be TTL-level not RS-232 level.
2. The adapter's VCC needs to supply the right voltage for your module.
   Note that the voltage level for the signals is independent of the VCC voltage level.
   Most boards accept 3.3V-5V, but some need 5V. There may be some that only accept 3.3V.
   Some adapters supply 3.3V, some supply 5V and some have a jumper or switch that allows you to choose.
   I recommend the last type.
3. If you are using this for timing, then the adapter needs to have at least CTS/RTS hardware flow control pins.
   The PPS pin of the module is connected to the CTS pin of the adapter.
   A few adapters have a full set of modem control lines including DCD; this is useful on Linux,
   since it enables you to use kernel PPS timestamping, but these are unusual and more expensive.
   But SatPulse can work well with the CTS pin.
4. The chipset manufacturer is usually FTDI (FT232R family), Prolific (PL2303 family), Silicon Labs (CP210x family) or WCH (CH34x family).
   I strongly recommend choosing one from FTDI particularly if you are using the connection for PPS.
   FTDI has the best cross-platform driver support. The cp210x driver on Linux does not support some important APIs.
   Some WCH chips use the CDC USB class, which does not allow the CTS pin to be used for PPS.
5. Most USB serial adapters are USB full speed, meaning they run at 12 Mbit/s, which was the maximum speed supported by USB 1.
   This is plenty for serial data. But there are a few USB serial adapters that use the FT232H chip, which is USB high speed, meaning it runs at 480 Mbit/s. The advantage of the high speed chip is it provides more consistent timing for the PPS signal in some USB topologies, since it avoids delays introduced by the need to translate between high speed and full speed transactions.
6. Some adapters physically consist of a cable with a USB connector at one end and Dupont female connectors at the other.
   Some adapters have no cable, and have Dupont male pins; these often include a separate jumper cable.
   I recommend the latter type, since with a suitable jumper cable they can work with boards using JST connectors.
   The USB side of the adapter is typically either USB A male or USB C female. I find the former more convenient when
   working with desktop computers, and the latter more convenient when working with laptop computers.

There are many suitable FTDI full-speed adapters. These use the FT232R chip, usually in the FT232RL package.
I like the ones from [Waveshare](https://www.waveshare.com/), which are available as a [USB C board](https://www.waveshare.com/ft232-usb-uart-board-type-c.htm),
[USB A board](https://www.waveshare.com/ft232-usb-uart-board-type-a.htm) or a [USB A dongle](https://www.waveshare.com/usb-to-ttl.htm).
The dongle has the disadvantage that it tends to block adjacent ports,
but it has the unusual feature of having two GND pins, which is convenient when you want to connect the PPS pin of a GPS board to an SDP on a PHC,
which also needs a ground connection,
while connecting the other pins on the GPS board to the USB serial adapter.

There are relatively few suitable FT232H adapters.
I specifically recommend the [Adafruit FT232H breakout board](https://www.adafruit.com/product/2264).
The only inconvenience is that you have to solder on a pin header.
It [supports the full set of modem control lines](https://learn.adafruit.com/adafruit-ft232h-breakout/serial-uart).
If you want an FT232H adapter but don't want to solder,
the other possibility is the [FTDI C232HD-EDHSP-0](https://ftdichip.com/products/c232hd-edhsp-0/), which supplies 5V,
or [FTDI C232HD-DDHSP-0](https://ftdichip.com/products/c232hd-ddhsp-0/), which supplies 3.3V,
but these are rather expensive.

## RS-232 level

An adapter that uses RS-232 level signals is suitable when your GPS receiver has a DB9 female connector.
It is typically sold as a cable with an RS-232 DB9 male connector at one end and a USB A or USB C connector at the other.
Cables that support the full set of modem control lines are relatively rare.
As with TTL level serial adapters, I would recommend choosing something with an FT232R chipset.

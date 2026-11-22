---
title: Message file
toc: false
message_file_gallery:
  - url: /assets/images/wb-g5-message-file.png
    image_path: /assets/images/wb-g5-message-file.png
    alt: "Message file tab with the Septentrio mosaic-G5 message file loaded"
    title: "Message file for a mosaic-G5"
---

The Message file tab provides a low-level configuration model,
where the user selects messages to send from a library of message files,
organized by vendor.
A message file defines a collection of messages specific to a vendor protocol,
with the messages grouped under named tags.
You choose a file, select a tag, and click Send.


{% include gallery id="message_file_gallery" %}

Points to note:

* The message file library is compiled into the binary,
  so the dropdowns work without anything installed on disk.
  The **SATPULSE_GPSMSG_PATH** environment variable puts your own directories ahead of it
  (see [satpulsewb(1)]({% link man/satpulsewb.1.md %})).
* Messages received from the receiver are correlated with the messages that they are a response to,
  so where the protocol allows it you get an accepted or rejected line per message sent.
  Anything else is shown as it arrived; clicking on it decodes it.
* Port and Save appear only when the loaded file has tags that need them.

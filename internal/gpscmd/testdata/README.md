You can recreate the list of commands that was used to create a test file using

```
cat filename.jsonl | jq -r 'select(.type == "env") | "out/amd64/satpulsetool gps " + (.args | join(" "))'
```

Or to generate a command file format (like signal.sh):

```
cat filename.jsonl | jq -r 'select(.type == "env") | "t " + (.args[6:] | join(" "))'
```

This skips the device connection arguments and formats the output for use in command files.
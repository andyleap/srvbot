# srvbot
Simple bot for control and monitoring of a server over IRC or Slack.
Run it on all your servers and be able to control all of them directly.

Currently in alpha, not offering pre-built binaries at this point.

# Usage
Edit srvbot.json with your desired endpoints, commands, logfiles, and monitors.

TODO: Change over from json to something a little more human friendly

# Commands
All commands are parsed from IRC using sh/bash style quotes, i.e. `status cron` becomes `"status", "cron"` and `this "is a" test` becomes `"this", "is a", "test"`

Once parsed, the first "word" is matched against the nickname and the list of groups, and the second "word" refers to the specific command.  If a match does not exist, the line is ignored.

If it matches, the command is processed by replacing `$0`, `$1`, `$2` and so on with the appropriate values from the parsed line.

$1 refers to the command name itself, $2 the first argument, and so on.  This allows rather complex commands like `service $1 status | head -n 3 | tail -n 1 | grep $2` where you could do something like `status cron dead` and only those servers where cron was dead would respond(note, this example is for a system running systemd, tune to match your service manager)

# Logging
Logs operate in 2 ways, Live, and Held.  Live logs output directly to specific channels as new log lines come in, and Held logs store the last X lines, and output them on demand.  A log can be both Live and Held at the same time, if desired.  Logs can be filtered by regex using golang's regexp library [Syntax](https://github.com/google/re2/wiki/Syntax).

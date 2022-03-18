# pflog
Page Fault Logging

Basic logging with buffering for better context leading up to an issue.

## Configuration
Configuration is supported via yaml files.  Yaml was chosen to allow for the usage of comments as desired/needed.
```bash
settings:
  level: [ Trace, Debug, Information, Warning, Error, Fatal ]
  trigger_level: [ Debug, Information, Warning, Error, Fatal ]
  backlog: 500
formatters:
  -
    id: [ text, yaml, json ]
    filename: "whatever.txt"
```

## Details
### Settings
#### Level
 Level can be one of the indicated levels and should be less than the trigger level.
#### Trigger Level
 The trigger level should always be one more than the standard level.  The trigger level is where something is triggered to dump out the backlog context.
#### Back log
 The backlog is how deep of a backlog that should be kept of logs (all levels) to be dumped when triggered.
### Formatters
#### ID
 The ID of the formatter which can currently be one of the three shown, text, yaml, or json.
#### Filename
 The name of the file to use for the given formatter.
 
## To Do's
Add more context i.e. logging "areas" to better differentiate between code areas.

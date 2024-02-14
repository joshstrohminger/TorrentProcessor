# Torrent Processor

This is a utility for handling torrents once they have been completed. The `add` command should be used by the torrent client and will write the completed torrent's details to a JSON file to a configurable directory. The `process` command should be used separately (perhaps as part of a service/daemon) to poll that directory for new files. It will attempt to process them based on the provided category. If it fails to parse the file, it will retry with exponential backoff. If it fails for other reasons it will ignore the file until it is restarted. Files are deleted once they've been successfully processed. This is designed to make is clear that work is still pending and easy to retry, whether a file failed to be processed, or the processor wasn't running.

## Logging

Structured logs are written to the same directory as the executable. If there is an issue determining that directory, it will fall back to the [lumberjack](https://github.com/natefinch/lumberjack) default location. Each command writes to a separate logfile and it's assumed that only one instance of each command is running at a time.

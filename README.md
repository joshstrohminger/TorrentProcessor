# Torrent Processor

This is a utility for handling torrents once they have been completed. The `add` command should be used by the torrent client and will write the completed torrent's details to a JSON file to a configurable directory. The `process` command should be used separately (perhaps as part of a service/daemon) to poll that directory for new files. It will attempt to process them based on the provided category. If it fails to parse the file, it will retry with exponential backoff. If it fails for other reasons it will ignore the file until it is restarted. Files are deleted once they've been successfully processed. This is designed to make is clear that work is still pending and easy to retry, whether a file failed to be processed, or the processor wasn't running.

## Configuration

A file can be provided directly via the global `--config` option.

If one is not provided, or a directory is specified, this will search for one named _tp.yaml_ ([other extensions/formats](https://pkg.go.dev/github.com/spf13/viper@v1.19.0#SupportedExts) may work but have not been tested) in the following directories in order:
1. The `--config` option interpretted as a directory, if provided.
2. In a _TorrentProccessor_ subdirectory in the user's OS-specific configuration directory (see [UserConfigDir](https://pkg.go.dev/os#UserConfigDir)).
3. In the user's OS-specific home directory (see [UserHomeDir](https://pkg.go.dev/os#UserHomeDir)).
4. **🚧 experimental**: In the current working directory, if run via `go run .` from the root of the source directory.
5. In the same directory as the executable.

Some information an utilies are provided:

```sh
go run . config --help
```

## Logging

Structured logs are written to the configured directory. Each command writes to a separate logfile and it's assumed that only one instance of each command is running at a time.

Some information an utilies are provided:

```sh
go run . logs --help
```

## Install

Install an OS-specific executable in `$(go env GOPATH)/bin` named named _TorrentProcessor_ (with a _.exe_ extension in Windows).

```sh
./install.sh
```

## Processing Daemon

To setup/install/start the processing daemon, run `TorrentProcessor process daemon start`. This will run as a GUI LaunchAgent, so it will start when the user logs in, and is triggered by files changing in the work directory.


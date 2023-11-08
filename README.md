# Syslog-NG Exporter for Prometheus and API

* Exports syslog-ng statistics via HTTP for Prometheus consumption.

Changes:
* blynn - Exposes syslog-ng RELOAD and HEALTHCHECK support
* blynn - Updated go code and layout to support new golang requirements

Help on flags:

```
  --help                        Show context-sensitive help (also try --help-long and --help-man).
  --version                     Print version information.
  --telemetry.address=":9577"   Address on which to expose metrics.
  --telemetry.endpoint="/metrics"
                                Path under which to expose metrics.
  --socket.path="/var/lib/syslog-ng/syslog-ng.ctl"
                                Path to syslog-ng control socket.
  --log.level="info"            Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]
  --log.format="logger:stderr"  Set the log target and format. Example: "logger:syslog?appname=bob&local=7" or "logger:stdout?json=true"
```

Tested with syslog-ng 3.5.6, 3.20.1, and 3.22.1.
blynn tested against 3.38

# Running/Building
> Note that the original code was likely developed with an older golang version. blynn made a few changes to
> support newer golang versions.
>
> The following steps are slightly different that the original build steps.
> 
> Also note that with the addition of `test-server.go`, go <run|build> require specifying the path as both 
> `test-server.go` and `syslog_ng_api.go` have a `main`

## Build steps
* go mod init
* go mod tidy
* go build syslog_ng_api.go
./syslog_ng_api --help

> Note that for IPE syslog-ng deployment, syslog_ng_api.go is required in the ipe-syslog-ng docker image.
> Make sure to copy the newly built `syslog_ng_api` to the ipe-syslog-ng folder and follow the steps to 
> create a new ipe-syslog-ng image.


## Using Docker
```
docker run -d -p 9577:9577 -v /var/lib/syslog-ng/syslog-ng.ctl:/syslog-ng.ctl \
  brandond/syslog_ng_api --socket.path=/syslog-ng.ctl
```


# Details

## Collectors

```
# HELP syslog_ng_destination_messages_dropped_total Number of messages dropped by this destination.
# TYPE syslog_ng_destination_messages_dropped_total counter
# HELP syslog_ng_destination_messages_processed_total Number of messages processed by this destination.
# TYPE syslog_ng_destination_messages_processed_total counter
# HELP syslog_ng_destination_messages_stored_total Number of messages stored by this destination.
# TYPE syslog_ng_destination_messages_stored_total gauge
# HELP syslog_ng_source_messages_processed_total Number of messages processed by this source.
# TYPE syslog_ng_source_messages_processed_total counter
# HELP syslog_ng_up Reads 1 if the syslog-ng server could be reached, else 0.
# TYPE syslog_ng_up gauge
```

## Author

* The exporter was originally created by [brandond](https://github.com/brandond), heavily inspired by the [apache_exporter](https://github.com/Lusitaniae/apache_exporter/).
* blynn added RELOAD and HEALTHCHECK support

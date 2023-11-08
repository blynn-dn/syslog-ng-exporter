# How to test
syslog-ng exposes metrics, health checks, etc. as well as reload support via a unix domain socket.  The 
socket file is created when syslog-ng starts.  The default path is /var/lib/syslog-ng/syslog-ng.ctl.

Since we deploy syslog-ng in a container (a K8s Pod to be exact), testing can prove somewhat difficult. So far 
I haven't had much success exposing the socket file external to the container.  While docker supports read/write 
access to a volume, K8s does not.  There are potentially a few work-around(s), but I haven't found a solution that
is "production" quality.

A few test deployment scenarios/options:
> Note that for all scenarios/options, the source files are cloned from the repo to
> ~/go/src/syslog_ng_api

## stand-alone
The stand-alone option is good for quick tests. This option uses a domain socket servers to
simulate syslog-ng.

First, start the domain socket server in a window or background the process.
```shell
/go/src/syslog_ng_api$ go run test-server.go
```

Next, start the API (make sure to specify the path to the control file)
```shell
~/go/src/syslog_ng_api$ go run syslog_ng_api.go --telemetry.address=":9991" --socket.path="/tmp/echo.sock" --log.level="debug"
```

Test
```shell
% curl -s http://<FQDN>:9991/metrics
# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 0
go_gc_duration_seconds{quantile="0.25"} 0
go_gc_duration_seconds{quantile="0.5"} 0
go_gc_duration_seconds{quantile="0.75"} 0
go_gc_duration_seconds{quantile="1"} 0
...
```

## docker
Deploying docker containers is probably the easiest solution.
The following is a simple docker-compose config.  
A few notes:
* A basic syslog-ng.conf is required.  The default does not listen externally. (see the example below)
* In order to access the socket/control file:
    * Use an empty volume and make it `rw` (read/write)
    * Tell syslog-ng the alternate path of the control file with flag `-c`
```shell
  syslog-ng:
    image: balabit/syslog-ng:latest
    command: /usr/sbin/syslog-ng -F -c /var/lib/syslog-ng/ctl/syslog-ng.ctl
    volumes:
      - ./files/syslog-ng/syslog-ng.conf:/etc/syslog-ng/syslog-ng.conf
      - ./files/syslog-ng/ctl:/var/lib/syslog-ng/ctl:rw
    ports:
      - "601:601"
      - "514:514/udp"  
```

Example syslog-ng config: 
```
@version: 3.38
@include "scl.conf"

options { chain_hostnames(off); flush_lines(0); 
    # disable dns resolution such that client IPs are used vs hostname
    #use_dns(yes); 
    use_fqdn(yes);
    dns_cache(yes); owner("root"); group("adm"); perm(0640);
    stats_freq(0); bad_hostname("^gconfd$");
};
 
########################
# Sources
########################
# This is the default behavior of sysklogd package
# Logs may come from unix stream, but not from another machine.
#
source s_src {
   system();
   internal();
};

# listen for TCP and UDP
source s_net { udp(); tcp(); network( transport("tcp") port("601") flags(syslog-protocol) ); };

destination d_syslog { file("/var/log/syslog"); };
filter f_syslog3 { not facility(auth, authpriv, mail) and not filter(f_debug); };

log { source(s_src); filter(f_syslog3); destination(d_syslog); };

# example syslog forward
destination d_promtail {
    # forward to promtail
    syslog(
        "promtail"
        transport("tcp")
        port(1514)
    );
}; 

log { source(s_net); destination(d_promtail); };
```

### Verify
Verify docker container
```shell
docker ps
CONTAINER ID   IMAGE         ...   PORTS                                                                                                                                 NAMES
5588504d41be   ipe-syslog-ng ...  0.0.0.0:601->601/tcp, :::601->601/tcp, 0.0.0.0:9577->9577/tcp, :::9577->9577/tcp, 6514/tcp, 0.0.0.0:2514->514/udp, :::2514->514/udp   ubuntu_ipe-syslog-ng_1
```
Change access to the control file
```shell
docker exec  ubuntu_ipe-syslog-ng_1 chmod 777 /var/lib/syslog-ng/ctl/syslog-ng.ctl
```

Quick test (make sure to use the host's control file path)
```shell
$ echo STATS | nc -U ~/files/syslog-ng/ctl/syslog-ng.ctl
SourceName;SourceId;SourceInstance;State;Type;Number
destination;d_mail;;a;processed;0
destination;d_cron;;a;processed;0
destination;d_error;;a;processed;165
...
```
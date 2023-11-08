#!/bin/bash

# turn on bash's job control
set -m

# start syslog-ng in the background
/usr/sbin/syslog-ng -F &

# use the following to expose the syslog control socket for testing
#/usr/sbin/syslog-ng -F -c /var/lib/syslog-ng/ctl/syslog-ng.ctl &

# start the API in the background
/usr/sbin/syslog_ng_api &

# now bring syslog-ng to the foreground
fg %1
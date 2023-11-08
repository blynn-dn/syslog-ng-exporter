package main

import (
	"fmt"
	"net"
	"os"
)

/*
This is a very simple unix domain socket server.  The idea is to provide a way to syslog-ng STATS w/o
a syslog-ng server.  If a client connects to this server via /tmp/echo.sock and sends "STATS", the
server will respond with mock data.  This simulates syslog-ng ctl file behavior.

Note that very little validation is in place so be warned.

This code was copied from https://eli.thegreenplace.net/2019/unix-domain-sockets-in-go/ with a few
changes.
*/

import (
	"bufio"
	"io"
	"log"
	"strings"
)

const SockAddr = "/tmp/echo.sock"

var statsMock = `SourceName;SourceId;SourceInstance;State;Type;Number
dst.file;d_mesg#0;/var/log/messages;a;dropped;0
dst.file;d_mesg#0;/var/log/messages;a;processed;610
dst.file;d_mesg#0;/var/log/messages;a;stored;0
destination;d_spol;;a;processed;0
src.internal;s_sys#2;;a;processed;72
src.internal;s_sys#2;;a;stamp;1556092051
center;;received;a;processed;72
src.unix-dgram;s_sys#0;/run/systemd/journal/syslog;a;processed;675
src.unix-dgram;s_sys#0;/run/systemd/journal/syslog;a;stamp;1556092606
destination;d_mesg;;a;processed;610
destination;d_mail;;a;processed;0
destination;d_auth;;a;processed;51
destination;d_mlal;;a;processed;0
center;;queued;a;processed;797
src.none;;;a;processed;0
src.none;;;a;stamp;0
destination;d_cron;;a;processed;111
global;payload_reallocs;;a;processed;88
global;sdata_updates;;a;processed;0
dst.file;d_kern#0;/var/log/kern;o;dropped;0
dst.file;d_kern#0;/var/log/kern;o;processed;25
dst.file;d_kern#0;/var/log/kern;o;stored;0
src.host;;l261767-vm;d;processed;772
src.host;;l261767-vm;d;stamp;1556092606
dst.file;d_cron#0;/var/log/cron;o;dropped;0
dst.file;d_cron#0;/var/log/cron;o;processed;111
dst.file;d_cron#0;/var/log/cron;o;stored;0
src.file;s_sys#1;/dev/kmsg;a;processed;25
src.file;s_sys#1;/dev/kmsg;a;stamp;1556091325
destination;d_boot;;a;processed;0
destination;d_kern;;a;processed;25
global;msg_clones;;a;processed;0
source;s_sys;;a;processed;72
dst.file;d_auth#0;/var/log/secure;a;dropped;0
dst.file;d_auth#0;/var/log/secure;a;processed;51
dst.file;d_auth#0;/var/log/secure;a;stored;0
src.tcp;s_net;afsocket_sd.(stream,AF_INET(0.0.0.0:514));a;connections;0
src.network;s_net;afsocket_sd.(stream,AF_INET(0.0.0.0:601));a;connections;0
.`

func echoServer(c net.Conn) {
	log.Printf("Client connected [%s]", c.RemoteAddr().Network())
	buff := bufio.NewReader(c)
	line, err := buff.ReadString('\n')
	if err != nil {
		log.Printf("Error reading header from control socket: %v", err)
		return
	}

	/* only supports command "STATS" for now */
	fmt.Printf("command: %s", line)
	if strings.Contains(line, "STATS") {
		log.Printf("sending STATS")
		c.Write([]byte(statsMock))
	} else {
		/* default to acting like an echo server */
		io.Copy(c, c)
	}
	c.Close()
}

func main() {
	if err := os.RemoveAll(SockAddr); err != nil {
		log.Fatal(err)
	}

	l, err := net.Listen("unix", SockAddr)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	defer l.Close()

	for {
		// Accept new connections, dispatching them to echoServer
		// in a goroutine.
		conn, err := l.Accept()
		if err != nil {
			log.Fatal("accept error:", err)
		}

		go echoServer(conn)
	}
}

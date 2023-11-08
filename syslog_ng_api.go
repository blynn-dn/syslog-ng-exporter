package main

// Adds support for syslog-ng commands "RELOAD" and "HEALTHCHECK" to the existing "STATS" support.
// Note that while "STATS" renders Prometheus metrics, "RELOAD" and "HEALTHCHECK" render json

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	namespace = "syslog_ng" // For Prometheus metrics.
)

var (
	app              = kingpin.New("syslog_ng_exporter", "A Syslog-NG exporter for Prometheus")
	showVersion      = app.Flag("version", "Print version information.").Bool()
	listeningAddress = app.Flag("telemetry.address", "Address on which to expose metrics.").Default(":9577").String()
	metricsEndpoint  = app.Flag("telemetry.endpoint", "Path under which to expose metrics.").Default("/metrics").String()
	socketPath       = app.Flag("socket.path", "Path to syslog-ng control socket.").Default("/var/lib/syslog-ng/syslog-ng.ctl").String()
)

type Exporter struct {
	sockPath string
	mutex    sync.Mutex

	srcConnections *prometheus.Desc
	srcProcessed   *prometheus.Desc
	dstProcessed   *prometheus.Desc
	dstDropped     *prometheus.Desc
	dstStored      *prometheus.Desc
	dstWritten     *prometheus.Desc
	dstMemory      *prometheus.Desc
	up             *prometheus.Desc
	scrapeFailures prometheus.Counter
}

type Stat struct {
	objectType string
	id         string
	instance   string
	state      string
	metric     string
	value      float64
}

func NewExporter(path string) *Exporter {
	return &Exporter{
		sockPath: path,
		srcConnections: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "source_connections", "total"),
			"Number of source connections.",
			[]string{"type", "id", "source"},
			nil),
		srcProcessed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "source_messages_processed", "total"),
			"Number of messages processed by this source.",
			[]string{"type", "id", "source"},
			nil),
		dstProcessed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "destination_messages_processed", "total"),
			"Number of messages processed by this destination.",
			[]string{"type", "id", "destination"},
			nil),
		dstDropped: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "destination_messages_dropped", "total"),
			"Number of messages dropped by this destination due to store overflow.",
			[]string{"type", "id", "destination"},
			nil),
		dstStored: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "destination_messages_stored", "total"),
			"Number of messages currently stored for this destination.",
			[]string{"type", "id", "destination"},
			nil),
		dstWritten: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "destination_messages_written", "total"),
			"Number of messages successfully written by this destination.",
			[]string{"type", "id", "destination"},
			nil),
		dstMemory: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "destination_bytes_stored", "total"),
			"Bytes of memory currently used to store messages for this destination.",
			[]string{"type", "id", "destination"},
			nil),
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Reads 1 if the syslog-ng server could be reached, else 0.",
			nil,
			nil),
		scrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_scrape_failures_total",
			Help:      "Number of errors while scraping syslog-ng.",
		}),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.srcConnections
	ch <- e.srcProcessed
	ch <- e.dstProcessed
	ch <- e.dstDropped
	ch <- e.dstStored
	ch <- e.dstWritten
	ch <- e.dstMemory
	ch <- e.up
	e.scrapeFailures.Describe(ch)
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if err := e.collect(ch); err != nil {
		log.Errorf("Error scraping syslog-ng: %s", err)
		e.scrapeFailures.Inc()
		e.scrapeFailures.Collect(ch)
	}
}

func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	conn, err := net.Dial("unix", e.sockPath)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		return fmt.Errorf("Error connecting to syslog-ng: %v", err)
	}

	defer conn.Close()

	err = conn.SetDeadline(time.Now().Add(time.Second))
	if err != nil {
		return fmt.Errorf("Failed to set conn deadline: %v", err)
	}

	_, err = conn.Write([]byte("STATS\n"))
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		return fmt.Errorf("Error writing to control socket: %v", err)
	}

	buff := bufio.NewReader(conn)

	_, err = buff.ReadString('\n')
	if err != nil {
		return fmt.Errorf("Error reading header from control socket: %v", err)
	}

	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 1)

	for {
		line, err := buff.ReadString('\n')

		if err != nil || line[0] == '.' {
			log.Debug("Reached end of STATS output")
			break
		}

		stat, err := parseStatLine(line)
		if err != nil {
			log.Debugf("Skipping STATS line: %v", err)
			continue
		}

		log.Debugf("Successfully parsed STATS line: %v", stat)
		log.Debugf("case: %v, metric: %v, id: %v", stat.objectType[0:4], stat.metric, stat.id)

		switch stat.objectType[0:4] {
		case "src.", "sour":
			switch stat.metric {
			case "processed":
				ch <- prometheus.MustNewConstMetric(e.srcProcessed, prometheus.CounterValue,
					stat.value, stat.objectType, stat.id, stat.instance)
			case "connections":
				ch <- prometheus.MustNewConstMetric(e.srcConnections, prometheus.CounterValue,
					stat.value, stat.objectType, stat.id, stat.instance)
			}

		case "dst.", "dest":
			switch stat.metric {
			case "dropped":
				ch <- prometheus.MustNewConstMetric(e.dstDropped, prometheus.CounterValue,
					stat.value, stat.objectType, stat.id, stat.instance)
			case "processed":
				ch <- prometheus.MustNewConstMetric(e.dstProcessed, prometheus.CounterValue,
					stat.value, stat.objectType, stat.id, stat.instance)
			case "written":
				ch <- prometheus.MustNewConstMetric(e.dstWritten, prometheus.CounterValue,
					stat.value, stat.objectType, stat.id, stat.instance)
			case "stored", "queued":
				ch <- prometheus.MustNewConstMetric(e.dstStored, prometheus.GaugeValue,
					stat.value, stat.objectType, stat.id, stat.instance)
			case "memory_usage":
				ch <- prometheus.MustNewConstMetric(e.dstMemory, prometheus.GaugeValue,
					stat.value, stat.objectType, stat.id, stat.instance)
			}
		}
	}

	return nil
}

func parseStatLine(line string) (Stat, error) {
	part := strings.SplitN(strings.TrimSpace(line), ";", 6)
	if len(part) < 6 {
		return Stat{}, fmt.Errorf("insufficient parts: %d < 6", len(part))
	}

	if len(part[0]) < 4 {
		return Stat{}, fmt.Errorf("invalid name: %s", part[0])
	}

	val, err := strconv.ParseFloat(part[5], 64)
	if err != nil {
		return Stat{}, err
	}

	stat := Stat{part[0], part[1], part[2], part[3], part[4], val}

	return stat, nil
}

// Processes syslog-ng commands; "RELOAD" and "HEALTHCHECK"
func processCommand(w http.ResponseWriter, sockPath string, command string) {

	//

	// todo evaluate the command to ensure it's supported

	log.Infof("process command: %s", command)
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		JSONError(w, fmt.Errorf("error connecting to syslog-ng: %v", err), 500)
		return
	}

	defer conn.Close()

	err = conn.SetDeadline(time.Now().Add(time.Second))
	if err != nil {
		JSONError(w, fmt.Errorf("failed to set conn deadline: %v", err), 500)
		return
	}

	// send the command
	_, err = conn.Write([]byte(fmt.Sprintf("%s\n", command)))
	if err != nil {
		JSONError(w, fmt.Errorf("error writing to control socket: %v", err), 500)
		return
	}
	log.Infof("sent command: %s", command)

	buff := bufio.NewReader(conn)
	payload := make(map[string]any)
	payloadData := make(map[string]any)
	var errors []string

	// retrieve the command results
	lineNo := 0
	for {
		line, err := buff.ReadString('\n')
		log.Infof("line command: %s", line)

		if err != nil || line[0] == '.' {
			log.Info("Reached end of STATS output")
			break
		}

		switch command {
		case "RELOAD":
			// Example of syslog-ng RELOAD result:
			// OK Config reload successful
			//.

			payload["message"] = line
			if !strings.Contains(line, "OK Config reload successful") {
				errors = append(errors, "failed to reload")
			}

			break
		case "HEALTHCHECK":
			// Example of syslog-ng HEALTHCHECK results:
			// OK syslogng_io_worker_latency_seconds 6.0819000000000002e-05
			// syslogng_mainloop_io_worker_roundtrip_latency_seconds 0.000114926
			// syslogng_internal_events_queue_usage_ratio 0
			//.
			part := strings.SplitN(strings.TrimSpace(line), " ", 3)
			if lineNo == 0 && len(part) == 3 {
				if part[0] == "OK" {
					payload["message"] = "OK"
					payloadData[part[1]] = part[2]
				} else {
					errors = append(errors, line)
				}
			} else if len(part) == 2 {
				payloadData[part[0]] = part[1]
			} else {
				errors = append(
					errors, fmt.Sprintf("error: invalid/unexpected results, line: %d: %s", lineNo, line))
			}
			break
		}

		lineNo++
	}

	// if there are errors then set the status as "failed" and apply the errors
	if len(errors) != 0 {
		payload["status"] = "failed"
		payload["error_messages"] = errors
	} else {
		payload["status"] = "success"
	}

	// add "data" to the payload if it exists
	if payloadData != nil {
		payload["data"] = payloadData
	}

	// send the results to the requester as json
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(payload)
	if err != nil {
		JSONError(w, fmt.Errorf("error: marshalling payload: %v", err), 500)
	}
}

// JSONError
// Helper: formats `error` into a json payload and sends the payload to the client
func JSONError(w http.ResponseWriter, err error, code int) {
	log.Info(err.Error())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	resp := map[string]string{"message": err.Error(), "status": "failed"}
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		err := fmt.Errorf("critical error: %v", err)
		log.Infof("error: %s", err.Error())

		// try one more time to let client know about the error
		_, err = w.Write([]byte(err.Error()))
	}
}

func main() {
	log.AddFlags(app)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("syslog_ng_exporter"))
		os.Exit(0)
	}

	exporter := NewExporter(*socketPath)
	prometheus.MustRegister(exporter)
	prometheus.MustRegister(version.NewCollector("syslog_ng_exporter"))

	log.Infoln("Starting syslog_ng_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())
	log.Infof("Starting server: %s", *listeningAddress)

	http.Handle(*metricsEndpoint, promhttp.Handler())

	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		processCommand(w, *socketPath, "RELOAD")
	})

	http.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		processCommand(w, *socketPath, "HEALTHCHECK")
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
			<head><title>Syslog-NG Exporter/API</title></head>
			<body>
			<h1>Syslog-NG Exporter/API</h1>
			<ul>
				<li><a href='` + *metricsEndpoint + `'>Metrics</a></li>
				<li><a href='/reload'>Reload</a></li>
				<li><a href='/healthcheck'>Healthcheck</a></li>
			</ul>
			</body>
			</html>`))
		if err != nil {
			log.Warnf("Failed sending response: %v", err)
		}
	})

	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

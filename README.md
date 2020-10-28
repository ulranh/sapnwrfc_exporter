## SAP NWRFC Exporter for Prometheus  [![Go Report Card](https://goreportcard.com/badge/github.com/ulranh/sapnwrfc_exporter)](https://goreportcard.com/report/github.com/ulranh/sapnwrfc_exporter)

The purpose of this exporter is to support monitoring SAP instances with [Prometheus](https://prometheus.io) and [Grafana](https://grafana.com). It is possible to count the occurrence for some defined values of a field in a SAP function module result table - for example the number of dialog, batch and update processes or the number of the SAP lock entries at a given time. Another possibility is to use the export field results of a function module as prometheus label values - for example to record the database client version and kernel patch level of the SAP instance.

## Prerequisites

You need the SAP NWRFC SDK 7.50 PL3 library (or later) as a prequisite for the installation of this exporter. To download this library you must have a customer or partner account on the SAP Service Marketplace. Please take a look at SAP note "2573790 - Installation, Support and Availability of the SAP NetWeaver RFC Library 7.50" and the [gorfc](https://github.com/SAP/gorfc) readme.

With the nwrfcsdk zip file unpacked in /usr/sap, the following environment variables are necessary under Linux:

```
LD_LIBRARY_PATH="/usr/sap/nwrfcsdk/lib"
CGO_LDFLAGS="-L /usr/sap/nwrfcsdk/lib"
CGO_CFLAGS="-I /usr/sap/nwrfcsdk/include"
CGO_LDFLAGS_ALLOW="^.*"
CGO_CFLAGS_ALLOW="^.*"
```

## Installation
To build the exporter you need the [Go](https://golang.org/) programming language. It can then be built with:

```
$ git clone git@github.com:ulranh/sapnwrfc_exporter.git
$ cd sapnwrfrc_exporter
$ go build
```
## Preparation

#### SAP User
A SAP user is necessary for every SAP system with read access for all affected remote function modules.

#### Configfile
The next necessary piece is a [toml](https://github.com/toml-lang/toml) configuration file where the encrypted passwords, the system- and metric-information are stored. The expected default name is sapnwrfc_exporter.toml and the expected default location of this file is the home directory of the user. The flag -config can be used to assign other locations or names.

The file contains a Systems- followed by a TableMetrics- and/or FieldMetrics- slice:

```
// user/password logon
[[Systems]]
  Name = "t01"
  Usage = "test"
  Tags = []
  User = "sapuser1"
  Lang = "en"
  Client = "100"
  Server = "host1.example.com"
  Sysnr = "01"

// load balanced logon
[[Systems]]
  Name = "t02"
  Usage = "test"
  Tags = []
  User = "sapuser2"
  Lang = "en"
  Client = "100"
  Mshost = "server2.example.com"
  Msserv = "3600"
  Group = "server2_1"

// load balanced logon and saprouter
[[Systems]]
  Name = "t03"
  Usage = "test"
  Tags = []
  User = "sapuser3"
  Lang = "en"
  Client = "100"
  Mshost = "server3.example.com"
  Msserv = "3600"
  Group = "server3_1"
  Saprouter = "/H/saprouter/W/pass/H/target"

[[TableMetrics]]
  Name = "sap_processes"
  Help = "Number of sm50 processes"
  MetricType = "gauge"
  TagFilter = []
  FunctionModule = "TH_WPINFO"
  Table = "WPLIST"
  AllServers = true
  [TableMetrics.Params]
    SRVNAME = ""
  [TableMetrics.RowCount]
    WP_TABLE = ["dbvm", "dbvl", "ma61v", "mdup"]
    WP_TYP = ["dia", "bgd", "upd", "upd2", "spo"] # with logon language "de": ["dia", "btc", "upd", "upd2", "spo"] 
  [TableMetrics.RowFilter]
    WP_STATUS = ["on hold", "running"] # with logon language "de": ["hält", "läuft"]

[[FieldMetrics]]
  Name = "sap_kernel_info"
  Help = "SAP kernel and db client info"
  MetricType = "gauge"
  TagFilter = []
  AllServers = true
  FunctionModule = "TH_SAPREL2"
  FieldLabels = ["kern_rel", "kern_dblib", "kern_patchlevel"]
```

Below is a description of the system and metric struct fields:

#### System information

| Field      | Type         | Description | Example |
| ---------- | ------------ |------------ | ------- |
| Name       | string       | SAP SID  | "P01", "q02" |
| Usage      | string       | SAP system usage | "development", "test", "production" |
| Tags       | string array | Tags describing the system | ["erp"], ["bw"] |
| User       | string       | SAP system user | |
| Lang       | string       | The entries of TableMetrics.RowFilterOut and TableMetrics.RowCount can differ, depending on the logon language | "en", "de" |
| Client     | string       | SAP system client | |
| Server     | string       | SAP system server | |
| Sysnr      | string       | SAP system number | |
| Mshost      | string       | Message Server | |
| Msserv      | string       | is needed only, if the service of the message server is not defined as sapms<Sysnr> in /etc/services |3600 |
| Group      | string       | Logon group (transaction SMLG) | |
| Saprouter  | string       | SAP router string | |

#### TableMetric information

| Field        | Type         | Description | Example |
| ------------ | ------------ |------------ | ------- |
| Name         | string       | Metric name (words separated by underscore, otherwise a panic can occur) | "sap_processes" |
| Help         | string       | Metric help text | "Number of sm50 processes"|
| MetricType   | string       | Type of metric | "counter" or "gauge" |
| TagFilter    | string array | The metric will only be executed, if all values correspond with the existing tenant tags | TagFilter ["erp"] needs at least system Tag ["erp"] otherwise the metric will not be used |
| AllServers   | bool         | When true, the metric will be created for every applicationserver of the SAP system | "true","false" |
| FunctionModule | string       | Function module name | "TH_WPINFO" |
| Table        | string       | Result table of function module | "WPLIST" |
| TableMetrics.Params | map[string]interface{} | Params of the function module |  |
| TableMetrics.RowCount | map[string]interface{} | Values of a table result field, that should be counted  |  |
| TableMetrics.RowFilter | map[string]interface{} | Only some values of a table field shall be considered all other lines will be skipped|  |

#### FieldMetric information

Name, Help, MetricType, TagFilter, AllServers, FunctionModule and FieldMetrics.Params same as above.

| Field        | Type         | Description | Example |
| ------------ | ------------ |------------ | ------- |
| FieldLabels  | string array | Function module export field names with values that should be recorded | ["kern_rel","kern_patchlevel"] |

#### Database passwords

With the following commands the passwords for the example tenants above can be written to the Secret section of the configfile:
```
$ ./sapnwrfc_exporter pw -system t01 -config ./sapnwrfc_exporter.toml
$ ./sapnwrfc_exporter pw -system t02 -config ./sapnwrfc_exporter.toml
```
With one password for multiple systems, the following notation is also possible:
```
$ ./sapnwrfc_exporter pw -tenant t01,t02 -config ./sapnwrfc_exporter.toml
```

## Usage

Now the web server can be started:
#### Binary

The default port is 9663 which can be changed with the -port flag.
```
$ ./sapnwrfc_exporter web -config ./sapnwrfc_exporter.toml
```

#### Docker
The Docker image can be built with the existing Dockerfile. As a prerequisite the SAP NW RFC library has to be unzipped in the working directory. Then it can be started as follows:
```
$ docker run -d --name=sapnwrfc_exporter --restart=always -p 9663:9663 -v /home/<user>/sapnwrfc_exporter.toml:/app/sapnwrfc_exporter.toml \<image name\>
```

#### Kubernetes
Due to the license restrictions it is not possible to publish a docker image that includes the sapnwrfc library. But all SAP customers can create their own images and use them. An example config can be found in the examples folder. First of all create a SAP namespace. Then apply the created configfile as configmap and start the deployment:
```
$ kubectl apply -f sap-namespace.yaml 
$ kubectl create configmap sapnwrfc-config -n sap --from-file ./sapnwrfc_exporter.toml -o yaml
$ kubectl apply -f sapnwrfc-deployment.yaml
```

Configfile changes can be applied in the following way:
```
$ kubectl create configmap sapnwrfc-config -n sap --from-file ./sapnwrfc_exporter.toml -o yaml --dry-run | sudo kubectl replace -f -
$ kubectl scale --replicas=0 -n sap deployment sapnwrfc-exporter
$ kubectl scale --replicas=1 -n sap deployment sapnwrfc-exporter
```

#### Prometheus configfile
The necessary entries in the prometheus configfile can look something like the following:
```
  - job_name: sap
        scrape_interval: 60s
        static_configs:
          - targets: ['172.45.111.105:9663']
            labels:  {'instance': 'sapnwrfc-exporter-test'}
          - targets: ['sapnwrfc_exporter.sap.svc.cluster.local:9663']
            labels:  {'instance': 'sapnwrfc-exporter-dev'}
```

## Result
The resulting information can be found in the Prometheus expression browser and can be used as normal for creating alerts or displaying dashboards in Grafana.

The image below for example shows the number of active dialog, batch and update processes at a given time:

 ![processes](/examples/images/processes.png)

## More Information
* [Monitoring SAP and Hana Instances with Prometheus and Grafana](https://blogs.sap.com/2020/02/07/monitoring-sap-and-hana-instances-with-prometheus-and-grafana/) 

package main

import "flag"


func main() {
	var socketPath string
	var healthcheckUrl string

	flag.StringVar(&healthcheckUrl, "healthcheckUrl", "", "Healthcheck URL for reporting container status")
	flag.StringVar(&socketPath, "socketPath", "/var/run/containermon.sock", "File Path to container socket")

}
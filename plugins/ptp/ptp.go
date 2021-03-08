package ptp

import (
	"fmt"
	"log"
	"math/rand"
	"os/exec"
)

func checkPTPStatus() string { //nolint:deadcode,unused
	//pmc -u -b 0 "GET CURRENT_DATA_SET" -s /var/run/ptp4l.0.socket
	//pmc -u -b 0 "GET TIME_STATUS_NP" -s /var/run/ptp4l.0.socket
	// pmc -u -b 0 "GET PORT_DATA_SET" -s /var/run/ptp4l.0.socket

	cmdString := []string{"GET CURRENT_DATA_SET", "GET TIME_STATUS_NP", "GET PORT_DATA_SET"}
	cmd := exec.Command("pmc", "-u", "-b 0", cmdString[rand.Int()%len(cmdString)], "-s", "/var/run/ptp4l.0.socket")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error reading ptp satatus %v", err)
	}
	if string(out) == "" {
		return fmt.Sprintf("error reading ptp satatus %v", err)
	}
	return string(out)
}

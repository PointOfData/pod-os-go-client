// Package debuglog is TEMPORARY debug-mode instrumentation. It appends NDJSON
// lines to a fixed session log file so we can capture runtime evidence of how a
// gateway RST / dead connection is handled during long build-sample-enm runs.
//
// REMOVE this package and all call sites once the dead-connection bug is fixed
// and verified.
package debuglog

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

const (
	logPath   = "/home/schindler/Development/PodOsManagementDashboard/.cursor/debug-d8e0a9.log"
	sessionID = "d8e0a9"
)

var mu sync.Mutex

// Log appends a single NDJSON entry. hypothesisID maps the line to a debug
// hypothesis; location/message describe the site; data carries runtime values.
func Log(hypothesisID, location, message string, data map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	entry := map[string]any{
		"sessionId":    sessionID,
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
}

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/msgfile"
)

// msgDirs returns the message-file library search path:
// SATPULSE_GPSMSG_PATH when set, otherwise the user's own library
// followed by the installed locations.
func msgDirs() []string {
	if dirs := msgfile.EnvDirs(); dirs != nil {
		return dirs
	}
	dirs := []string{"/usr/local/share/satpulse/gpsmsg", "/usr/share/satpulse/gpsmsg"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append([]string{filepath.Join(home, ".satpulse", "gpsmsg")}, dirs...)
	}
	return dirs
}

// msgCatalog is the /api/msgfile/catalog response: the message files
// found on the library search path, in search order (the UI sorts and
// groups), and the vendor to preselect.
type msgCatalog struct {
	Names     []msgfile.Entry `json:"names"`
	Preselect string          `json:"preselect,omitempty"` // vendor to preselect, or ""
}

// msgFileResult is the /api/msgfile/select response: the path the name
// resolved to (echoed for display) and the tags SetMsgFile parsed from
// the file.
type msgFileResult struct {
	Path string               `json:"path"`
	Tags []session.MsgFileTag `json:"tags"`
}

func (s *server) handleMsgCatalog(w http.ResponseWriter, _ *http.Request) {
	names := msgfile.ListNames(s.msgDirs)
	writeJSON(w, msgCatalog{Names: names, Preselect: s.msgPreselect(names)})
}

func (s *server) handleMsgSelect(w http.ResponseWriter, r *http.Request) {
	var req msgfile.Name
	if !readJSON(w, r, &req) {
		return
	}
	// FindName's component validation is the path-traversal guard: a
	// Name never resolves outside a search-path directory.
	path, err := msgfile.FindName(req, s.msgDirs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	mf, err := msgfile.Load(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, msgFileResult{Path: path, Tags: s.sess.SetMsgFile(mf)})
}

func (s *server) handleMsgSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tag  string `json:"tag"`
		Port string `json:"port"`
		Save bool   `json:"save"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.sess.SendMsgFile(req.Tag, req.Port, req.Save); err != nil {
		sessionError(w, err)
		return
	}
	writeJSON(w, struct{}{})
}

func (s *server) handleMsgCancel(w http.ResponseWriter, _ *http.Request) {
	if err := s.sess.CancelMsgSend(); err != nil {
		sessionError(w, err)
		return
	}
	writeJSON(w, struct{}{})
}

// msgPreselect returns the vendor the UI should preselect: the session
// vendor (--vendor if set, else the vendor the probe detected) matched
// by lowercased name against the catalog's vendors; "" when nothing
// matches.
func (s *server) msgPreselect(names []msgfile.Entry) string {
	vendor := strings.ToLower(s.sessionVendorName())
	if vendor == "" {
		return ""
	}
	for _, e := range names {
		if strings.ToLower(e.Vendor) == vendor {
			return e.Vendor
		}
	}
	return ""
}

// sessionVendorName returns the vendor name driving preselection: the
// --vendor value when one was given, otherwise the vendor the probe
// detected (empty under passive detection).
func (s *server) sessionVendorName() string {
	if s.vendor != gpsreg.VendorUnknown {
		return s.vendor.String()
	}
	if r := s.sess.Receiver(); r.Info.IsSet() {
		return r.Info.Get().Vendor
	}
	return ""
}

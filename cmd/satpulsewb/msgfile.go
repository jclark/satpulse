package main

import (
	"net/http"
	"strings"

	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/msgfile"
)

// msgDirs returns the message-file library search path: the
// SATPULSE_GPSMSG_PATH directories, when set, followed by the built-in
// library.
func msgDirs() []msgfile.Dir {
	return append(msgfile.EnvDirs(), msgfile.Builtin())
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
	dir, name, err := msgfile.FindName(req, s.msgDirs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	mf, err := msgfile.LoadFS(dir, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, msgFileResult{Path: dir.DisplayPath(name), Tags: s.sess.SetMsgFile(mf)})
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

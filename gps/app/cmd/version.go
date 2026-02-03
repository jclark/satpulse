package cmd

var version string
var buildDate string

func VersionInfo() string {
	s := ""
	if version != "" {
		s = "version " + version
	}
	if buildDate != "" {
		if s != "" {
			s += ", "
		}
		s += "build date " + buildDate
	}
	if s == "" {
		return "no version information available"
	}
	return "satpulse" + " " + s
}

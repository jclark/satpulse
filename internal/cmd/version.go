package cmd

var gitVersion string
var buildDate string

func VersionInfo() string {
	s := ""
	if gitVersion != "" {
		s = "git version " + gitVersion
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

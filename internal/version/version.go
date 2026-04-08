package version

var (
	Name    = "phis-host"
	Version = "0.1.0-dev"
)

func String() string {
	return Name + " " + Version
}

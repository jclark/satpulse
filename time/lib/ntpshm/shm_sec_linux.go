//go:build linux && (amd64 || arm64)

package ntpshm

//go:generate sh -c "{ echo '//go:build linux && (amd64 || arm64)'; echo; go tool cgo -godefs types_linux.go; } | gofmt > ztypes_linux.go && rm -rf _obj"

type shmSec = int64

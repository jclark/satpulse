//go:build linux && (arm || 386)

package ntpshm

//go:generate sh -c "{ echo '//go:build linux && (arm || 386)'; echo; go tool cgo -godefs types.go; } | gofmt > ztypes_linux32.go && rm -rf _obj _cgo_*.o"

type shmSec = int32

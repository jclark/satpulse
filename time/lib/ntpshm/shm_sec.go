//go:build (linux || darwin || freebsd) && (amd64 || arm64)

package ntpshm

//go:generate sh -c "{ echo '//go:build $GOOS && (amd64 || arm64)'; echo; go tool cgo -godefs types.go; } | gofmt > ztypes_$GOOS.go && rm -rf _obj _cgo_*.o"

type shmSec = int64

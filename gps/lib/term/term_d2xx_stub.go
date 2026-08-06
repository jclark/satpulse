//go:build !windows && !darwin

package term

func tryD2XX(string, []AttrSetter) (Term, bool, error) {
	return nil, false, nil
}

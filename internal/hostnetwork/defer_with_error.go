package hostnetwork

func deferWithError(errors *[]error, toDefer func() error) {
	if err := toDefer(); err != nil {
		*errors = append(*errors, err)
	}
}

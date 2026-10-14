//go:build !darwin

package ebitenplatform

func installPinchMonitor(*pinchQueue) error { return errPinchUnsupported }

//go:build !darwin

package ebitenplatform

func installOpenFilesHandler(*openQueue) error { return errOpenFilesUnsupported }

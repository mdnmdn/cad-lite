//go:build !js

package ebitenplatform

func installWheelCtrlListener() {}

func takeWheelCtrl() bool { return false }

//go:build js && wasm
// +build js,wasm

package interfaces

func (tc *TCPClientInterface) setTimeoutsLinux() error {
	return nil
}

func (tc *TCPClientInterface) setTimeoutsOSX() error {
	return nil
}

package cli

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// readNoEcho reads a line from stdin without echoing characters.
// Uses POSIX termios to disable echo temporarily.
func readNoEcho() ([]byte, error) {
	fd := int(syscall.Stdin)

	// Get current terminal state
	var oldState syscall.Termios
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		// TCGETS
		uintptr(0x5401),
		uintptr(unsafe.Pointer(&oldState)),
		0, 0, 0,
	); errno != 0 {
		return nil, fmt.Errorf("TCGETS: %v", errno)
	}

	// Disable echo
	newState := oldState
	newState.Lflag &^= syscall.ECHO
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		// TCSETS
		uintptr(0x5402),
		uintptr(unsafe.Pointer(&newState)),
		0, 0, 0,
	); errno != 0 {
		return nil, fmt.Errorf("TCSETS: %v", errno)
	}

	// Restore echo on exit
	defer func() {
		syscall.Syscall6( //nolint
			syscall.SYS_IOCTL,
			uintptr(fd),
			uintptr(0x5402),
			uintptr(unsafe.Pointer(&oldState)),
			0, 0, 0,
		)
	}()

	// Read line
	var buf []byte
	tmp := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			if tmp[0] == '\n' || tmp[0] == '\r' {
				break
			}
			buf = append(buf, tmp[0])
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

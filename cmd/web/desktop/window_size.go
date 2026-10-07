package main

const (
	desktopWindowDefaultWidth  = 1180
	desktopWindowDefaultHeight = 760
	desktopWindowMinimumWidth  = 600
	desktopWindowMinimumHeight = 400
)

// desktopWindowSize keeps the native window inside the current display while
// preserving a useful minimum size for the responsive app layout.
func desktopWindowSize(screenWidth, screenHeight int) (width, height, minWidth, minHeight int) {
	if screenWidth <= 0 || screenHeight <= 0 {
		return desktopWindowDefaultWidth, desktopWindowDefaultHeight, desktopWindowMinimumWidth, desktopWindowMinimumHeight
	}

	width = min(desktopWindowDefaultWidth, max(320, screenWidth-48))
	height = min(desktopWindowDefaultHeight, max(320, screenHeight-96))
	minWidth = min(desktopWindowMinimumWidth, width)
	minHeight = min(desktopWindowMinimumHeight, height)
	return width, height, minWidth, minHeight
}

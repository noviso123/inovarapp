package main

import "testing"

func TestDesktopWindowSizeFitsCurrentDisplay(t *testing.T) {
	tests := []struct {
		name                        string
		screenWidth, screenHeight   int
		wantWidth, wantHeight       int
		wantMinWidth, wantMinHeight int
	}{
		{name: "large display", screenWidth: 1920, screenHeight: 1080, wantWidth: 1180, wantHeight: 760, wantMinWidth: 600, wantMinHeight: 400},
		{name: "laptop display", screenWidth: 1366, screenHeight: 768, wantWidth: 1180, wantHeight: 672, wantMinWidth: 600, wantMinHeight: 400},
		{name: "compact display", screenWidth: 800, screenHeight: 600, wantWidth: 752, wantHeight: 504, wantMinWidth: 600, wantMinHeight: 400},
		{name: "small display", screenWidth: 520, screenHeight: 390, wantWidth: 472, wantHeight: 320, wantMinWidth: 472, wantMinHeight: 320},
		{name: "unknown display uses safe default", wantWidth: 1180, wantHeight: 760, wantMinWidth: 600, wantMinHeight: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, minWidth, minHeight := desktopWindowSize(tt.screenWidth, tt.screenHeight)
			if width != tt.wantWidth || height != tt.wantHeight || minWidth != tt.wantMinWidth || minHeight != tt.wantMinHeight {
				t.Fatalf("desktopWindowSize() = (%d, %d, %d, %d), want (%d, %d, %d, %d)", width, height, minWidth, minHeight, tt.wantWidth, tt.wantHeight, tt.wantMinWidth, tt.wantMinHeight)
			}
			if tt.screenWidth > 0 && (width > tt.screenWidth || minWidth > width) {
				t.Errorf("window width %d (min %d) does not fit display width %d", width, minWidth, tt.screenWidth)
			}
			if tt.screenHeight > 0 && (height > tt.screenHeight || minHeight > height) {
				t.Errorf("window height %d (min %d) does not fit display height %d", height, minHeight, tt.screenHeight)
			}
		})
	}
}

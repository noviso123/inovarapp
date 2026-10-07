package webapp

type browserPermissionResult struct {
	permission string
	err        error
}

type nativePushResult struct {
	platform string
	token    string
	err      error
}

type desktopNotificationResult struct {
	allowed bool
	err     error
}

//go:build !linux && !darwin

package conversationarchive

func filesystemFreeBytes(string) int64 { return 0 }

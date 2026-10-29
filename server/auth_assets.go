package main

import "os"

// The content version avoids stale fixed-name CSS even when an older response
// was cached before revalidation headers were introduced. Read at render time
// so a frontend-only build takes effect without restarting Go.
func authAssetURL(path string) string {
	body, err := os.ReadFile("admin/dist" + path)
	if err != nil {
		return path
	}
	return path + "?v=" + digest(string(body))[:16]
}
